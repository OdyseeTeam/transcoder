package storage

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"time"

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

type S3Configuration struct {
	endpoint, region, accessKey, secretKey, bucket string
	disableSSL                                     bool
}

func S3Configure() *S3Configuration {
	return &S3Configuration{region: "us-east-1"}
}

// Endpoint ...
func (c *S3Configuration) Endpoint(e string) *S3Configuration {
	c.endpoint = e
	return c
}

// Region ...
func (c *S3Configuration) Region(r string) *S3Configuration {
	c.region = r
	return c
}

// Bucket ...
func (c *S3Configuration) Bucket(b string) *S3Configuration {
	c.bucket = b
	return c
}

// DisableSSL ...
func (c *S3Configuration) DisableSSL() *S3Configuration {
	c.disableSSL = true
	return c
}

// Credentials set access key and secret key for accessing S3 bucket.
func (c *S3Configuration) Credentials(accessKey, secretKey string) *S3Configuration {
	c.accessKey = accessKey
	c.secretKey = secretKey
	return c
}

func InitS3Driver(cfg *S3Configuration) (*S3Driver, error) {
	s3cfg := &aws.Config{
		Credentials:      credentials.NewStaticCredentials(cfg.accessKey, cfg.secretKey, ""),
		Endpoint:         aws.String(cfg.endpoint),
		Region:           aws.String(cfg.region),
		S3ForcePathStyle: aws.Bool(true),
		DisableSSL:       aws.Bool(cfg.disableSSL),
	}
	sess := session.New(s3cfg)
	s := &S3Driver{
		S3Configuration: cfg,
		session:         sess,
	}

	client := s3.New(sess)

	_, err := client.CreateBucket(&s3.CreateBucketInput{
		Bucket: aws.String(s.bucket),
		ACL:    aws.String("public-read"),
	})
	if err != nil {
		if awsErr, ok := err.(awserr.Error); ok {
			if awsErr.Code() != "BucketAlreadyOwnedByYou" {
				return nil, err
			}
		}
	}
	return s, nil
}

type S3Driver struct {
	*S3Configuration
	session *session.Session
}

func (s *S3Driver) Put(ls *LocalStream, overwrite bool) (*RemoteStream, error) {
	return s.PutWithContext(aws.BackgroundContext(), ls, overwrite)
}

func (s *S3Driver) PutWithContext(ctx context.Context, ls *LocalStream, overwrite bool) (*RemoteStream, error) {
	if !overwrite {
		dl := s3manager.NewDownloader(s.session)
		_, err := dl.Download(discardAt{}, &s3.GetObjectInput{
			Bucket: aws.String(s.bucket),
			Key:    aws.String(s3Key(ls.SDHash(), MasterPlaylistName)),
		})
		if err == nil {
			return &RemoteStream{URL: ls.SDHash(), Manifest: ls.Manifest}, ErrStreamExists
		}

	}

	ul := s3manager.NewUploader(s.session)
	err := ls.Walk(
		func(fi fs.FileInfo, fullPath, name string) error {
			var ctype string
			f, err := os.Open(fullPath)
			if err != nil {
				return err
			}
			defer f.Close()

			switch path.Ext(name) {
			case PlaylistExt:
				ctype = PlaylistContentType
			case FragmentExt:
				ctype = FragmentContentType
			default:
				ctype = "text/plain"
			}
			logger.Debugw("uploading", "key", s3Key(ls.SDHash(), name), "ctype", ctype, "size", fi.Size(), "bucket", s.bucket)
			_, err = ul.UploadWithContext(ctx, &s3manager.UploadInput{
				Bucket:      aws.String(s.bucket),
				Key:         aws.String(s3Key(ls.SDHash(), name)),
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

	return &RemoteStream{URL: ls.SDHash(), Manifest: ls.Manifest}, err
}

func (s *S3Driver) Delete(sdHash string) error {
	ctx, cancelFn := context.WithTimeout(context.Background(), 600*time.Second)
	client := s3.New(s.session)
	bucket := aws.String(s.bucket)
	objects, err := client.ListObjectsWithContext(ctx, &s3.ListObjectsInput{
		Bucket:  bucket,
		Prefix:  aws.String(sdHash),
		MaxKeys: aws.Int64(500),
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
		ctx, cancelFn := context.WithTimeout(context.Background(), 600*time.Second)
		_, err = client.DeleteObjectsWithContext(ctx, delInput)
		cancelFn()
		if err != nil {
			return err
		}
	}

	if *objects.IsTruncated {
		return s.Delete(sdHash)
	}

	return nil
}

func (s *S3Driver) Get(sdHash string) (*LocalStream, error) {
	return nil, nil
}

func (s *S3Driver) GetFragment(sdHash, name string) (StreamFragment, error) {
	client := s3.New(s.session)
	obj, err := client.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s3Key(sdHash, name)),
	})
	if err != nil {
		return nil, err
	}
	return obj.Body, nil
}

func s3Key(sdHash, name string) string {
	return fmt.Sprintf("%v/%v", sdHash, name)
}
