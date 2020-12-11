package video

import (
	"fmt"
	"os"
	"path"

	"github.com/lbryio/transcoder/db"
	"github.com/lbryio/transcoder/encoder"
	"github.com/lbryio/transcoder/formats"
	"github.com/lbryio/transcoder/queue"
)

var tempStreamPath = path.Join(os.TempDir(), "transcoder")

func SpawnProcessing(videoPath string) {
	p := queue.StartPoller(queue.NewQueue(db.OpenDB("video.sqlite")))
	lib := NewLibrary(OpenDB())
	for t := range p.IncomingTasks() {
		c, err := ValidateIncomingVideo(t.URL)
		ll := logger.With("url", t.URL)
		if err != nil {
			if err == ErrChannelNotEnabled {
				p.RejectTask(t)
				ll.Errorw("validation failed", "err", err)
				continue
			}
			p.RejectTask(t)
			ll.Errorw("resolve failed", "err", err)
			continue
		}

		fh, _, err := c.Download()

		if err != nil {
			p.RejectTask(t)
			ll.Errorw("download failed", "err", err)
			continue
		}

		ll = ll.With("file", fh.Name())

		if err := fh.Close(); err != nil {
			p.RejectTask(t)
			ll.Errorw("closing downloaded file failed", "err", err)
			continue
		}

		streamPath := fmt.Sprintf("%v_%v", c.NormalizedName, c.sdHash[:6])
		out := path.Join(videoPath, streamPath)
		e, err := encoder.Encode(fh.Name(), out)
		if err != nil {
			ll.Errorw("encode failed", "err", err)
			p.ReleaseTask(t)
			continue
		}

		for i := range e {
			if i.GetProgress() >= 100.0 {
				ll.Infow("encode complete", "out", out)
				_, err := lib.Add(t.URL, t.SDHash, formats.TypeHLS, streamPath)
				if err != nil {
					logger.Errorw("adding to video library failed", "err", err)
				}
				p.CompleteTask(t)
			}
		}
		err = os.Remove(fh.Name())
		if err != nil {
			logger.Errorw("cleanup failed", "err", err)
		}
	}
}
