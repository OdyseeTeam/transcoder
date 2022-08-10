package library

import (
	"bytes"
	"database/sql"
	"io"
	"os"
	"path"
	"path/filepath"
	"testing"
	"time"

	"github.com/Pallinder/go-randomdata"
	"github.com/lbryio/transcoder/ladder"
	"github.com/lbryio/transcoder/library/db"
	"github.com/lbryio/transcoder/pkg/migrator"

	"github.com/stretchr/testify/require"
)

var PopulatedHLSPlaylistFiles = []string{
	"master.m3u8", "s0_000000.ts", "s0_000001.ts", "s0_000002.ts", "s0_000003.ts", "s0_000004.ts", "s0_000005.ts", "s0_000006.ts", "s0_000007.ts",
	"s0_000008.ts", "s0_000009.ts", "s0_000010.ts", "s0_000011.ts", "s0_000012.ts", "s0_000013.ts", "s0_000014.ts", "s0_000015.ts", "s0_000016.ts",
	"s0_000017.ts", "s0_000018.ts", "s0_000019.ts", "s0_000020.ts", "s0_000021.ts", "s0_000022.ts", "s0_000023.ts", "s0_000024.ts", "s0_000025.ts",
	"s0_000026.ts", "s0_000027.ts", "s0_000028.ts", "s0_000029.ts", "s0_000030.ts", "s0_000031.ts", "s0_000032.ts", "s0_000033.ts", "s0_000034.ts",
	"s0_000035.ts", "s0_000036.ts", "s0_000037.ts", "s0_000038.ts", "s0_000039.ts", "s0_000040.ts", "s0_000041.ts", "s0_000042.ts", "s0_000043.ts",
	"s0_000044.ts", "s0_000045.ts", "s0_000046.ts", "s0_000047.ts", "s0_000048.ts", "s0_000049.ts", "s0_000050.ts", "s0_000051.ts", "s0_000052.ts",
	"s0_000053.ts", "s0_000054.ts", "s0_000055.ts", "s0_000056.ts", "s0_000057.ts", "s0_000058.ts", "s0_000059.ts", "s0_000060.ts", "s0_000061.ts",
	"s0_000062.ts", "s0_000063.ts", "s0_000064.ts", "s0_000065.ts", "s0_000066.ts", "s0_000067.ts", "s0_000068.ts", "s0_000069.ts", "s0_000070.ts",
	"s0_000071.ts", "s0_000072.ts", "s0_000073.ts", "s0_000074.ts", "s0_000075.ts", "s0_000076.ts", "s0_000077.ts", "s1_000000.ts", "s1_000001.ts",
	"s1_000002.ts", "s1_000003.ts", "s1_000004.ts", "s1_000005.ts", "s1_000006.ts", "s1_000007.ts", "s1_000008.ts", "s1_000009.ts", "s1_000010.ts",
	"s1_000011.ts", "s1_000012.ts", "s1_000013.ts", "s1_000014.ts", "s1_000015.ts", "s1_000016.ts", "s1_000017.ts", "s1_000018.ts", "s1_000019.ts",
	"s1_000020.ts", "s1_000021.ts", "s1_000022.ts", "s1_000023.ts", "s1_000024.ts", "s1_000025.ts", "s1_000026.ts", "s1_000027.ts", "s1_000028.ts",
	"s1_000029.ts", "s1_000030.ts", "s1_000031.ts", "s1_000032.ts", "s1_000033.ts", "s1_000034.ts", "s1_000035.ts", "s1_000036.ts", "s1_000037.ts",
	"s1_000038.ts", "s1_000039.ts", "s1_000040.ts", "s1_000041.ts", "s1_000042.ts", "s1_000043.ts", "s1_000044.ts", "s1_000045.ts", "s1_000046.ts",
	"s1_000047.ts", "s1_000048.ts", "s1_000049.ts", "s1_000050.ts", "s1_000051.ts", "s1_000052.ts", "s1_000053.ts", "s1_000054.ts", "s1_000055.ts",
	"s1_000056.ts", "s1_000057.ts", "s1_000058.ts", "s1_000059.ts", "s1_000060.ts", "s1_000061.ts", "s1_000062.ts", "s1_000063.ts", "s1_000064.ts",
	"s1_000065.ts", "s1_000066.ts", "s1_000067.ts", "s1_000068.ts", "s1_000069.ts", "s1_000070.ts", "s1_000071.ts", "s1_000072.ts", "s1_000073.ts",
	"s1_000074.ts", "s1_000075.ts", "s1_000076.ts", "s1_000077.ts", "s2_000000.ts", "s2_000001.ts", "s2_000002.ts", "s2_000003.ts", "s2_000004.ts",
	"s2_000005.ts", "s2_000006.ts", "s2_000007.ts", "s2_000008.ts", "s2_000009.ts", "s2_000010.ts", "s2_000011.ts", "s2_000012.ts", "s2_000013.ts",
	"s2_000014.ts", "s2_000015.ts", "s2_000016.ts", "s2_000017.ts", "s2_000018.ts", "s2_000019.ts", "s2_000020.ts", "s2_000021.ts", "s2_000022.ts",
	"s2_000023.ts", "s2_000024.ts", "s2_000025.ts", "s2_000026.ts", "s2_000027.ts", "s2_000028.ts", "s2_000029.ts", "s2_000030.ts", "s2_000031.ts",
	"s2_000032.ts", "s2_000033.ts", "s2_000034.ts", "s2_000035.ts", "s2_000036.ts", "s2_000037.ts", "s2_000038.ts", "s2_000039.ts", "s2_000040.ts",
	"s2_000041.ts", "s2_000042.ts", "s2_000043.ts", "s2_000044.ts", "s2_000045.ts", "s2_000046.ts", "s2_000047.ts", "s2_000048.ts", "s2_000049.ts",
	"s2_000050.ts", "s2_000051.ts", "s2_000052.ts", "s2_000053.ts", "s2_000054.ts", "s2_000055.ts", "s2_000056.ts", "s2_000057.ts", "s2_000058.ts",
	"s2_000059.ts", "s2_000060.ts", "s2_000061.ts", "s2_000062.ts", "s2_000063.ts", "s2_000064.ts", "s2_000065.ts", "s2_000066.ts", "s2_000067.ts",
	"s2_000068.ts", "s2_000069.ts", "s2_000070.ts", "s2_000071.ts", "s2_000072.ts", "s2_000073.ts", "s2_000074.ts", "s2_000075.ts", "s2_000076.ts",
	"s2_000077.ts", "s3_000000.ts", "s3_000001.ts", "s3_000002.ts", "s3_000003.ts", "s3_000004.ts", "s3_000005.ts", "s3_000006.ts", "s3_000007.ts",
	"s3_000008.ts", "s3_000009.ts", "s3_000010.ts", "s3_000011.ts", "s3_000012.ts", "s3_000013.ts", "s3_000014.ts", "s3_000015.ts", "s3_000016.ts",
	"s3_000017.ts", "s3_000018.ts", "s3_000019.ts", "s3_000020.ts", "s3_000021.ts", "s3_000022.ts", "s3_000023.ts", "s3_000024.ts", "s3_000025.ts",
	"s3_000026.ts", "s3_000027.ts", "s3_000028.ts", "s3_000029.ts", "s3_000030.ts", "s3_000031.ts", "s3_000032.ts", "s3_000033.ts", "s3_000034.ts",
	"s3_000035.ts", "s3_000036.ts", "s3_000037.ts", "s3_000038.ts", "s3_000039.ts", "s3_000040.ts", "s3_000041.ts", "s3_000042.ts", "s3_000043.ts",
	"s3_000044.ts", "s3_000045.ts", "s3_000046.ts", "s3_000047.ts", "s3_000048.ts", "s3_000049.ts", "s3_000050.ts", "s3_000051.ts", "s3_000052.ts",
	"s3_000053.ts", "s3_000054.ts", "s3_000055.ts", "s3_000056.ts", "s3_000057.ts", "s3_000058.ts", "s3_000059.ts", "s3_000060.ts", "s3_000061.ts",
	"s3_000062.ts", "s3_000063.ts", "s3_000064.ts", "s3_000065.ts", "s3_000066.ts", "s3_000067.ts", "s3_000068.ts", "s3_000069.ts", "s3_000070.ts",
	"s3_000071.ts", "s3_000072.ts", "s3_000073.ts", "s3_000074.ts", "s3_000075.ts", "s3_000076.ts", "s3_000077.ts",
	"stream_0.m3u8", "stream_1.m3u8", "stream_2.m3u8", "stream_3.m3u8"}

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
	t.Helper()
	err := os.MkdirAll(path.Join(dstPath, sdHash), os.ModePerm)
	require.NoError(t, err)

	srcPath, err := filepath.Abs("./testdata")
	require.NoError(t, err)

	err = WalkStream(
		path.Join(srcPath, "dummy-stream"),
		func(rootPath ...string) (io.ReadCloser, error) {
			if path.Ext(rootPath[len(rootPath)-1]) == ".m3u8" {
				return os.Open(path.Join(rootPath...))
			}
			return io.ReadCloser(io.NopCloser(bytes.NewReader(make([]byte, 10000)))), nil
		},
		func(name string, r io.ReadCloser) error {
			f, err := os.Create(path.Join(dstPath, sdHash, name))
			if err != nil {
				return err
			}
			_, err = io.Copy(f, r)
			return err
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
			Ladder:       ladder.Default,
		},
	}
	s.Manifest.TID = s.generateTID()
	return s
}
