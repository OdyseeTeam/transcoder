package storage

import (
	"os"
	"path"
)

type LocalStorage struct {
	path string
}

func Local(path string) LocalStorage {
	return LocalStorage{path}
}

func (s LocalStorage) Delete(sdHash string) error {
	return os.RemoveAll(path.Join(s.path, sdHash))
}

func (s LocalStorage) Path() string {
	return s.path
}
