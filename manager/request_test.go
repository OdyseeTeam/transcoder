package manager

import (
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTranscodingRequestResolve(t *testing.T) {
	url := "@specialoperationstest#3/fear-of-death-inspirational#a"
	c, err := Resolve(url)
	require.NoError(t, err)
	assert.Equal(t, "fear-of-death-inspirational", c.NormalizedName)
}

func TestTranscodingRequestDownload(t *testing.T) {
	url := "@specialoperationstest#3/fear-of-death-inspirational#a"
	c, err := Resolve(url)
	require.NoError(t, err)

	r, err := ResolveRequest(url)
	require.NoError(t, err)
	f, n, err := r.Download(path.Join(os.TempDir(), "transcoder_test"))
	f.Close()
	require.NoError(t, err)

	fi, err := os.Stat(f.Name())
	require.NoError(t, err)
	assert.Equal(t, int64(c.Value.GetStream().GetSource().Size), fi.Size())
	assert.Equal(t, int64(c.Value.GetStream().GetSource().Size), n)

	os.Remove(f.Name())
}
