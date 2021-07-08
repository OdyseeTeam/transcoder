// +build linux

package client

import (
	"os"
	"syscall"
)

func openFile(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC|syscall.O_DIRECT, 0666)
}
