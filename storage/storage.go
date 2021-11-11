package storage

import (
	"io"
)

type StreamFragment interface {
	io.ReadCloser
}

type RemoteDriver interface {
	Put(stream *LocalStream) (*RemoteStream, error)
	Delete(sdHash string) error
	GetFragment(sdHash, name string) (StreamFragment, error)
}

type LocalDriver interface {
	New(sdHash string) *LocalStream
	Open(sdHash string) (*LocalStream, error)
	Delete(sdHash string) error
	Path() string
}
