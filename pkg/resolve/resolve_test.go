package resolve

import (
	"os"
	"path"
	"testing"

	ljsonrpc "github.com/lbryio/lbry.go/v2/extras/jsonrpc"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testStreamURL = "@specialoperationstest#3/fear-of-death-inspirational#a"

func TestTranscodingRequestResolve(t *testing.T) {
	c, err := Resolve(testStreamURL)
	require.NoError(t, err)
	assert.Equal(t, "fear-of-death-inspirational", c.NormalizedName)
}

func TestTranscodingRequestResolveClaimID(t *testing.T) {
	claimID := "aa372cc164a4164ce9ea20741dd7331c28c0e044"
	c, err := Resolve(claimID)
	require.NoError(t, err)
	assert.Equal(t, "fear-of-death-inspirational", c.NormalizedName)
}

func TestTranscodingRequestResolveClaimID2(t *testing.T) {
	claimID := "11b6b88a7e31a6663c5b7734540f3784124e16f7"
	c, err := Resolve(claimID)
	require.NoError(t, err)
	assert.Equal(t, "weekly_webinar_april14", c.NormalizedName)
}

func TestTranscodingRequestResolveFailure(t *testing.T) {
	lbrytvClientOrig := lbrytvClient
	lbrytvClient = ljsonrpc.NewClient("http://localhost:2/")
	_, err := Resolve(testStreamURL)
	require.ErrorIs(t, err, ErrNetwork)
	lbrytvClient = lbrytvClientOrig
}

func TestTranscodingRequestDownload(t *testing.T) {
	dstPath := path.Join(os.TempDir(), "transcoder_test")
	c, err := Resolve(testStreamURL)
	require.NoError(t, err)

	r, err := ResolveStream(testStreamURL)
	require.NoError(t, err)

	assert.Equal(t, "395b0f23dcd07212c3e956b697ba5ba89578ca54", r.ChannelClaimID)
	assert.Equal(t, "lbry://@specialoperationstest:3", r.ChannelURI)

	f, n, err := r.Download(dstPath)
	f.Close()
	require.NoError(t, err)

	fi, err := os.Stat(f.Name())
	require.NoError(t, err)
	assert.EqualValues(t, c.Value.GetStream().GetSource().Size, fi.Size())
	assert.EqualValues(t, c.Value.GetStream().GetSource().Size, n)

	require.NoError(t, os.Remove(f.Name()))
	require.NoError(t, os.Remove(dstPath))

	_, err = os.Stat(dstPath)
	require.Error(t, err)
}
