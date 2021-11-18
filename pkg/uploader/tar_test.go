package uploader

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"testing"

	"github.com/lbryio/transcoder/pkg/logging"
	"github.com/lbryio/transcoder/pkg/logging/zapadapter"
	"github.com/lbryio/transcoder/storage"

	"github.com/Pallinder/go-randomdata"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPackUnpackStream(t *testing.T) {
	log = zapadapter.NewKV(logging.Create("tar", logging.Dev).Desugar())
	sdHash := randomdata.Alphanumeric(64)
	p, err := ioutil.TempDir("", "")
	unDir := path.Join(p, "out")
	tarPath := path.Join(p, fmt.Sprintf("%v.tar", sdHash))
	require.NoError(t, err)
	defer os.RemoveAll(p)

	storage.PopulateHLSPlaylist(t, p, sdHash)

	ls, err := storage.OpenLocalStream(path.Join(p, sdHash))
	require.NoError(t, err)
	csum, err := packStream(ls, tarPath)
	require.NoError(t, err)
	require.NotNil(t, csum)

	tarFile, err := os.Open(tarPath)
	require.NoError(t, err)
	defer tarFile.Close()
	csum2, err := unpackStream(tarFile, unDir)
	require.NoError(t, err)
	assert.Equal(t, csum, csum2)

	size, err := verifyPathChecksum(unDir, csum)
	require.NoError(t, err)
	assert.EqualValues(t, 3131915, size, "%s is zero size", unDir)
}
