package video

import (
	"time"

	"github.com/lbryio/transcoder/internal/metrics"
	"github.com/lbryio/transcoder/pkg/dispatcher"
	"github.com/lbryio/transcoder/pkg/mfr"

	cmap "github.com/orcaman/concurrent-map"
)

func SpawnProcessing(m *mfr.Queue, lib *Library) {
	logger.Info("started video processor")
	defer logger.Info("quit video processor")

	// for t := range p.IncomingTasks() {
	// 	ll := logger.Named("worker").With("url", t.URL, "task_id", t.ID)

	// 	c, err := claim.Resolve(t.URL)
	// 	if err != nil {
	// 		ll.Errorw("resolve failed", "err", err)
	// 		p.RejectTask(t)
	// 		continue
	// 	}

	// 	ll.Infow("starting task")
	// 	p.StartTask(t)
	// 	streamFH, streamSize, err := c.Download(path.Join(os.TempDir(), "transcoder", "streams"))
	// 	metrics.DownloadedSizeMB.Add(float64(streamSize) / 1024 / 1024)

	// 	if err != nil {
	// 		ll.Errorw("task released", "reason", "download failed", "err", err)
	// 		tErr := p.ReleaseTask(t)
	// 		if tErr != nil {
	// 			ll.Errorw("error releasing task", "tid", t.ID, "err", tErr)
	// 		}
	// 		continue
	// 	}

	// 	ll = ll.With("file", streamFH.Name())

	// 	if err := streamFH.Close(); err != nil {
	// 		ll.Errorw("task released", "reason", "closing downloaded file failed", "err", err)
	// 		p.ReleaseTask(t)
	// 		continue
	// 	}

	// 	tmr := timer.Start()

	// 	localStream := lib.local.New(c.SDHash)

	// 	enc, err := encoder.NewEncoder(streamFH.Name(), localStream.FullPath())
	// 	if err != nil {
	// 		ll.Errorw("task rejected", "reason", "encoder initialization failure", "err", err)
	// 		p.RejectTask(t)
	// 		continue
	// 	}

	// 	ll.Infow("starting encoding")

	// 	metrics.TranscodingRunning.Inc()
	// 	e, err := enc.Encode()
	// 	if err != nil {
	// 		ll.Errorw("task rejected", "reason", "encoding failure", "err", err)
	// 		p.RejectTask(t)
	// 		metrics.TranscodingRunning.Dec()
	// 		continue
	// 	}

	// 	for i := range e {
	// 		ll.Debugw("encoding", "progress", fmt.Sprintf("%.2f", i.GetProgress()))
	// 		p.ProgressTask(t, i.GetProgress())

	// 		if i.GetProgress() >= 99.9 {
	// 			p.CompleteTask(t)
	// 			metrics.TranscodingRunning.Dec()
	// 			metrics.TranscodingSpentSeconds.Add(tmr.Duration())
	// 			ll.Infow(
	// 				"encoding complete",
	// 				"out", localStream.FullPath(),
	// 				"seconds_spent", tmr.String(),
	// 				"duration", enc.Meta.Format.Duration,
	// 				"bitrate", enc.Meta.Format.GetBitRate(),
	// 			)
	// 			break
	// 		}
	// 	}

	// 	time.Sleep(10 * time.Second)
	// 	err = localStream.ReadMeta()
	// 	if err != nil {
	// 		logger.Errorw("filling stream metadata failed", "err", err)
	// 	}

	// 	_, err = lib.Add(AddParams{
	// 		URL:      t.URL,
	// 		SDHash:   t.SDHash,
	// 		Type:     formats.TypeHLS,
	// 		Channel:  c.SigningChannel.CanonicalURL,
	// 		Path:     localStream.LastPath(),
	// 		Size:     localStream.Size(),
	// 		Checksum: localStream.Checksum(),
	// 	})
	// 	if err != nil {
	// 		logger.Errorw("adding to video library failed", "err", err)
	// 	}

	// 	metrics.TranscodedCount.Inc()
	// 	metrics.TranscodedSizeMB.Add(float64(localStream.Size()) / 1024 / 1024)

	// 	err = os.Remove(streamFH.Name())
	// 	if err != nil {
	// 		logger.Errorw("cleanup failed", "err", err)
	// 	}
	// }
}

type s3uploader struct {
	lib        *Library
	processing cmap.ConcurrentMap
}

func (u s3uploader) Do(t dispatcher.Task) error {
	v := t.Payload.(*Video)
	u.processing.Set(v.SDHash, v)

	logger.Infow("uploading stream to S3", "sd_hash", v.SDHash, "size", v.GetSize())

	// ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	// defer cancel()
	// err := dispatcher.WaitUntilTrue(ctx, 300*time.Millisecond, func() bool {
	// 	if _, err := u.lib.local.Open(v.SDHash); err == nil {
	// 		return true
	// 	}
	// 	return false
	// })
	// if err != nil {
	// 	return errors.New("timed out waiting for master playlist to appear")
	// }

	lv, err := u.lib.local.Open(v.SDHash)
	if err != nil {
		u.processing.Remove(v.SDHash)
		return err
	}

	rs, err := u.lib.remote.Put(lv)
	if err != nil {
		u.processing.Remove(v.SDHash)
		return err
	}
	v.RemotePath = rs.URL()

	err = u.lib.UpdateRemotePath(v.SDHash, v.RemotePath)
	if err != nil {
		logger.Errorw("error updating video", "sd_hash", v.SDHash, "remote_path", rs.URL(), "err", err)
		u.processing.Remove(v.SDHash)
		return err
	}
	metrics.S3UploadedSizeMB.Add(float64(v.GetSize()))
	logger.Infow("uploaded stream to S3", "sd_hash", v.SDHash, "remote_path", rs.URL(), "size", v.GetSize())
	return nil
}

func Spawns3uploader(lib *Library) dispatcher.Dispatcher {
	logger.Info("starting s3 uploader")
	s3up := s3uploader{lib: lib, processing: cmap.New()}
	d := dispatcher.Start(5, s3up, 100)
	ticker := time.NewTicker(5 * time.Second)

	go func() {
		for {
			select {
			case <-ticker.C:
				videos, err := lib.ListLocalOnly()
				if err != nil {
					logger.Errorw("listing non-uploaded videos failed", "err", err)
					return
				}
				for _, v := range videos {
					absent := s3up.processing.SetIfAbsent(v.SDHash, &v)
					if absent {
						d.Dispatch(v)
					}
				}
			}
		}
	}()

	return d
}
