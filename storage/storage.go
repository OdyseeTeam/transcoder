package storage

import (
	"io"
)

type RawStream struct {
	file io.ReadCloser
}

type Storage interface {
	Put(sdHash, name string, stream RawStream) error
	Get(sdHash string) (*RawStream, error)
}
