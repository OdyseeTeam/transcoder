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
	"github.com/lbryio/transcoder/ladder"
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

	SkipChecksum = "SkipChecksumForThisStream"
)

var SDHashRe = regexp.MustCompile(`/([A-Za-z0-9]{96})/`)

type LocalStream struct {
	Path     string
	Manifest *Manifest
}

type RemoteStream struct {
	URL      string
	Manifest *Manifest
}

type TowerStreamCredentials struct {
	CallbackURL, Token string
}

type Manifest struct {
	URL        string
	ChannelURL string `yaml:",omitempty"`
	SDHash     string
	Size       int64  `yaml:",omitempty"`
	Checksum   string `yaml:",omitempty"`

	Ladder ladder.Ladder `yaml:",omitempty,flow"`
}

type StreamFileLoader func(rootPath ...string) ([]byte, error)
type StreamFileProcessor func(data []byte, name string) error

type StreamWalker func(fi fs.FileInfo, fullPath, name string) error

func GetStreamHasher() hash.Hash {
	return sha512.New512_224()
}

func OpenLocalStream(dir string, manifest ...Manifest) (*LocalStream, error) {
	s := LocalStream{Path: dir}
	if len(manifest) > 0 {
		s.Manifest = &manifest[0]
	} else if _, err := os.Stat(path.Join(dir, ManifestName)); !os.IsNotExist(err) {
		s.ReadManifest()
	}

	return &s, nil
}

// FillManifest needs to be created for newly created (transcoded) streams.
func (s *LocalStream) FillManifest() error {
	var err error
	if s.Manifest == nil {
		s.Manifest = &Manifest{}
	}
	m := s.Manifest
	if m.Checksum == "" {
		m.Checksum, err = s.getChecksum()
		if err != nil {
			return errors.Wrap(err, "cannot calculate checksum")
		}
	}
	if m.Size == 0 {
		m.Size, err = s.getSize()
		if err != nil {
			return errors.Wrap(err, "cannot calculate size")
		}
	}
	err = s.WriteManifest()
	if err != nil {
		return err
	}
	return nil
}

// func OpenLocalStream(path string) (*LocalStream, error) {
// 	info, err := os.Stat(path)
// 	if err != nil {
// 		return nil, err
// 	} else if !info.IsDir() {
// 		return nil, fmt.Errorf("%v is not a directory", path)
// 	}
// 	return &LocalStream{Path: path}, nil
// }

func (s *LocalStream) Checksum() string {
	if s.Manifest == nil {
		return ""
	}
	return s.Manifest.Checksum
}

func (s *LocalStream) getChecksum() (string, error) {
	hash := GetStreamHasher()
	err := s.WalkPlaylists(
		readFile,
		func(data []byte, name string) error {
			r := bytes.NewReader(data)
			_, err := io.Copy(hash, r)
			if err != nil {
				return err
			}
			return nil
		},
	)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func (s *LocalStream) getSize() (int64, error) {
	var size int64
	err := s.Walk(func(fi fs.FileInfo, fullPath, name string) error {
		if name == ManifestName {
			return nil
		}
		size += fi.Size()
		return nil
	})
	return size, err
}

func (s *LocalStream) Walk(walker StreamWalker) error {
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

// WalkPlaylists processes Local HLS stream, calling `loader` to load and `processor`
// for each master/child playlists and all the files they reference.
// `processor` with filename as second argument.
func (s *LocalStream) WalkPlaylists(loader StreamFileLoader, processor StreamFileProcessor) error {
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

	data, err := doFile(s.Path, MasterPlaylistName)
	if err != nil {
		return err
	}

	pl, _, err := m3u8.DecodeFrom(data, true)
	if err != nil {
		return err
	}

	masterpl := pl.(*m3u8.MasterPlaylist)
	for _, plv := range masterpl.Variants {
		data, err := doFile(s.Path, plv.URI)
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
			_, err := doFile(s.Path, seg.URI)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// Move just renames the stream directory, useful for adding to the library.
// Does not support cross-volume action yet.
func (s *LocalStream) Move(newDir string) error {
	return os.Rename(s.Path, path.Join(newDir, s.BasePath()))
}

func (s *LocalStream) BasePath() string {
	return path.Base(s.Path)
}

func (s *LocalStream) SDHash() string {
	if s.Manifest == nil {
		return ""
	}
	return s.Manifest.SDHash
}

func (s *LocalStream) Size() int64 {
	if s.Manifest == nil {
		return 0
	}
	return s.Manifest.Size
}

func (s *LocalStream) ChecksumValid(checksum string) bool {
	return checksum == s.Checksum()
}

func (s *LocalStream) WriteManifest() error {
	d, err := yaml.Marshal(s.Manifest)
	if err != nil {
		return err
	}
	return os.WriteFile(path.Join(s.Path, ManifestName), d, os.ModePerm)
}

func (s *LocalStream) ReadManifest() error {
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

func (s *RemoteStream) SDHash() string {
	if s.Manifest == nil {
		return ""
	}
	return s.Manifest.SDHash
}

func (s *RemoteStream) Size() int64 {
	if s.Manifest == nil {
		return 0
	}
	return s.Manifest.Size
}

func (s *RemoteStream) Checksum() string {
	if s.Manifest == nil {
		return ""
	}
	return s.Manifest.Checksum
}

func readFile(rootPath ...string) ([]byte, error) {
	return ioutil.ReadFile(path.Join(rootPath...))
}
