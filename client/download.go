package client

import (
	"github.com/lbryio/transcoder/pkg/dispatcher"
	"github.com/lbryio/transcoder/video"
)

type downloader struct{}

func (d downloader) Do(t dispatcher.Task) error {
	dl := t.Payload.(Downloadable)
	err := dl.Init()
	if err != nil {
		if err == video.ErrChannelNotEnabled || err == ErrAlreadyDownloading {
			return nil
		}
		return err
	}
	err = dl.Download()
	if err != nil && err != ErrAlreadyDownloading {
		return err
	}
	// for p := range dl.Progress() {
	// 	if p.Stage == DownloadDone {
	// 		break
	// 	}
	// 	if p.Error != nil {
	// 		return err
	// 	}
	// }
	return nil
}

var pool = dispatcher.Start(10, downloader{})

func PoolDownload(d Downloadable) *dispatcher.Result {
	return pool.TryDispatch(d)
}
