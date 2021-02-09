package video

import (
	"database/sql"
	"math/rand"
	"testing"
	"time"

	"github.com/lbryio/transcoder/db"
	"github.com/lbryio/transcoder/formats"
	"github.com/lbryio/transcoder/storage"
	"github.com/stretchr/testify/suite"
)

var testDB *db.DB

func ns(s string) sql.NullString {
	return sql.NullString{String: s, Valid: true}
}

type LibrarySuite struct {
	suite.Suite
	db *db.DB
}

func TestLibrarySuite(t *testing.T) {
	suite.Run(t, new(LibrarySuite))
}

func (s *LibrarySuite) SetupSuite() {
	rand.Seed(time.Now().UTC().UnixNano())
}

func (s *LibrarySuite) SetupTest() {
	s.db = db.OpenTestDB()
	s.Require().NoError(s.db.MigrateUp(InitialMigration))
}

func (s *LibrarySuite) TestVideoAdd() {
	lib := NewLibrary(Configure().LocalStorage(storage.Local("/tmp/test")).DB(s.db))
	params := AddParams{
		URL:     "what",
		SDHash:  "string",
		Type:    formats.TypeHLS,
		Path:    "string",
		Channel: "@specialoperationstest#3",
	}
	video, err := lib.Add(params)
	s.Require().NoError(err)
	s.Equal(params.URL, video.URL)
	s.EqualValues(params.SDHash, video.SDHash)
	s.EqualValues(params.Type, video.Type)
	s.EqualValues(params.Path, video.Path)
	s.Equal(sql.NullTime{}, video.LastAccessed)

	video, err = lib.Add(params)
	s.Require().Error(err, "UNIQUE constraint failed")
}

func (s *LibrarySuite) TestVideoGet() {
	lib := NewLibrary(Configure().LocalStorage(storage.Local("/tmp/test")).DB(s.db))
	params := AddParams{
		URL:     "what",
		SDHash:  "string",
		Type:    formats.TypeHLS,
		Path:    "string",
		Channel: "@specialoperationstest#3",
	}
	video, err := lib.Get(params.SDHash)
	s.Error(err, sql.ErrNoRows)
	s.Nil(video)

	_, err = lib.Add(params)

	s.Require().NoError(err)

	video, err = lib.Get(params.SDHash)
	s.Require().NoError(err)
	s.EqualValues(params.URL, video.URL)
	s.EqualValues(params.SDHash, video.SDHash)
	s.EqualValues(params.Type, video.Type)
	s.EqualValues(params.Path, video.Path)
	s.EqualValues(params.Channel, video.Channel)
	s.LessOrEqual((time.Since(video.LastAccessed.Time)).Seconds(), float64(1))
	s.EqualValues(1, video.AccessCount)

	video, err = lib.Get(params.SDHash)
	s.Require().NoError(err)
	s.EqualValues(2, video.AccessCount)
}
