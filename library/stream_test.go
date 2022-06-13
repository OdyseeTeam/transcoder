package library

import (
	"path"
	"sort"
	"testing"
	"time"

	"github.com/Pallinder/go-randomdata"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStream(t *testing.T) {
	dir := t.TempDir()
	sdHash := randomdata.Alphanumeric(96)
	PopulateHLSPlaylist(t, dir, sdHash)

	t.Run("InitStream", func(t *testing.T) {
		t.Parallel()

		stream := InitStream(path.Join(dir, sdHash), "")
		err := stream.GenerateManifest(randomdata.SillyName(), randomdata.SillyName(), sdHash)
		require.NoError(t, err)

		assert.Greater(t, stream.Manifest.Size, int64(1000))
		assert.NotEmpty(t, stream.Manifest.Checksum)
		sort.Strings(PopulatedHLSPlaylistFiles)
		assert.Equal(t, PopulatedHLSPlaylistFiles, stream.Manifest.Files)
	})

	t.Run("GenerateTID", func(t *testing.T) {
		t.Parallel()
		ts := time.Now()

		stream1 := InitStream(path.Join(dir, sdHash), "")
		require.NoError(t,
			stream1.GenerateManifest(randomdata.SillyName(), randomdata.SillyName(), sdHash, WithTimestamp(ts)),
		)

		stream2 := InitStream(path.Join(dir, sdHash), "")
		require.NoError(t,
			stream2.GenerateManifest(stream1.URL(), stream1.Manifest.ChannelURL, stream1.SDHash(), WithTimestamp(ts)),
		)
		err := stream1.GenerateManifest(randomdata.SillyName(), randomdata.SillyName(), sdHash, WithTimestamp(ts))
		require.NoError(t, err)

		assert.Equal(t, stream1.Manifest.TID, stream2.Manifest.TID)
	})

	t.Run("WithManifestOptions", func(t *testing.T) {
		t.Parallel()
		ts := time.Now()

		workerName := randomdata.SillyName()
		version := randomdata.BoundedDigits(5, 0, 99999)
		url := randomdata.SillyName()
		channelURL := randomdata.SillyName()

		stream := InitStream(path.Join(dir, sdHash), "")
		require.NoError(t,
			stream.GenerateManifest(
				url, channelURL, sdHash,
				WithTimestamp(ts),
				WithVersion(version),
				WithWorkerName(workerName),
			),
		)

		assert.Equal(t, ts, stream.Manifest.TranscodedAt)
		assert.Equal(t, workerName, stream.Manifest.TranscodedBy)
		assert.Equal(t, version, stream.Manifest.Version)
		assert.Equal(t, url, stream.Manifest.URL)
		assert.Equal(t, channelURL, stream.Manifest.ChannelURL)
		assert.Equal(t, sdHash, stream.Manifest.SDHash)
	})
}
