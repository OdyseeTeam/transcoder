package storage

import (
	"bytes"
	"encoding/hex"
	"io"
	"io/ioutil"
	"path"

	"crypto/sha512"

	"github.com/grafov/m3u8"
	"github.com/pkg/errors"
)

const (
	MasterPlaylistName  = "master.m3u8"
	PlaylistExt         = ".m3u8"
	FragmentExt         = ".ts"
	PlaylistContentType = "application/x-mpegurl"
	FragmentContentType = "video/mp2t"
)

type RemoteStream struct {
	url      string
	checksum string
	size     int64
}

type LocalStream struct {
	rootPath string
	sdHash   string
	size     int64
	checksum string
}

type StreamFileLoader func(rootPath ...string) ([]byte, error)
type StreamFileProcessor func(data []byte, name string) error

// func (s LocalStream) Path() string {
// 	return s.path
// }

func (s LocalStream) FullPath() string {
	return path.Join(s.rootPath, s.sdHash)
}

func (s LocalStream) LastPath() string {
	return s.sdHash
}

func (s LocalStream) Checksum() string {
	return s.checksum
}

func (s LocalStream) Size() int64 {
	return s.size
}

func (s *LocalStream) ReadMeta() error {
	var err error
	s.checksum, s.size, err = s.calculateChecksum()
	return err
}

func (s LocalStream) calculateChecksum() (string, int64, error) {
	var size int64

	hash := sha512.New512_224()

	err := s.Dive(
		readFile,
		func(data []byte, name string) error {
			r := bytes.NewReader(data)
			n, err := io.Copy(hash, r)
			if err != nil {
				return err
			}
			size += n
			return nil
		},
	)
	if err != nil {
		return "", size, err
	}
	return hex.EncodeToString(hash.Sum(nil)), size, nil
}

// func (s LocalStream) Validate() error {
// 	if cs != s.checksum {
// 		return fmt.Errorf("checksum mismatch: %v != %v", s.checksum, cs)
// 	}
// 	return nil
// }

// Dive processes Local HLS stream, calling `loader` to load and `processor`
// for each master/child playlists and all the files they reference.
// `processor` with filename as second argument.
func (s LocalStream) Dive(loader StreamFileLoader, processor StreamFileProcessor) error {
	doFile := func(path ...string) (io.Reader, error) {
		data, err := loader(path...)
		if err != nil {
			return nil, err
		}

		err = processor(data, path[len(path)-1])
		if err != nil {
			return nil, errors.Wrapf(err, `error processing stream item "%v"`, path[len(path)-1])
		}
		return bytes.NewReader(data), err
	}

	data, err := doFile(s.FullPath(), MasterPlaylistName)
	if err != nil {
		return err
	}

	pl, _, err := m3u8.DecodeFrom(data, true)
	if err != nil {
		return err
	}

	masterpl := pl.(*m3u8.MasterPlaylist)
	for _, plv := range masterpl.Variants {
		data, err := doFile(s.FullPath(), plv.URI)
		if err != nil {
			return err
		}

		p, _, err := m3u8.DecodeFrom(data, true)
		if err != nil {
			return err
		}
		mediapl := p.(*m3u8.MediaPlaylist)

		for _, seg := range mediapl.Segments {
			if seg == nil {
				continue
			}
			_, err := doFile(s.FullPath(), seg.URI)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func readFile(rootPath ...string) ([]byte, error) {
	return ioutil.ReadFile(path.Join(rootPath...))
}

func (s RemoteStream) URL() string {
	return s.url
}
