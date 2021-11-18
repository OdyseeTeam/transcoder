package storage

import (
	"io/ioutil"
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManifest(t *testing.T) {
	dir := t.TempDir()
	sdHash := randomString(96)
	PopulateHLSPlaylist(t, dir, sdHash)
	m := &Manifest{
		URL:    "lbry://what",
		SDHash: sdHash,
		Tower: &TowerStreamCredentials{
			CallbackURL: randomString(255),
			Token:       randomString(32),
		},
	}
	ls, err := InitLocalStream(path.Join(dir, sdHash), m)
	require.NoError(t, err)

	ols, err := OpenLocalStream(path.Join(dir, sdHash))
	require.NoError(t, err)
	err = ols.ReadManifest()
	require.NoError(t, err)
	assert.Equal(t, ls.Manifest, ols.Manifest)
}

func TestManifest_Faulty(t *testing.T) {
	dir := t.TempDir()
	sdHash := randomString(96)
	PopulateHLSPlaylist(t, dir, sdHash)
	ioutil.WriteFile(path.Join(dir, sdHash, ManifestName), []byte(`gibberish`), os.ModePerm)

	ols, err := OpenLocalStream(path.Join(dir, sdHash))
	require.NoError(t, err)
	err = ols.ReadManifest()
	assert.Error(t, err, `unmarshal errors`)
}
