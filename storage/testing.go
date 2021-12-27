package storage

import (
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	OpDelete = iota
	OpGetFragment
	OpPut
)

type StorageOp struct {
	Op     int
	SDHash string
}

type DummyStorage struct {
	LocalStorage
	Ops []StorageOp
}

func Dummy() *DummyStorage {
	return &DummyStorage{LocalStorage: LocalStorage{"/tmp/dummy_storage"}, Ops: []StorageOp{}}
}

func (s *DummyStorage) Delete(sdHash string) error {
	s.Ops = append(s.Ops, StorageOp{OpDelete, sdHash})
	return nil
}

func (s *DummyStorage) GetFragment(sdHash, name string) (StreamFragment, error) {
	s.Ops = append(s.Ops, StorageOp{OpGetFragment, sdHash})
	return nil, nil
}

func (s *DummyStorage) Put(ls *LocalStream, _ bool) (*RemoteStream, error) {
	s.Ops = append(s.Ops, StorageOp{OpGetFragment, ls.SDHash()})
	return &RemoteStream{URL: "http://dummy/url"}, nil
}

// PopulateHLSPlaylist generates a stream of 3131915 bytes in size, segments binary data will all be zeroes.
func PopulateHLSPlaylist(t *testing.T, dstPath, sdHash string) {
	err := os.MkdirAll(path.Join(dstPath, sdHash), os.ModePerm)
	require.NoError(t, err)

	srcPath, err := filepath.Abs("./testdata")
	require.NoError(t, err)
	dummyls, err := OpenLocalStream(path.Join(srcPath, "dummy-stream"), Manifest{Checksum: SkipChecksum})
	require.NoError(t, err)
	err = dummyls.WalkPlaylists(
		func(rootPath ...string) ([]byte, error) {
			if path.Ext(rootPath[len(rootPath)-1]) == ".m3u8" {
				return ioutil.ReadFile(path.Join(rootPath...))
			}
			return make([]byte, 10000), nil
		},
		func(data []byte, name string) error {
			return ioutil.WriteFile(path.Join(dstPath, sdHash, name), data, os.ModePerm)
		},
	)
	require.NoError(t, err)
}
