package storage

import (
	"io/ioutil"
	"math/rand"
	"os"
	"path"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/draganm/miniotest"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/suite"
)

type s3suite struct {
	suite.Suite
	cleanup func() error
	addr    string
	local   LocalStorage
	sdHash  string
}

func Tests3suite(t *testing.T) {
	suite.Run(t, new(s3suite))
}

func (s *s3suite) SetupSuite() {
	var err error

	rand.Seed(time.Now().UTC().UnixNano())
	s.local = Local("storage_s3_test")
	s.sdHash = randomString(96)

	s.addr, s.cleanup, err = miniotest.StartEmbedded()
	s.Require().NoError(err)
}

func (s *s3suite) SetupTest() {
	s.populateHLSPlaylist()
}

func (s *s3suite) TestPutDelete() {
	s3drv, err := InitS3Driver(
		S3Configure().
			Endpoint(s.addr).
			Region("us-east-1").
			Credentials("minioadmin", "minioadmin").
			Bucket("storage-s3-test").
			DisableSSL(),
	)
	s.Require().NoError(err)

	stream, err := s.local.Open(s.sdHash)
	s.Require().NoError(err)

	rstream, err := s3drv.Put(stream)
	s.Require().NoError(err)
	s.Equal(s.sdHash, rstream.URL())

	p, err := s3drv.GetFragment(s.sdHash, MasterPlaylistName)
	s.Require().NoError(err)
	s.Require().NotNil(p)

	err = s3drv.Delete(s.sdHash)
	s.Require().NoError(err)

	deletedPieces := []string{"", MasterPlaylistName, "stream_0.m3u8", "stream_1.m3u8", "stream_2.m3u8", "stream_3.m3u8"}
	for _, n := range deletedPieces {
		p, err := s3drv.GetFragment(s.sdHash, n)
		awsErr := err.(awserr.Error)
		s.Equal("NoSuchKey", awsErr.Code())
		s.Nil(p)
	}
}

func (s *s3suite) TearDownSuite() {
	s.NoError(s.cleanup())
	s.NoError(os.RemoveAll(s.local.path))
}

func (s *s3suite) populateHLSPlaylist() {
	stream := s.local.New(s.sdHash)
	err := os.MkdirAll(stream.FullPath(), os.ModePerm)
	s.Require().NoError(err)

	incomingStorage := Local(".")

	// plPath, _ := filepath.Abs("./testdata")
	// ls := InitLocalStream(plPath, s.sdHash)
	// ls.path = plPath

	ls, err := incomingStorage.Open("testdata")
	s.Require().NoError(err)
	err = ls.Dive(
		func(rootPath ...string) ([]byte, error) {
			if path.Ext(rootPath[len(rootPath)-1]) == ".m3u8" {
				d, err := ioutil.ReadFile(path.Join(rootPath...))
				if err != nil {
					return nil, errors.Wrap(err, "error reading path")
				}
				return d, nil
			}
			return make([]byte, 10000), nil
		},
		func(data []byte, name string) error {

			err := ioutil.WriteFile(path.Join(s.local.path, s.sdHash, name), data, os.ModePerm)
			if err != nil {
				return errors.Wrap(err, "error writing path")
			}
			return nil

		},
	)
	s.Require().NoError(err)
}

func randomString(n int) string {
	var letter = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")

	b := make([]rune, n)
	for i := range b {
		b[i] = letter[rand.Intn(len(letter))]
	}
	return string(b)
}
