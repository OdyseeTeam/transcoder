// +build linux

package client

import (
	"os"
	"syscall"
)

func directCopy(path string) (*os.File, error) {
	f, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC|syscall.O_DIRECT, 0666)
	if err != nil {
		return 0, err
	}

	df, err := directio.New(f)
	if err != nil {
		return 0, err
	}
	defer df.Flush()
	defer f.Close()
	return io.Copy(df, from)
}
