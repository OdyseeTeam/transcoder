package storage

import (
	"io/ioutil"
	"os"
	"path"
	"testing"

	"github.com/Pallinder/go-randomdata"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenLocalStream(t *testing.T) {
	dir := t.TempDir()
	sdHash := randomdata.Alphanumeric(96)
	PopulateHLSPlaylist(t, dir, sdHash)
	m := NewManifest(randomdata.SillyName(), randomdata.SillyName(), sdHash)
	ls, err := OpenLocalStream(path.Join(dir, sdHash), m)
	require.NoError(t, err)

	assert.Equal(t, path.Join(dir, sdHash), ls.Path)
	assert.Equal(t, m, *ls.Manifest)
	err = ls.FillManifest()
	require.NoError(t, err)
	assert.Greater(t, ls.Manifest.Size, int64(0))
	assert.NotEmpty(t, ls.Manifest.Checksum)

	ols, err := OpenLocalStream(path.Join(dir, sdHash))
	require.NoError(t, err)
	assert.Equal(t, ls.Manifest, ols.Manifest)
}

func TestManifest_Faulty(t *testing.T) {
	dir := t.TempDir()
	sdHash := randomdata.Alphanumeric(96)
	PopulateHLSPlaylist(t, dir, sdHash)
	ioutil.WriteFile(path.Join(dir, sdHash, ManifestName), []byte(`gibberish`), os.ModePerm)

	ols, err := OpenLocalStream(path.Join(dir, sdHash))
	require.NoError(t, err)
	err = ols.ReadManifest()
	assert.Error(t, err, `unmarshal errors`)
}
