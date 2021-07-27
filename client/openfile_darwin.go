// +build darwin

package client

import (
	"io"
	"os"
)

func directCopy(dst string, from io.Reader) (int64, error) {
	f, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
	if err != nil {
		return 0, err
	}

	defer f.Close()
	return io.Copy(f, from)
}
