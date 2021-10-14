package retriever

import (
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRetrieve(t *testing.T) {
	outPath := path.Join(os.TempDir(), "retriever_test")
	defer os.RemoveAll(outPath)
	url := "@specialoperationstest#3/fear-of-death-inspirational#a"

	p := NewPool(10)

	r := p.Retrieve(url, outPath)
	rv := <-r.Value()
	require.NoError(t, r.Error)
	dr := rv.(DownloadResult)

	err := dr.File.Close()
	require.NoError(t, err)

	fi, err := os.Stat(dr.File.Name())
	require.NoError(t, err)
	assert.EqualValues(t, 11814366, fi.Size())
	assert.EqualValues(t, 11814366, dr.Size)
}
