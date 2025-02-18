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

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
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
	s3cfg := &aws.Config{
		Credentials:      credentials.NewStaticCredentials(cfg.Key, cfg.Secret, ""),
		Endpoint:         aws.String(cfg.Endpoint),
		Region:           aws.String(cfg.Region),
		S3ForcePathStyle: aws.Bool(true),
	}
	sess, err := session.NewSession(s3cfg)
	if err != nil {
		return nil, err
	}
	s := &S3Driver{
		config:  cfg,
		session: sess,
	}

	if cfg.CreateBucket {
		logger.Infow("creating s3 bucket", "name", cfg.Bucket)
		client := s3.New(sess)
		_, err := client.CreateBucket(&s3.CreateBucketInput{
			Bucket: aws.String(cfg.Bucket),
			ACL:    aws.String("public-read"),
		})
		if err != nil {
			if awsErr, ok := err.(awserr.Error); ok {
				if awsErr.Code() != "BucketAlreadyOwnedByYou" {
					return nil, err
				}
			}
		}
	}

	return s, nil
}

type S3Driver struct {
	config  config.S3Config
	session *session.Session
}

func (s *S3Driver) Name() string {
	return s.config.Name
}

func (s *S3Driver) GetURL(item string) string {
	return strings.Join([]string{s.config.Endpoint, s.config.Bucket, item}, "/")
}

func (s *S3Driver) Put(stream *library.Stream, overwrite bool) error {
	return s.PutWithContext(aws.BackgroundContext(), stream, overwrite)
}

func (s *S3Driver) PutWithContext(ctx context.Context, stream *library.Stream, overwrite bool) error {
	if !overwrite {
		dl := s3manager.NewDownloader(s.session)
		_, err := dl.Download(discardAt{}, &s3.GetObjectInput{
			Bucket: aws.String(s.config.Bucket),
			Key:    aws.String(s3FileKey(stream.TID(), library.MasterPlaylistName)),
		})
		if err == nil {
			return ErrStreamExists
		}

	}

	ul := s3manager.NewUploader(s.session)
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
			_, err = ul.UploadWithContext(ctx, &s3manager.UploadInput{
				Bucket:      aws.String(s.config.Bucket),
				Key:         aws.String(s3FileKey(stream.TID(), name)),
				ContentType: aws.String(ctype),
				Body:        f,
				ACL:         aws.String("public-read"),
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
	client := s3.New(s.session)
	bucket := aws.String(s.config.Bucket)
	objects, err := client.ListObjectsWithContext(ctx, &s3.ListObjectsInput{
		Bucket:  bucket,
		Prefix:  aws.String(tid),
		MaxKeys: aws.Int64(1000),
	})
	cancelFn()
	if err != nil {
		return err
	}

	delObjects := []*s3.ObjectIdentifier{}
	for _, o := range objects.Contents {
		delObjects = append(delObjects, &s3.ObjectIdentifier{Key: o.Key})
		if len(delObjects) >= 100 {
			delInput := &s3.DeleteObjectsInput{
				Bucket: bucket,
				Delete: &s3.Delete{
					Objects: delObjects,
					Quiet:   aws.Bool(false),
				},
			}
			ctx, cancelFn := context.WithTimeout(context.Background(), 600*time.Second)
			_, err = client.DeleteObjectsWithContext(ctx, delInput)
			cancelFn()
			if err != nil {
				return err
			}
			delObjects = []*s3.ObjectIdentifier{}
		}
	}

	if len(delObjects) > 0 {
		delInput := &s3.DeleteObjectsInput{
			Bucket: bucket,
			Delete: &s3.Delete{
				Objects: delObjects,
				Quiet:   aws.Bool(false),
			},
		}
		ctx, cancelFn := context.WithTimeout(context.Background(), 30*time.Second)
		_, err = client.DeleteObjectsWithContext(ctx, delInput)
		cancelFn()
		if err != nil {
			return err
		}
	}

	if *objects.IsTruncated {
		return s.Delete(tid)
	}

	return nil
}

func (s *S3Driver) DeleteFragments(tid string, fragments []string) error {
	workerCount := 120
	batchSize := 20
	client := s3.New(s.session)

	// Split objects into batches
	batches := make(chan []*s3.ObjectIdentifier, len(fragments)/batchSize+1)
	for i := 0; i < len(fragments); i += batchSize {
		end := i + batchSize
		if end > len(fragments) {
			end = len(fragments)
		}
		batch := []*s3.ObjectIdentifier{}
		for _, f := range fragments[i:end] {
			key := fmt.Sprintf("%s/%s", tid, f)
			batch = append(batch, &s3.ObjectIdentifier{Key: &key})
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
				if err := s.deleteBatch(client, batch); err != nil {
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

func (s *S3Driver) deleteBatch(client *s3.S3, batch []*s3.ObjectIdentifier) error {
	delInput := &s3.DeleteObjectsInput{
		Bucket: aws.String(s.config.Bucket),
		Delete: &s3.Delete{
			Objects: batch,
			Quiet:   aws.Bool(false),
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	_, err := client.DeleteObjectsWithContext(ctx, delInput)
	return err
}

func (s *S3Driver) GetFragment(streamTID, name string) (StreamFragment, error) {
	client := s3.New(s.session)
	obj, err := client.GetObject(&s3.GetObjectInput{
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
