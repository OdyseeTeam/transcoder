package library

import (
	"context"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/odyseeteam/transcoder/library/db"
	"github.com/odyseeteam/transcoder/pkg/logging/zapadapter"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type maintenanceSuite struct {
	suite.Suite
	LibraryTestHelper
}

const (
	OpDelete = iota
	OpGetFragment
	OpPut
)

type StorageOp struct {
	Op  int
	TID string
}

type DummyStorage struct {
	Ops            []StorageOp
	ReturnName     string
	name, endpoint string
}

func NewDummyStorage(name, endpoint string) *DummyStorage {
	return &DummyStorage{
		Ops:      []StorageOp{},
		name:     name,
		endpoint: endpoint,
	}
}

func (s *DummyStorage) Delete(streamTID string) error {
	s.Ops = append(s.Ops, StorageOp{OpDelete, streamTID})
	return nil
}

func (s *DummyStorage) Put(stream *Stream, _ bool) error {
	s.Ops = append(s.Ops, StorageOp{OpGetFragment, stream.TID()})
	return nil
}

func (s *DummyStorage) Name() string {
	return s.name
}

func (s *DummyStorage) GetURL(tid string) string {
	return fmt.Sprintf("%s/%s", s.endpoint, tid)
}

func (s *maintenanceSuite) SetupTest() {
	s.Require().NoError(s.SetupLibraryDB())
}

func (s *maintenanceSuite) TearDownTest() {
	s.Require().NoError(s.TearDownLibraryDB())
}

func TestTailSizeables(t *testing.T) {
	vs := []db.Video{
		{Size: 10000, AccessedAt: time.Now().Add(-25 * time.Hour)},
		{Size: 20000, AccessedAt: time.Now().Add(-24 * time.Hour)},
		{Size: 50000, AccessedAt: time.Now().Add(-1 * time.Hour)},
		{Size: 30000, AccessedAt: time.Now().Add(-30 * time.Hour)},
		{Size: 20000, AccessedAt: time.Now().Add(-23 * time.Hour)},
	}
	vsog := make([]db.Video, 5)
	copy(vsog, vs)

	removed := []db.Video{}

	totalSize, furloughedSize, err := tailVideos(vs, 75000, func(v db.Video) error { removed = append(removed, v); return nil })
	require.NoError(t, err)
	assert.EqualValues(t, 130000, totalSize)
	assert.EqualValues(t, 60000, furloughedSize)
	assert.Equal(t, vsog[3], removed[0])
	assert.Equal(t, vsog[0], removed[1])
	assert.Equal(t, vsog[1], removed[2])
}

func TestMaintenanceSuite(t *testing.T) {
	suite.Run(t, new(maintenanceSuite))
}

func (s *maintenanceSuite) TestRetireVideos() {
	var totalSize, sizeToKeep, sizeRemote uint64
	var initialCount, afterCount int64

	dummyStorage := NewDummyStorage("", "")
	lib := New(Config{DB: s.DB, Storage: dummyStorage, Log: zapadapter.NewKV(nil)})

	for i := range [100]int{} {
		stream := GenerateDummyStream()
		err := lib.AddRemoteStream(*stream)
		s.Require().NoError(err)

		if i%3 == 0 {
			// Mark these as
			_, err = s.DB.ExecContext(
				context.Background(),
				"UPDATE videos SET storage = $2 where tid = $1",
				"storage2",
				stream.TID(),
			)
			s.Require().NoError(err)
		}
		_, err = s.DB.ExecContext(
			context.Background(),
			"UPDATE videos SET accessed_at = $2 where tid = $1",
			stream.TID(),
			time.Now().AddDate(0, 0, -rand.Intn(30)),
		) // #nosec G404
		s.Require().NoError(err)
	}

	r := s.DB.QueryRowContext(context.Background(), `select sum(size) from videos`)
	err := r.Scan(&totalSize)
	s.Require().NoError(err)

	r = s.DB.QueryRowContext(context.Background(), `select count(*) from videos`)
	err = r.Scan(&initialCount)
	s.Require().NoError(err)

	sizeToKeep = uint64(rand.Int63n(1000000 * 50)) // #nosec G404
	totalSizeAfterRetire, retiredSize, err := lib.RetireVideos("storage1", sizeToKeep)
	s.NoError(err)
	s.Equal(totalSize, totalSizeAfterRetire)
	s.InDelta(sizeToKeep, totalSizeAfterRetire-retiredSize, 5000000)

	r = s.DB.QueryRowContext(context.Background(), `select sum(size) from videos`)
	err = r.Scan(&sizeRemote)
	s.Require().NoError(err)
	s.InDelta(sizeToKeep, sizeRemote, 5000000)

	r = s.DB.QueryRowContext(context.Background(), `select count(*) from videos`)
	err = r.Scan(&afterCount)
	s.Require().NoError(err)

	s.GreaterOrEqual(len(dummyStorage.Ops), 10)
	s.EqualValues(initialCount-afterCount, len(dummyStorage.Ops))
}
