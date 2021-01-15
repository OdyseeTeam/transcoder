package claim

import (
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClaimResolve(t *testing.T) {
	url := "lbry://@specialoperationstest#3/fear-of-death-inspirational#a"
	c, err := Resolve(url)
	require.NoError(t, err)
	assert.Equal(t, "fear-of-death-inspirational", c.NormalizedName)
}

func TestClaimDownload(t *testing.T) {
	url := "lbry://@specialoperationstest#3/fear-of-death-inspirational#a"
	c, err := Resolve(url)
	require.NoError(t, err)
	f, n, err := c.Download(path.Join(os.TempDir(), "transcoder_test"))
	f.Close()
	require.NoError(t, err)

	fi, err := os.Stat(f.Name())
	require.NoError(t, err)
	assert.Equal(t, int64(c.Value.GetStream().GetSource().Size), fi.Size())
	assert.Equal(t, int64(c.Value.GetStream().GetSource().Size), n)

	os.Remove(f.Name())
}
