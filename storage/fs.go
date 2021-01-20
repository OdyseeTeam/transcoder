package storage

import (
	"io"
	"os"
	"path"
)

type FSConfiguration struct {
	path string
}

type FSStorage struct {
	*FSConfiguration
}

func FSConfigure() *FSConfiguration {
	return &FSConfiguration{}
}

// Endpoint ...
func (c *FSConfiguration) Path(p string) *FSConfiguration {
	c.path = p
	return c
}

func (s *FSStorage) Put(sdHash, name string, stream RawStream) error {
	f, err := os.Open(path.Join(s.path, sdHash, name))
	if err != nil {
		return err
	}
	_, err = io.Copy(f, stream.file)
	return err
}
