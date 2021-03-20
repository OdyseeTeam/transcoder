package client

import (
	"github.com/lbryio/transcoder/pkg/dispatcher"
)

type downloader struct{}

func (d downloader) Do(t dispatcher.Task) error {
	dl := t.Payload.(Downloadable)
	err := dl.Download()
	if err != nil && err != ErrAlreadyDownloading {
		return err
	}
	return nil
}

var pool = dispatcher.Start(10, downloader{})

func PoolDownload(d Downloadable) *dispatcher.Result {
	return pool.TryDispatch(d)
}
