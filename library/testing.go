package library

import (
	"database/sql"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"testing"
	"time"

	"github.com/Pallinder/go-randomdata"
	"github.com/lbryio/transcoder/library/db"
	"github.com/lbryio/transcoder/pkg/migrator"

	"github.com/stretchr/testify/require"
)

type LibraryTestHelper struct {
	DB        *sql.DB
	DBCleanup migrator.TestDBCleanup
}

func (h *LibraryTestHelper) SetupLibraryDB() error {
	db, dbCleanup, err := migrator.CreateTestDB(db.MigrationsFS)
	if err != nil {
		return err
	}
	h.DB = db
	h.DBCleanup = dbCleanup
	return nil
}

func (h *LibraryTestHelper) TearDownLibraryDB() error {
	db, dbCleanup, err := migrator.CreateTestDB(db.MigrationsFS)
	if err != nil {
		return err
	}
	h.DB = db
	h.DBCleanup = dbCleanup
	return nil
}

// PopulateHLSPlaylist generates a stream of 3131915 bytes in size, segments binary data will all be zeroes.
func PopulateHLSPlaylist(t *testing.T, dstPath, sdHash string) {
	err := os.MkdirAll(path.Join(dstPath, sdHash), os.ModePerm)
	require.NoError(t, err)

	srcPath, err := filepath.Abs("./testdata")
	require.NoError(t, err)

	err = WalkPlaylists(
		path.Join(srcPath, "dummy-stream"),
		func(rootPath ...string) ([]byte, error) {
			if path.Ext(rootPath[len(rootPath)-1]) == ".m3u8" {
				return ioutil.ReadFile(path.Join(rootPath...))
			}
			return make([]byte, 10000), nil
		},
		func(data []byte, name string) error {
			return ioutil.WriteFile(path.Join(dstPath, sdHash, name), data, os.ModePerm)
		},
	)
	require.NoError(t, err)
}

func GenerateDummyStream() *Stream {
	s := &Stream{
		LocalPath:     "/tmp/stream",
		RemoteStorage: "storage1",
		Manifest: &Manifest{
			URL:          randomdata.SillyName(),
			ChannelURL:   randomdata.SillyName(),
			SDHash:       randomdata.Alphanumeric(96),
			TranscodedAt: time.Now(),
			Size:         int64(randomdata.Number(10000, 5000000)),
		},
	}
	s.Manifest.TID = s.generateTID()
	return s
}
