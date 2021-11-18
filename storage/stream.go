package storage

import (
	"bytes"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"io/fs"
	"io/ioutil"
	"os"
	"path"
	"regexp"

	"github.com/grafov/m3u8"
	"github.com/karrick/godirwalk"
	"github.com/lbryio/transcoder/formats"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
)

const (
	MasterPlaylistName  = "master.m3u8"
	PlaylistExt         = ".m3u8"
	FragmentExt         = ".ts"
	ManifestName        = ".manifest"
	PlaylistContentType = "application/x-mpegurl"
	FragmentContentType = "video/mp2t"
)

var SDHashRe = regexp.MustCompile(`/([A-Za-z0-9]{96})/`)

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

type LightLocalStream struct {
	Path     string
	SDHash   string
	Size     int64
	Checksum []byte
	Manifest *Manifest
}

type TowerStreamCredentials struct {
	CallbackURL, Token string
}

type Manifest struct {
	URL      string
	SDHash   string
	Size     int64  `yaml:",omitempty"`
	Checksum string `yaml:",omitempty"`

	Formats []formats.Format        `yaml:",omitempty,flow"`
	Tower   *TowerStreamCredentials `yaml:",omitempty,flow"`
}

type StreamFileLoader func(rootPath ...string) ([]byte, error)
type StreamFileProcessor func(data []byte, name string) error

type StreamWalker func(fi fs.FileInfo, fullPath, name string) error

func GetHash() hash.Hash {
	return sha512.New512_224()
}

func InitLocalStream(path string, m *Manifest) (*LightLocalStream, error) {
	s, err := OpenLocalStream(path)
	if err != nil {
		return nil, err
	}
	err = s.WriteManifest(m)
	if err != nil {
		return nil, err
	}
	s.Manifest = m
	return s, nil
}

func OpenLocalStream(path string) (*LightLocalStream, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	} else if !info.IsDir() {
		return nil, fmt.Errorf("%v is not a directory", path)
	}
	return &LightLocalStream{Path: path}, nil
}

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

	hash := GetHash()
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

func (s *LightLocalStream) ChecksumString() string {
	return hex.EncodeToString(s.Checksum)
}

func (s *LightLocalStream) ChecksumValid(checksum []byte) bool {
	return bytes.Equal(checksum, s.Checksum)
}

func (s *LightLocalStream) Walk(walker StreamWalker) error {
	return godirwalk.Walk(s.Path, &godirwalk.Options{
		Callback: func(fullPath string, de *godirwalk.Dirent) error {
			if de.IsDir() {
				if fullPath != s.Path {
					return fmt.Errorf("%v is a directory while only files are expected here", fullPath)
				}
				return nil
			}
			fi, err := os.Stat(fullPath)
			if err != nil {
				return err
			}
			return walker(fi, fullPath, de.Name())
		},
	})
}

func (s *LightLocalStream) WriteManifest(m *Manifest) error {
	d, err := yaml.Marshal(m)
	if err != nil {
		return err
	}
	return os.WriteFile(path.Join(s.Path, ManifestName), d, os.ModePerm)
}

func (s *LightLocalStream) ReadManifest() error {
	m := &Manifest{}
	d, err := os.ReadFile(path.Join(s.Path, ManifestName))
	if err != nil {
		return errors.Wrap(err, "cannot read manifest file")
	}
	err = yaml.Unmarshal(d, m)
	if err != nil {
		return errors.Wrap(err, "cannot unmarshal manifest")
	}
	s.Manifest = m
	return nil
}
