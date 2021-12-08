package video

import (
	"path"
	"time"

	"github.com/lbryio/transcoder/internal/metrics"
	"github.com/lbryio/transcoder/pkg/dispatcher"
	"github.com/lbryio/transcoder/storage"

	cmap "github.com/orcaman/concurrent-map"
)

type s3uploader struct {
	lib        *Library
	processing cmap.ConcurrentMap
}

func (u s3uploader) Work(t dispatcher.Task) error {
	v := t.Payload.(*Video)
	u.processing.Set(v.SDHash, v)
	defer u.processing.Remove(v.Path)

	logger.Infow("uploading stream to S3", "sd_hash", v.SDHash, "size", v.GetSize())

	ls, err := storage.OpenLocalStream(path.Join(u.lib.local.Path(), v.Path))
	if err != nil {
		return err
	}

	rs, err := u.lib.remote.Put(ls)
	if err != nil {
		if err != storage.ErrStreamExists {
			u.lib.UpdateRemotePath(v.SDHash, v.RemotePath)
			return err
		}
		v.RemotePath = rs.URL()
	}

	err = u.lib.UpdateRemotePath(v.SDHash, v.RemotePath)
	if err != nil {
		logger.Errorw("error updating video", "sd_hash", v.SDHash, "remote_path", rs.URL(), "err", err)
		return err
	}
	metrics.S3UploadedSizeMB.Add(float64(v.GetSize()))
	logger.Infow("uploaded stream to S3", "sd_hash", v.SDHash, "remote_path", rs.URL(), "size", v.GetSize())
	return nil
}

func SpawnS3uploader(lib *Library) chan<- interface{} {
	logger.Info("starting s3 uploaders")
	s3up := s3uploader{lib: lib, processing: cmap.New()}
	d := dispatcher.Start(10, s3up, 0)
	ticker := time.NewTicker(1 * time.Second)
	stopChan := make(chan interface{})

	go func() {
		for {
			select {
			case <-ticker.C:
				videos, err := lib.ListLocalOnly()
				if err != nil {
					logger.Errorw("listing non-uploaded videos failed", "err", err)
					return
				}
				if len(videos) > 0 {
					logger.Infow("found non-uploaded videos", "count", len(videos))
				} else {
					logger.Infow("no non-uploaded videos found")
				}
				for _, v := range videos {
					absent := s3up.processing.SetIfAbsent(v.SDHash, &v)
					if absent {
						d.Dispatch(v)
					}
				}
			case <-stopChan:
				d.Stop()
				return
			}
		}
	}()

	return stopChan
}
