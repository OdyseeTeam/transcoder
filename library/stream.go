package library

import (
	"bytes"
	"crypto/sha1"
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
	"time"

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

	tidTimestampFormat = "2006-01-02T15:04"
)

var SDHashRe = regexp.MustCompile(`/([A-Za-z0-9]{96})/`)

type Stream struct {
	LocalPath     string
	RemoteStorage string
	Manifest      *Manifest
}

type Manifest struct {
	URL        string
	ChannelURL string `yaml:",omitempty"`
	SDHash     string

	// Generated attributes
	TID          string `yaml:",omitempty"`
	TranscodedAt time.Time
	Size         int64  `yaml:",omitempty"`
	Checksum     string `yaml:",omitempty"`

	Ladder ladder.Ladder `yaml:",omitempty,flow"`
}

type StreamFileLoader func(rootPath ...string) ([]byte, error)
type StreamFileProcessor func(data []byte, name string) error

type StreamWalker func(fi fs.FileInfo, fullPath, name string) error

func GetStreamHasher() hash.Hash {
	return sha512.New512_224()
}

func InitStream(dir string, remoteStorage string) *Stream {
	s := Stream{LocalPath: dir, RemoteStorage: remoteStorage}
	return &s
}

func (s *Stream) generateTID() string {
	h := sha1.New()
	h.Write([]byte(s.SDHash()))
	h.Write([]byte(s.Manifest.TranscodedAt.Format(tidTimestampFormat)))
	return hex.EncodeToString(h.Sum(nil))
}

// GenerateManifest needs to be called for newly initialized (transcoded) streams.
func (s *Stream) GenerateManifest(url, channel, sdHash string) error {
	var err error
	m := &Manifest{
		URL:          url,
		ChannelURL:   channel,
		SDHash:       sdHash,
		TranscodedAt: time.Now(),
	}
	s.Manifest = m
	s.Manifest.TID = s.generateTID()

	m.Checksum, err = s.generateChecksum()
	if err != nil {
		return errors.Wrap(err, "cannot calculate checksum")
	}
	m.Size, err = s.getSize()
	if err != nil {
		return errors.Wrap(err, "cannot calculate size")
	}

	d, err := yaml.Marshal(s.Manifest)
	if err != nil {
		return err
	}
	return os.WriteFile(path.Join(s.LocalPath, ManifestName), d, os.ModePerm)
}

func (s *Stream) Checksum() string {
	if s.Manifest == nil {
		return ""
	}
	return s.Manifest.Checksum
}

func (s *Stream) URL() string {
	if s.Manifest == nil {
		return ""
	}
	return s.Manifest.URL
}

func (s *Stream) TID() string {
	if s.Manifest == nil {
		return ""
	}
	return s.Manifest.TID
}
func (s *Stream) generateChecksum() (string, error) {
	hash := GetStreamHasher()
	err := WalkPlaylists(
		s.LocalPath,
		readFile,
		func(data []byte, _ string) error {
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

func (s *Stream) getSize() (int64, error) {
	var size int64
	err := s.Walk(func(fi fs.FileInfo, _, name string) error {
		if name == ManifestName {
			return nil
		}
		size += fi.Size()
		return nil
	})
	return size, err
}

func (s *Stream) Walk(walker StreamWalker) error {
	return godirwalk.Walk(s.LocalPath, &godirwalk.Options{
		Callback: func(fullPath string, de *godirwalk.Dirent) error {
			if de.IsDir() {
				if fullPath != s.LocalPath {
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

func (s *Stream) SDHash() string {
	if s.Manifest == nil {
		return ""
	}
	return s.Manifest.SDHash
}

func (s *Stream) Size() int64 {
	if s.Manifest == nil {
		return 0
	}
	return s.Manifest.Size
}

func (s *Stream) ChecksumValid(checksum string) bool {
	return checksum == s.Checksum()
}

func (s *Stream) ReadManifest() error {
	m := &Manifest{}
	d, err := os.ReadFile(path.Join(s.LocalPath, ManifestName))
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

func readFile(rootPath ...string) ([]byte, error) {
	return ioutil.ReadFile(path.Join(rootPath...))
}

// WalkPlaylists processes Local HLS stream, calling `loader` to load and `processor`
// for each master/child playlists and all the files they reference.
// `processor` with filename as second argument.
func WalkPlaylists(dir string, loader StreamFileLoader, processor StreamFileProcessor) error {
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

	data, err := doFile(dir, MasterPlaylistName)
	if err != nil {
		return err
	}

	pl, _, err := m3u8.DecodeFrom(data, true)
	if err != nil {
		return err
	}

	masterpl := pl.(*m3u8.MasterPlaylist)
	for _, plv := range masterpl.Variants {
		data, err := doFile(dir, plv.URI)
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
			_, err := doFile(dir, seg.URI)
			if err != nil {
				return err
			}
		}
	}
	return nil
}
