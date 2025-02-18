package library

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/OdyseeTeam/transcoder/library/db"
	"github.com/OdyseeTeam/transcoder/pkg/logging/zapadapter"

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

	dummyStorage := NewDummyStorage("dummy1", "https://storage.host")
	lib := New(Config{DB: s.DB, Storages: map[string]Storage{dummyStorage.Name(): dummyStorage}, Log: zapadapter.NewKV(nil)})
	newStream := GenerateDummyStream(dummyStorage)

	url, err := lib.GetVideoURL(newStream.SDHash())
	s.ErrorIs(err, ErrStreamNotFound)
	s.Empty(url)

	err = lib.AddRemoteStream(*newStream)
	s.Require().NoError(err)

	url, err = lib.GetVideoURL(newStream.SDHash())
	s.Require().NoError(err)
	s.Equal(fmt.Sprintf("remote://%s/%s/", newStream.RemoteStorage, newStream.Manifest.TID), url)

	v, err := lib.GetVideo(newStream.SDHash())
	s.Require().NoError(err)
	s.EqualValues(1, v.AccessCount.Int32)
	s.GreaterOrEqual(2, int(time.Since(v.AccessedAt).Seconds()))
	m := &Manifest{}
	err = json.Unmarshal(v.Manifest.RawMessage, m)
	s.Require().NoError(err)
	m.TranscodedAt = time.Time{}
	newStream.Manifest.TranscodedAt = time.Time{}
	s.EqualValues(m, newStream.Manifest)
}
