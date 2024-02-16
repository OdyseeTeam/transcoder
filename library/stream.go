package library

import (
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"io/fs"
	"os"
	"path"
	"regexp"
	"sort"
	"time"

	"github.com/lbryio/transcoder/ladder"

	"github.com/karrick/godirwalk"
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
	LocalPath     string `json:"local_path,omitempty"`
	RemoteStorage string `json:"remote_storage,omitempty"`
	Manifest      *Manifest
}

type Manifest struct {
	URL        string
	ChannelURL string `yaml:",omitempty" json:"channel_url"`
	SDHash     string

	// Meta attributes
	TranscodedBy string    `yaml:"transcoded_by,omitempty" json:"transcoded_by"`
	TranscodedAt time.Time `yaml:"transcoded_at,omitempty" json:"transcoded_at"`
	Version      string    `yaml:",omitempty"`

	// Auto-filled attributes
	TID      string `yaml:",omitempty"`
	Size     int64  `yaml:",omitempty"`
	Checksum string `yaml:",omitempty"`

	Ladder ladder.Ladder `yaml:",omitempty"`
	Files  []string      `yaml:",omitempty"`
}

type StreamWalker func(fi fs.FileInfo, fullPath, name string) error

func WithTimestamp(ts time.Time) func(*Manifest) {
	return func(m *Manifest) {
		m.TranscodedAt = ts
	}
}

func WithWorkerName(n string) func(*Manifest) {
	return func(m *Manifest) {
		m.TranscodedBy = n
	}
}

func WithVersion(v string) func(*Manifest) {
	return func(m *Manifest) {
		m.Version = v
	}
}

func GetStreamHasher() hash.Hash {
	return sha512.New512_224()
}

func InitStream(dir string, remoteStorage string) *Stream {
	s := Stream{LocalPath: dir, RemoteStorage: remoteStorage}
	return &s
}

func (s *Stream) generateTID() string {
	h := sha256.New()
	h.Write([]byte(s.SDHash()))
	return hex.EncodeToString(h.Sum([]byte(s.Manifest.TranscodedAt.Format(tidTimestampFormat))))
}

// GenerateManifest needs to be called for newly initialized (transcoded) streams.
func (s *Stream) GenerateManifest(url, channel, sdHash string, manifestFuncs ...func(*Manifest)) error {
	var err error
	m := &Manifest{
		URL:        url,
		ChannelURL: channel,
		SDHash:     sdHash,
	}

	for _, f := range manifestFuncs {
		f(m)
	}

	s.Manifest = m
	s.Manifest.TID = s.generateTID()

	m.Files, m.Size, err = s.getFileList()
	if err != nil {
		return errors.Wrap(err, "cannot calculate size")
	}
	m.Checksum, err = s.generateChecksum()
	if err != nil {
		return errors.Wrap(err, "cannot calculate checksum")
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
	err := WalkStream(
		s.LocalPath,
		openFile,
		func(_ string, r io.ReadCloser) error {
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

func (s *Stream) getFileList() ([]string, int64, error) {
	var size int64
	fl := []string{}
	err := s.Walk(func(fi fs.FileInfo, _, name string) error {
		if name == ManifestName {
			return nil
		}
		size += fi.Size()
		fl = append(fl, name)
		return nil
	})
	sort.Strings(fl)
	return fl, size, err
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

func openFile(rootPath ...string) (io.ReadCloser, error) {
	return os.Open(path.Join(rootPath...))
}
