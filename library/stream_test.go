package library

import (
	"path"
	"testing"

	"github.com/Pallinder/go-randomdata"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitStream(t *testing.T) {
	dir := t.TempDir()
	sdHash := randomdata.Alphanumeric(96)
	PopulateHLSPlaylist(t, dir, sdHash)
	stream := InitStream(path.Join(dir, sdHash), "")
	err := stream.GenerateManifest(randomdata.SillyName(), randomdata.SillyName(), sdHash)
	require.NoError(t, err)

	assert.Greater(t, stream.Manifest.Size, int64(1000))
	assert.NotEmpty(t, stream.Manifest.Checksum)
}
