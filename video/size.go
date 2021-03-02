package video

import (
	"github.com/c2h5oh/datasize"
)

func StringToSize(s string) uint64 {
	var size datasize.ByteSize
	err := size.UnmarshalText([]byte(s))
	if err != nil {
		logger.Warn(err)
	}
	return size.Bytes()
}
