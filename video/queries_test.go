package video

import (
	"database/sql"
	"math/rand"
	"testing"
	"time"

	"github.com/lbryio/transcoder/db"
	"github.com/lbryio/transcoder/formats"
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
	s.db.MigrateUp(InitialMigration)
}

func (s *LibrarySuite) TearDownTest() {
	s.db.Cleanup()
}

func (s *LibrarySuite) TestVideoAdd() {
	lib := NewLibrary(s.db)
	params := AddParams{
		URL:    "what",
		SDHash: "string",
		Type:   formats.TypeHLS,
		Path:   "/tmp/test",
	}
	video, err := lib.Add(params.URL, params.SDHash, params.Type, params.Path)
	s.Require().NoError(err)
	s.Equal(params.URL, video.URL)
	s.EqualValues(params.SDHash, video.SDHash)
	s.EqualValues(params.Type, video.Type)
	s.EqualValues(params.Path, video.Path)

	video, err = lib.Add(params.URL, params.SDHash, params.Type, params.Path)
	s.Require().Error(err, "UNIQUE constraint failed")
}

func (s *LibrarySuite) TestVideoGet() {
	lib := NewLibrary(s.db)
	params := AddParams{
		URL:    "what",
		SDHash: "string",
		Type:   formats.TypeHLS,
		Path:   "/tmp/test",
	}
	video, err := lib.Get(params.SDHash)
	s.Error(err, sql.ErrNoRows)
	s.Nil(video)

	_, err = lib.Add(params.URL, params.SDHash, params.Type, params.Path)
	s.Require().NoError(err)

	video, err = lib.Get(params.SDHash)
	s.EqualValues(params.URL, video.URL)
	s.EqualValues(params.SDHash, video.SDHash)
	s.EqualValues(params.Type, video.Type)
	s.EqualValues(params.Path, video.Path)
}
