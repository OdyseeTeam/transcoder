package storage

import (
	"math/rand"
	"path"
	"testing"
	"time"

	"github.com/Pallinder/go-randomdata"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/draganm/miniotest"
	"github.com/lbryio/transcoder/library"
	"github.com/stretchr/testify/suite"
)

type s3suite struct {
	suite.Suite
	cleanup     func() error
	addr        string
	sdHash      string
	streamsPath string
}

func TestS3suite(t *testing.T) {
	suite.Run(t, new(s3suite))
}

func (s *s3suite) SetupSuite() {
	var err error

	rand.Seed(time.Now().UTC().UnixNano())

	s.addr, s.cleanup, err = miniotest.StartEmbedded()
	s.Require().NoError(err)
}

func (s *s3suite) SetupTest() {
	s.streamsPath = s.T().TempDir()
	s.sdHash = randomdata.Alphanumeric(96)
	library.PopulateHLSPlaylist(s.T(), s.streamsPath, s.sdHash)
}

func (s *s3suite) TestPutDelete() {
	s3drv, err := InitS3Driver(
		S3Configure().
			Name("test").
			Endpoint(s.addr).
			Region("us-east-1").
			Credentials("minioadmin", "minioadmin").
			Bucket("storage-s3-test").
			DisableSSL(),
	)
	s.Require().NoError(err)

	stream := library.InitStream(path.Join(s.streamsPath, s.sdHash), "")
	err = stream.GenerateManifest("url", "channel", s.sdHash)
	s.Require().NoError(err)

	err = s3drv.Put(stream, false)
	s.Require().NoError(err)

	sf, err := s3drv.GetFragment(stream.TID(), library.MasterPlaylistName)
	s.Require().NoError(err)
	s.Require().NotNil(sf)

	mf, err := s3drv.GetFragment(stream.TID(), library.ManifestName)
	s.Require().NoError(err)
	s.Require().NotNil(mf)

	err = s3drv.Put(stream, false)
	s.ErrorIs(err, ErrStreamExists)

	err = s3drv.Put(stream, true)
	s.NoError(err)

	err = s3drv.Delete(stream.TID())
	s.Require().NoError(err)

	deletedPieces := []string{"", library.MasterPlaylistName, "stream_0.m3u8", "stream_1.m3u8", "stream_2.m3u8", "stream_3.m3u8"}
	for _, n := range deletedPieces {
		p, err := s3drv.GetFragment(stream.TID(), n)
		s.NotNil(err)
		awsErr := err.(awserr.Error)
		s.Equal("NoSuchKey", awsErr.Code())
		s.Nil(p)
	}
}

func (s *s3suite) TearDownSuite() {
	s.NoError(s.cleanup())
}
