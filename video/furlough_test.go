package video

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/lbryio/transcoder/db"
	"github.com/lbryio/transcoder/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

func TestTailSizeables(t *testing.T) {
	vs := []*Video{
		{Size: 10000, LastAccessed: sql.NullTime{Time: time.Now().Add(-25 * time.Hour)}},
		{Size: 20000, LastAccessed: sql.NullTime{Time: time.Now().Add(-24 * time.Hour)}},
		{Size: 50000, LastAccessed: sql.NullTime{Time: time.Now().Add(-1 * time.Hour)}},
		{Size: 30000, LastAccessed: sql.NullTime{Time: time.Now().Add(-30 * time.Hour)}},
		{Size: 20000, LastAccessed: sql.NullTime{Time: time.Now().Add(-23 * time.Hour)}},
	}
	vsog := make([]*Video, 5)
	copy(vsog, vs)

	removed := []*Video{}

	totalSize, furloughedSize, err := tailVideos(vs, 75000, func(v *Video) error { removed = append(removed, v); return nil })
	require.NoError(t, err)
	fmt.Println(removed)
	assert.EqualValues(t, 130000, totalSize)
	assert.EqualValues(t, 60000, furloughedSize)
	assert.Equal(t, vsog[3], removed[0])
	assert.Equal(t, vsog[0], removed[1])
	assert.Equal(t, vsog[1], removed[2])
}

type FurloughSuite struct {
	suite.Suite
	db *db.DB
}

func TestFurloughSuite(t *testing.T) {
	suite.Run(t, new(FurloughSuite))
}

func (s *FurloughSuite) SetupSuite() {
	rand.Seed(time.Now().UnixNano())
}

func (s *FurloughSuite) SetupTest() {
	s.db = db.OpenTestDB()
	s.Require().NoError(s.db.MigrateUp(InitialMigration))
}

func (s FurloughSuite) TestFurloughVideos() {
	var totalSize, sizeToKeep, sizeRemoteOnly uint64

	dummyls := storage.Dummy()
	dummyrs := storage.Dummy()
	lib := NewLibrary(Configure().
		LocalStorage(dummyls).
		RemoteStorage(dummyrs).
		DB(s.db),
	)

	for i := range [100]int{} {
		v, err := lib.Add(AddParams{
			SDHash: randomString(96),
			URL:    "lbry://" + randomString(32),
			Path:   randomString(96),
			Size:   int64(1000000 + rand.Intn(1000000)),
		})
		s.Require().NoError(err)
		if i%3 != 0 {
			lib.UpdateRemotePath(v.SDHash, "https://s3.wasabi.com/"+v.SDHash)
		}
		_, err = lib.queries.db.ExecContext(
			context.Background(),
			"update video set last_accessed = $2 where sd_hash = $1",
			time.Now().AddDate(0, 0, -rand.Intn(30)),
			v.SDHash,
		)
		s.Require().NoError(err)
	}

	r := lib.queries.db.QueryRowContext(context.Background(), `select sum(size) from video where remote_path != ""`)
	err := r.Scan(&totalSize)
	s.Require().NoError(err)

	sizeToKeep = uint64(rand.Int63n(1000000 * 50))
	ts, fs, err := FurloughVideos(lib, sizeToKeep)
	s.NoError(err)
	s.Equal(totalSize, ts)
	s.InDelta(sizeToKeep, ts-fs, 2000000)

	r = lib.queries.db.QueryRowContext(context.Background(), `select sum(size) from video where path = "" and remote_path != ""`)
	err = r.Scan(&sizeRemoteOnly)
	s.Require().NoError(err)
	s.Equal(sizeRemoteOnly, fs)

	s.GreaterOrEqual(len(dummyls.Ops), 10)
	s.Equal(0, len(dummyrs.Ops))
}

func (s FurloughSuite) TestRetireVideos() {
	var totalSize, sizeToKeep, sizeRemoteOnly uint64
	var initialCount, afterCount int64

	dummyls := storage.Dummy()
	dummyrs := storage.Dummy()
	lib := NewLibrary(Configure().
		LocalStorage(dummyls).
		RemoteStorage(dummyrs).
		DB(s.db),
	)

	for i := range [100]int{} {
		v, err := lib.Add(AddParams{
			SDHash: randomString(96),
			URL:    "lbry://" + randomString(32),
			Size:   int64(1000000 + rand.Intn(1000000)),
		})
		s.Require().NoError(err)
		s.Require().NoError(lib.UpdateRemotePath(v.SDHash, "https://s3.wasabi.com/"+v.SDHash))
		if i%3 == 0 {
			// Mark these as furloughed
			s.Require().NoError(lib.queries.UpdatePath(context.Background(), v.SDHash, ""))
		}
		_, err = lib.queries.db.ExecContext(
			context.Background(),
			"update video set last_accessed = $2 where sd_hash = $1",
			time.Now().AddDate(0, 0, -rand.Intn(30)),
			v.SDHash,
		)
		s.Require().NoError(err)
	}

	r := lib.queries.db.QueryRowContext(context.Background(), `select sum(size) from video where path == ""`)
	err := r.Scan(&totalSize)
	s.Require().NoError(err)

	r = lib.queries.db.QueryRowContext(context.Background(), `select count(*) from video`)
	err = r.Scan(&initialCount)
	s.Require().NoError(err)

	sizeToKeep = uint64(rand.Int63n(1000000 * 50))
	ts, fs, err := RetireVideos(lib, sizeToKeep)
	s.NoError(err)
	s.Equal(totalSize, ts)
	s.InDelta(sizeToKeep, ts-fs, 2000000)

	r = lib.queries.db.QueryRowContext(context.Background(), `select sum(size) from video where path = "" and remote_path != ""`)
	err = r.Scan(&sizeRemoteOnly)
	s.Require().NoError(err)
	s.InDelta(sizeToKeep, sizeRemoteOnly, 2000000)

	r = lib.queries.db.QueryRowContext(context.Background(), `select count(*) from video`)
	err = r.Scan(&afterCount)
	s.Require().NoError(err)

	s.Equal(0, len(dummyls.Ops))
	s.GreaterOrEqual(len(dummyrs.Ops), 10)
	s.EqualValues(initialCount-afterCount, len(dummyrs.Ops))
}

func randomString(n int) string {
	var letter = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")

	b := make([]rune, n)
	for i := range b {
		b[i] = letter[rand.Intn(len(letter))]
	}
	return string(b)
}
