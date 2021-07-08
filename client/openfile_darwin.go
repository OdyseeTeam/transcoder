// +build darwin

package client

import (
	"os"
)

func openFile(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
}
