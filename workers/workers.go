package workers

import (
	"fmt"
	"os"
	"path"
	"time"

	"github.com/lbryio/transcoder/encoder"
	"github.com/lbryio/transcoder/formats"
	"github.com/lbryio/transcoder/internal/metrics"
	"github.com/lbryio/transcoder/manager"
	"github.com/lbryio/transcoder/pkg/dispatcher"
	"github.com/lbryio/transcoder/pkg/timer"
	"github.com/lbryio/transcoder/video"
)

type encoderWorker struct {
	mgr *manager.VideoManager
}

func (w encoderWorker) Do(t dispatcher.Task) error {
	lib := w.mgr.Library()

	r := t.Payload.(*manager.TranscodingRequest)

	streamDest := path.Join(os.TempDir(), "transcoder", "streams")
	ll := logger.Named("worker").With("uri", r.URI)

	ll.Infow("started processing transcoding request", "local_destination", streamDest)
	streamFH, streamSize, err := r.Download(streamDest)
	metrics.DownloadedSizeMB.Add(float64(streamSize) / 1024 / 1024)

	if err != nil {
		r.Release()
		ll.Errorw("transcoding request released", "reason", "download failed", "err", err)
		return err
	}

	ll = ll.With("file", streamFH.Name())

	if err := streamFH.Close(); err != nil {
		r.Release()
		ll.Errorw("transcoding request released", "reason", "closing downloaded file failed", "err", err)
		return err
	}

	tmr := timer.Start()

	localStream := lib.New(r.SDHash)

	enc, err := encoder.NewEncoder(streamFH.Name(), localStream.FullPath())
	if err != nil {
		r.Reject()
		ll.Errorw("transcoding request rejected", "reason", "encoder initialization failure", "err", err)
		return err
	}

	ll.Infow("starting encoding")

	metrics.TranscodingRunning.Inc()
	e, err := enc.Encode()
	if err != nil {
		ll.Errorw("transcoding request rejected", "reason", "encoding failure", "err", err)
		r.Reject()
		metrics.TranscodingRunning.Dec()
		return err
	}

	for i := range e {
		ll.Debugw("encoding", "progress", fmt.Sprintf("%.2f", i.GetProgress()))
	}
	r.Complete()
	metrics.TranscodingRunning.Dec()
	metrics.TranscodingSpentSeconds.Add(tmr.Duration())
	ll.Infow(
		"encoding complete",
		"out", localStream.FullPath(),
		"seconds_spent", tmr.String(),
		"duration", enc.Meta().Format.Duration,
		"bitrate", enc.Meta().Format.GetBitRate(),
	)

	time.Sleep(2 * time.Second)
	err = localStream.ReadMeta()
	if err != nil {
		logger.Errorw("filling stream metadata failed", "err", err)
	}

	_, err = lib.Add(video.AddParams{
		URL:      r.URI,
		SDHash:   r.SDHash,
		Type:     formats.TypeHLS,
		Channel:  r.ChannelURI,
		Path:     localStream.LastPath(),
		Size:     localStream.Size(),
		Checksum: localStream.Checksum(),
	})
	if err != nil {
		logger.Errorw("adding to video library failed", "err", err)
	}

	metrics.TranscodedCount.Inc()
	metrics.TranscodedSizeMB.Add(float64(localStream.Size()) / 1024 / 1024)

	err = os.Remove(streamFH.Name())
	if err != nil {
		logger.Errorw("cleanup failed", "err", err)
	}
	return nil
}

func SpawnEncoderWorkers(wnum int, mgr *manager.VideoManager) chan<- interface{} {
	logger.Infof("starting %v encoders", wnum)
	worker := encoderWorker{mgr: mgr}
	d := dispatcher.Start(wnum, worker, 0)
	stopChan := make(chan interface{})

	requests := mgr.Requests()
	go func() {
		for {
			time.Sleep(100 * time.Millisecond)
			select {
			case e := <-requests:
				if e != nil {
					logger.Infow("got transcoding request", "lbry_url", e.URI)
					d.Dispatch(e)
				}
			case <-stopChan:
				d.Stop()
				return
			// case <-time.After(10 * time.Millisecond):
			// 	continue
			default:
				time.Sleep(50 * time.Millisecond)
			}
		}
	}()
	return stopChan
}
