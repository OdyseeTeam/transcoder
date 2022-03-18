package library

import (
	"fmt"
	"testing"

	"github.com/lbryio/transcoder/library/db"
	"github.com/lbryio/transcoder/pkg/logging/zapadapter"

	"github.com/stretchr/testify/suite"
)

type librarySuite struct {
	suite.Suite
	LibraryTestHelper
}

func TestLibrarySuite(t *testing.T) {
	suite.Run(t, new(librarySuite))
}

func (s *librarySuite) SetupTest() {
	s.Require().NoError(s.SetupLibraryDB())
}

func (s *librarySuite) TearDownTest() {
	s.Require().NoError(s.TearDownLibraryDB())
}

func (s *librarySuite) TestAddChannel() {
	lib := New(Config{DB: s.DB, Log: zapadapter.NewKV(nil)})
	c, err := lib.AddChannel("lbry://@specialoperationstest#3", "")
	s.Require().NoError(err)
	s.Equal("395b0f23dcd07212c3e956b697ba5ba89578ca54", c.ClaimID)
	s.Equal("lbry://@specialoperationstest#3", c.URL)
	s.Equal(db.ChannelPriorityNormal, c.Priority)
}

func (s *librarySuite) TestAddGetVideo() {
	var err error

	lib := New(Config{DB: s.DB, Storage: NewDummyStorage("storage1", "https://storage.host"), Log: zapadapter.NewKV(nil)})
	stream := GenerateDummyStream()

	url, err := lib.GetVideoURL(stream.SDHash())
	s.ErrorIs(err, ErrStreamNotFound)
	s.Empty(url)

	err = lib.AddRemoteStream(*stream)
	s.Require().NoError(err)

	url, err = lib.GetVideoURL(stream.SDHash())
	s.Require().NoError(err)
	s.Equal(fmt.Sprintf("remote://%s/%s/", stream.RemoteStorage, stream.Manifest.TID), url)
}
