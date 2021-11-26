package storage

import (
	"io"
)

type StreamFragment interface {
	io.ReadCloser
}
