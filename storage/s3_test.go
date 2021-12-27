package storage

import (
	"math/rand"
	"path"
	"testing"
	"time"

	"github.com/Pallinder/go-randomdata"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/draganm/miniotest"
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
	PopulateHLSPlaylist(s.T(), s.streamsPath, s.sdHash)
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

	ls, err := OpenLocalStream(path.Join(s.streamsPath, s.sdHash), Manifest{SDHash: s.sdHash})
	s.Require().NoError(err)
	err = ls.FillManifest()
	s.Require().NoError(err)

	rs, err := s3drv.Put(ls, false)
	s.Require().NoError(err)
	s.Equal(ls.SDHash(), rs.URL)

	sf, err := s3drv.GetFragment(ls.SDHash(), MasterPlaylistName)
	s.Require().NoError(err)
	s.Require().NotNil(sf)

	mf, err := s3drv.GetFragment(ls.SDHash(), ManifestName)
	s.Require().NoError(err)
	s.Require().NotNil(mf)

	rs2, err := s3drv.Put(ls, false)
	s.Equal(rs2.URL, rs.URL)
	s.Equal(ErrStreamExists, err)

	rs3, err := s3drv.Put(ls, true)
	s.Equal(rs3.URL, rs.URL)
	s.NoError(err)

	err = s3drv.Delete(ls.SDHash())
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
}
