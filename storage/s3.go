package storage

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/OdyseeTeam/transcoder/internal/config"
	"github.com/OdyseeTeam/transcoder/library"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

var ErrStreamExists = errors.New("stream already exists")

type discardAt struct{}

func (discardAt) WriteAt(p []byte, off int64) (int, error) {
	return len(p), nil
}

func InitS3Drivers(storageConfigs config.Storages) (map[string]library.Storage, error) {
	storages := map[string]library.Storage{}
	for _, v := range storageConfigs {
		s, err := InitS3Driver(v)
		if err != nil {
			return nil, err
		}
		storages[v.Name] = s
	}
	return storages, nil
}

func InitS3Driver(cfg config.S3Config) (*S3Driver, error) {
	if cfg.Name == "" {
		return nil, errors.New("storage name must me configured")
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.Key, cfg.Secret, "")),
		awsconfig.WithRegion(cfg.Region),
	)
	if err != nil {
		return nil, err
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(cfg.Endpoint)
		o.ResponseChecksumValidation = aws.ResponseChecksumValidationWhenRequired
		o.UsePathStyle = true
	})

	s := &S3Driver{
		config: cfg,
		client: client,
		awsCfg: awsCfg,
	}

	if cfg.CreateBucket {
		logger.Infow("creating s3 bucket", "name", cfg.Bucket)
		_, err := client.CreateBucket(context.Background(), &s3.CreateBucketInput{
			Bucket: aws.String(cfg.Bucket),
			ACL:    types.BucketCannedACLPublicRead,
		})
		if err != nil {
			var bucketExists *types.BucketAlreadyOwnedByYou
			if !errors.As(err, &bucketExists) {
				return nil, err
			}
		}
	}

	return s, nil
}

type S3Driver struct {
	config config.S3Config
	client *s3.Client
	awsCfg aws.Config
}

func (s *S3Driver) Name() string {
	return s.config.Name
}

func (s *S3Driver) GetURL(item string) string {
	return strings.Join([]string{s.config.Endpoint, s.config.Bucket, item}, "/")
}

func (s *S3Driver) Put(stream *library.Stream, overwrite bool) error {
	return s.PutWithContext(context.Background(), stream, overwrite)
}

func (s *S3Driver) PutWithContext(ctx context.Context, stream *library.Stream, overwrite bool) error {
	if !overwrite {
		dl := manager.NewDownloader(s.client)
		_, err := dl.Download(ctx, discardAt{}, &s3.GetObjectInput{
			Bucket: aws.String(s.config.Bucket),
			Key:    aws.String(s3FileKey(stream.TID(), library.MasterPlaylistName)),
		})
		if err == nil {
			return ErrStreamExists
		}
	}

	ul := manager.NewUploader(s.client)
	err := stream.Walk(
		func(fi fs.FileInfo, fullPath, name string) error {
			var ctype string
			f, err := os.Open(fullPath)
			if err != nil {
				return err
			}
			defer f.Close()

			switch path.Ext(name) {
			case library.PlaylistExt:
				ctype = library.PlaylistContentType
			case library.FragmentExt:
				ctype = library.FragmentContentType
			default:
				ctype = "text/plain"
			}
			logger.Debugw("uploading", "key", s3FileKey(stream.TID(), name), "ctype", ctype, "size", fi.Size(), "bucket", s.config.Bucket)
			_, err = ul.Upload(ctx, &s3.PutObjectInput{
				Bucket:      aws.String(s.config.Bucket),
				Key:         aws.String(s3FileKey(stream.TID(), name)),
				ContentType: aws.String(ctype),
				Body:        f,
				ACL:         types.ObjectCannedACLPublicRead,
			})
			if err != nil {
				return err
			}
			return nil
		},
	)

	return err
}

func (s *S3Driver) Delete(tid string) error {
	ctx, cancelFn := context.WithTimeout(context.Background(), 600*time.Second)
	bucket := aws.String(s.config.Bucket)
	objects, err := s.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket:  bucket,
		Prefix:  aws.String(tid),
		MaxKeys: aws.Int32(1000),
	})
	cancelFn()
	if err != nil {
		return err
	}

	delObjects := []types.ObjectIdentifier{}
	for _, o := range objects.Contents {
		delObjects = append(delObjects, types.ObjectIdentifier{Key: o.Key})
		if len(delObjects) >= 100 {
			delInput := &s3.DeleteObjectsInput{
				Bucket: bucket,
				Delete: &types.Delete{
					Objects: delObjects,
					Quiet:   aws.Bool(false),
				},
			}
			ctx, cancelFn := context.WithTimeout(context.Background(), 600*time.Second)
			_, err = s.client.DeleteObjects(ctx, delInput)
			cancelFn()
			if err != nil {
				return err
			}
			delObjects = []types.ObjectIdentifier{}
		}
	}

	if len(delObjects) > 0 {
		delInput := &s3.DeleteObjectsInput{
			Bucket: bucket,
			Delete: &types.Delete{
				Objects: delObjects,
				Quiet:   aws.Bool(false),
			},
		}
		ctx, cancelFn := context.WithTimeout(context.Background(), 30*time.Second)
		_, err = s.client.DeleteObjects(ctx, delInput)
		cancelFn()
		if err != nil {
			return err
		}
	}

	if objects.IsTruncated != nil && *objects.IsTruncated {
		return s.Delete(tid)
	}

	return nil
}

func (s *S3Driver) DeleteFragments(tid string, fragments []string) error {
	workerCount := 120
	batchSize := 20

	batches := make(chan []types.ObjectIdentifier, len(fragments)/batchSize+1)
	for i := 0; i < len(fragments); i += batchSize {
		end := i + batchSize
		if end > len(fragments) {
			end = len(fragments)
		}
		batch := []types.ObjectIdentifier{}
		for _, f := range fragments[i:end] {
			key := fmt.Sprintf("%s/%s", tid, f)
			batch = append(batch, types.ObjectIdentifier{Key: &key})
		}
		batches <- batch
	}
	close(batches)

	errChan := make(chan error, 1)
	var wg sync.WaitGroup

	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for batch := range batches {
				if err := s.deleteBatch(batch); err != nil {
					select {
					case errChan <- err:
					default:
					}
					return
				}
			}
		}()
	}

	wg.Wait()

	select {
	case err := <-errChan:
		return err
	default:
		return nil
	}
}

func (s *S3Driver) deleteBatch(batch []types.ObjectIdentifier) error {
	delInput := &s3.DeleteObjectsInput{
		Bucket: aws.String(s.config.Bucket),
		Delete: &types.Delete{
			Objects: batch,
			Quiet:   aws.Bool(false),
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	_, err := s.client.DeleteObjects(ctx, delInput)
	return err
}

func (s *S3Driver) GetFragment(streamTID, name string) (StreamFragment, error) {
	obj, err := s.client.GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String(s.config.Bucket),
		Key:    aws.String(s3FileKey(streamTID, name)),
	})
	if err != nil {
		return nil, err
	}
	return obj.Body, nil
}

func s3FileKey(tid, name string) string {
	return fmt.Sprintf("%v/%v", tid, name)
}
