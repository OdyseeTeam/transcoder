package video

import (
	"fmt"
	"os"
	"path"

	"github.com/lbryio/transcoder/encoder"
	"github.com/lbryio/transcoder/formats"
	"github.com/lbryio/transcoder/pkg/timer"
	"github.com/lbryio/transcoder/queue"
)

func SpawnProcessing(videoPath string, q *queue.Queue, lib *Library, p *queue.Poller) {
	logger.Info("started video processor")
	defer logger.Info("quit video processor")
	for t := range p.IncomingTasks() {
		ll := logger.With("url", t.URL, "task_id", t.ID)
		ll.Infow("incoming task")

		c, err := ValidateIncomingVideo(t.URL)
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

		sfh, _, err := c.Download(path.Join(os.TempDir(), "transcoder", "streams"))

		if err != nil {
			ll.Errorw("download failed", "err", err)
			tErr := p.ReleaseTask(t)
			if tErr != nil {
				ll.Errorw("error releasing task", "tid", t.ID, "err", tErr)
			}
			continue
		}

		ll = ll.With("file", sfh.Name())

		if err := sfh.Close(); err != nil {
			p.ReleaseTask(t)
			ll.Errorw("closing downloaded file failed", "err", err)
			continue
		}

		tmr := timer.Start()

		streamPath := fmt.Sprintf("%v_%v", c.NormalizedName, c.SDHash[:6])
		out := path.Join(videoPath, streamPath)

		enc, err := encoder.NewEncoder(sfh.Name(), out)
		if err != nil {
			p.ReleaseTask(t)
			ll.Errorw("encoding failure", "err", err)
			continue
		}

		ll.Infow("starting encoding")
		e, err := enc.Encode()
		if err != nil {
			p.RejectTask(t)
			ll.Errorw("encoding failure", "err", err)
			continue
		}

		for i := range e {
			ll.Debugw("encoding", "progress", fmt.Sprintf("%.2f", i.GetProgress()))
			if i.GetProgress() >= 99.9 {
				p.CompleteTask(t)
				ll.Infow(
					"encoding complete",
					"out", out,
					"seconds_spent", tmr.String(),
					"duration", enc.Meta.Format.Duration,
					"bitrate", enc.Meta.Format.GetBitRate(),
				)
				break
			}
		}
		_, err = lib.Add(t.URL, t.SDHash, formats.TypeHLS, streamPath)
		if err != nil {
			logger.Errorw("adding to video library failed", "err", err)
		}
		err = os.Remove(sfh.Name())
		if err != nil {
			logger.Errorw("cleanup failed", "err", err)
		}
	}
}
