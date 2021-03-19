package client

import (
	"github.com/lbryio/transcoder/pkg/dispatcher"
	"github.com/lbryio/transcoder/video"
)

type downloader struct{}

func (d downloader) Do(t dispatcher.Task) error {
	dl := t.Payload.(Downloadable)
	err := dl.Download()
	if err != nil {
		if err != video.ErrChannelNotEnabled || err != ErrAlreadyDownloading {
			return err
		}
		return nil
	}
	for p := range dl.Progress() {
		if p.Done {
			break
		}
		if p.Error != nil {
			return err
		}
	}
	return nil
}

var pool = dispatcher.Start(10, downloader{})

func PoolDownload(d Downloadable) chan bool {
	return pool.Dispatch(d)
}
