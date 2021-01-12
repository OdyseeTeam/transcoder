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
		ll := logger.Named("worker").With("url", t.URL, "task_id", t.ID)
		ll.Infow("incoming task")

		c, err := ValidateIncomingVideo(t.URL)
		if err != nil {
			ll.Errorw("task rejected", "reason", "validation failed", "err", err)
			p.RejectTask(t)
			continue
		}

		p.StartTask(t)
		streamFH, _, err := c.Download(path.Join(os.TempDir(), "transcoder", "streams"))

		if err != nil {
			ll.Errorw("task released", "reason", "download failed", "err", err)
			tErr := p.ReleaseTask(t)
			if tErr != nil {
				ll.Errorw("error releasing task", "tid", t.ID, "err", tErr)
			}
			continue
		}

		ll = ll.With("file", streamFH.Name())

		if err := streamFH.Close(); err != nil {
			ll.Errorw("task released", "reason", "closing downloaded file failed", "err", err)
			p.ReleaseTask(t)
			continue
		}

		tmr := timer.Start()

		streamPath := fmt.Sprintf("%v_%v", c.NormalizedName, c.SDHash[:6])
		out := path.Join(videoPath, streamPath)

		enc, err := encoder.NewEncoder(streamFH.Name(), out)
		if err != nil {
			ll.Errorw("task rejected", "reason", "encoder initialization failure", "err", err)
			p.RejectTask(t)
			continue
		}

		ll.Infow("starting encoding")
		e, err := enc.Encode()
		if err != nil {
			ll.Errorw("task rejected", "reason", "encoding failure", "err", err)
			p.RejectTask(t)
			continue
		}

		for i := range e {
			ll.Debugw("encoding", "progress", fmt.Sprintf("%.2f", i.GetProgress()))
			p.ProgressTask(t, i.GetProgress())

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
		_, err = lib.Add(AddParams{
			URL:     t.URL,
			SDHash:  t.SDHash,
			Type:    formats.TypeHLS,
			Path:    streamPath,
			Channel: c.SigningChannel.CanonicalURL,
		})
		if err != nil {
			logger.Errorw("adding to video library failed", "err", err)
		}
		err = os.Remove(streamFH.Name())
		if err != nil {
			logger.Errorw("cleanup failed", "err", err)
		}
	}
}
