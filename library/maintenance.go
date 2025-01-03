package library

import (
	"fmt"
	"sort"
	"time"

	"github.com/OdyseeTeam/transcoder/library/db"

	"github.com/c2h5oh/datasize"
)

func SpawnLibraryCleaning(lib *Library, storageName string, maxSize uint64) chan struct{} {
	stopChan := make(chan struct{})
	logger.Infow(
		"starting remote library maintenance",
		"max_remote_size", toGB(maxSize),
	)

	running := make(chan struct{}, 1)
	ticker := time.NewTicker(1 * time.Hour)

	running <- struct{}{}
	go func() {
		defer func() { <-running }()
		retireVideos(lib, storageName, maxSize)
	}()

	go func() {
		for range ticker.C {
			select {
			case running <- struct{}{}:
				go func() {
					defer func() { <-running }()
					retireVideos(lib, storageName, maxSize)
				}()
			case <-stopChan:
				logger.Info("stopping library maintenance")
				return
			default:
			}
		}
	}()

	return stopChan
}

func toGB(s uint64) string {
	return fmt.Sprintf("%.2fGB", datasize.ByteSize(s).GBytes())
}

func retireVideos(lib *Library, storageName string, maxSize uint64) {
	logger.Infow("starting library retirement procedure", "max_remote_size", toGB(maxSize))
	totalSize, retiredSize, err := lib.RetireVideos(storageName, maxSize)
	ll := logger.With("total_gb", toGB(totalSize), "retired_gb", toGB(retiredSize))
	LibraryBytes.Set(float64(totalSize))
	LibraryRetiredBytes.Add(float64(retiredSize))
	switch {
	case err != nil:
		ll.Infow("error retiring videos", "err", err)
	case retiredSize > 0:
		ll.Infow("retired some videos")
	default:
		ll.Infow("failed to retire any videos")
	}
}

func tailVideos(videos []db.Video, maxSize uint64, call func(v db.Video) error) (totalSize uint64, furloughedSize uint64, err error) {
	for _, v := range videos {
		if v.Size < 0 {
			logger.Warnw("invalid video size", "tid", v.TID, "size", v.Size)
			continue
		}
		totalSize += uint64(v.Size)
	}
	if maxSize >= totalSize {
		return
	}

	weight := func(v db.Video) int64 { return v.AccessedAt.Unix() }
	sort.Slice(videos, func(i, j int) bool { return weight(videos[i]) < weight(videos[j]) })
	allStart := time.Now()
	for _, v := range videos {
		if v.Size < 0 {
			continue
		}
		err := call(v)
		if err != nil {
			logger.Warnw("failed to execute function for video", "sd_hash", v.SDHash, "err", err)
			continue
		}
		furloughedSize += uint64(v.Size)
		remainingSize := totalSize - maxSize - furloughedSize

		furloughedGB := float64(furloughedSize) / float64(1<<30)
		remainingGB := float64(remainingSize) / float64(1<<30)
		speed := furloughedGB / time.Since(allStart).Seconds() * 60 * 60
		remainingHours := remainingGB / (furloughedGB / time.Since(allStart).Seconds()) / 60 / 60
		donePct := float64(furloughedSize) / float64(totalSize-maxSize) * float64(100)
		logger.Infof(
			"maintenance: %.1f h, %.4f%% , %.2f GB/h, %.2f GB, remaining: %.2f GB, %.1f h",
			time.Since(allStart).Hours(), donePct, speed, furloughedGB, remainingGB, remainingHours,
		)

		if maxSize >= totalSize-furloughedSize {
			break
		}
	}

	return
}
