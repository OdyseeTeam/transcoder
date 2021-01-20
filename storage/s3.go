package storage

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
)

type S3Configuration struct {
	bucket                                 string
	endpoint, region, accessKey, secretKey string
}

func S3ConfigureWasabi() *S3Configuration {
	return &S3Configuration{
		endpoint: "https://s3.wasabisys.com",
		region:   "us-east-1",
	}
}

// Endpoint ...
func (c *S3Configuration) Endpoint(e string) *S3Configuration {
	c.endpoint = e
	return c
}

// Credentials set access key and secret key for accessing S3 bucket.
func (c *S3Configuration) Credentials(accessKey, secretKey string) *S3Configuration {
	c.accessKey = accessKey
	c.secretKey = secretKey
	return c
}

func NewS3Storage(cfg *S3Configuration) *S3Storage {
	s3cfg := &aws.Config{
		Credentials:      credentials.NewStaticCredentials(cfg.accessKey, cfg.secretKey, ""),
		Endpoint:         aws.String(cfg.endpoint),
		Region:           aws.String(cfg.region),
		S3ForcePathStyle: aws.Bool(true),
	}
	sess := session.New(s3cfg)
	ss := &S3Storage{
		uploader: s3manager.NewUploader(sess),
	}
	return ss
}

type S3Storage struct {
	*S3Configuration
	uploader *s3manager.Uploader
}

func (s *S3Storage) Put(sdHash, name string, stream RawStream) error {
	_, err := s.uploader.Upload(&s3manager.UploadInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(s3Key(sdHash, name)),
		ContentType: aws.String("mmmm/aaa"),
		Body:        stream.file,
	})
	return err
}

func s3Key(sdHash, name string) string {
	return fmt.Sprintf("%v/%v", sdHash, name)
}
