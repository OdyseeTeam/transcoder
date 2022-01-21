package workers

import (
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/lbryio/transcoder/encoder"
	"github.com/lbryio/transcoder/internal/metrics"
	"github.com/lbryio/transcoder/ladder"
	"github.com/lbryio/transcoder/manager"
	"github.com/lbryio/transcoder/pkg/dispatcher"
	"github.com/lbryio/transcoder/pkg/logging/zapadapter"
	"github.com/lbryio/transcoder/pkg/timer"
	"github.com/lbryio/transcoder/storage"
	"github.com/lbryio/transcoder/video"

	"github.com/pkg/errors"
)

type encoderWorker struct {
	mgr     *manager.VideoManager
	encoder encoder.Encoder
}

func (w encoderWorker) Work(t dispatcher.Task) error {
	lib := w.mgr.Library()

	r := t.Payload.(*manager.TranscodingRequest)

	streamDest := path.Join(os.TempDir(), "transcoder", "streams")
	ll := logger.Named("worker").With("uri", r.URI, "sd_hash", r.SDHash)

	ll.Infow("started processing transcoding request", "dst", streamDest)
	TranscodingDownloading.Inc()
	streamFH, streamSize, err := r.Download(streamDest)
	TranscodingDownloading.Dec()
	metrics.DownloadedSizeMB.Add(float64(streamSize) / 1024 / 1024)

	if err != nil {
		if strings.HasSuffix(err.Error(), "503") || strings.Contains(err.Error(), "blob not found") {
			r.Reject()
			ll.Errorw("transcoding request rejected", "reason", "download failed fatally", "err", err)
		} else {
			r.Release()
			ll.Errorw("transcoding request released", "reason", "download failed", "err", err)
		}
		TranscodingErrorsCount.WithLabelValues("download").Inc()
		return err
	}

	if err := streamFH.Close(); err != nil {
		r.Release()
		TranscodingErrorsCount.WithLabelValues("fs").Inc()
		ll.Errorw("transcoding request released", "reason", "closing downloaded file failed", "err", err)
		return err
	}

	tmr := timer.Start()

	streamPath := path.Join(lib.Path(), r.SDHash)
	cleanupLocalStream := func() {
		err := os.RemoveAll(streamPath)
		if err != nil {
			ll.Warn("cleaning up incomplete local stream failed", "err", err)
		}
	}

	res, err := w.encoder.Encode(streamFH.Name(), streamPath)
	if err != nil {
		r.Reject()
		TranscodingErrorsCount.WithLabelValues("encode").Inc()
		cleanupLocalStream()
		return err
	}
	ll.Infow("starting encoding")
	TranscodingRunning.Inc()

	for i := range res.Progress {
		ll.Debugw("encoding", "progress", fmt.Sprintf("%.2f", i.GetProgress()))
	}

	TranscodingRunning.Dec()
	TranscodingSpentSeconds.Add(tmr.Duration())

	md, _ := strconv.ParseFloat(res.OrigMeta.FMeta.Format.Duration, 64)
	TranscodedSeconds.Add(md)
	ll.Infow(
		"encoding complete",
		"out", streamPath,
		"seconds_spent", tmr.String(),
		"duration", res.OrigMeta.FMeta.Format.Duration,
		"bitrate", res.OrigMeta.FMeta.Format.GetBitRate(),
		"channel", r.ChannelURI,
	)

	time.Sleep(2 * time.Second)
	ls, err := storage.OpenLocalStream(streamPath)
	if err != nil {
		TranscodingErrorsCount.WithLabelValues("encode").Inc()
		cleanupLocalStream()
		return errors.Wrap(err, "error opening stream")
	}

	_, err = lib.Add(video.AddParams{
		URL:      r.URI,
		SDHash:   r.SDHash,
		Type:     ladder.TypeHLS,
		Channel:  r.ChannelURI,
		Path:     ls.BasePath(),
		Size:     ls.Size(),
		Checksum: ls.Checksum(),
	})

	if err != nil {
		TranscodingErrorsCount.WithLabelValues("db").Inc()
		cleanupLocalStream()
		return errors.Wrap(err, "adding to video library failed")
	}

	r.Complete()
	TranscodedCount.Inc()
	TranscodedSizeMB.Add(float64(ls.Size()) / 1024 / 1024)

	err = os.Remove(streamFH.Name())
	if err != nil {
		logger.Errorw("cleanup failed", "err", err)
	}
	return nil
}

func SpawnEncoderWorkers(wnum int, mgr *manager.VideoManager) chan<- interface{} {
	RegisterMetrics()

	logger.Infof("starting %v encoders", wnum)
	enc, err := encoder.NewEncoder(encoder.Configure().Log(zapadapter.NewKV(logger.Desugar())))
	if err != nil {
		logger.Fatal(err)
	}
	worker := encoderWorker{mgr: mgr, encoder: enc}
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
