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

	retireTicker := time.NewTicker(1 * time.Hour)

	go func() {
		for {
			select {
			case <-retireTicker.C:
				retireVideos(lib, storageName, maxSize)
			case <-stopChan:
				logger.Info("stopping library maintenance")
				return
			default:
				time.Sleep(1 * time.Second)
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
		totalSize += uint64(v.Size)
	}
	if maxSize >= totalSize {
		return
	}

	weight := func(v db.Video) int64 { return v.AccessedAt.Unix() }
	sort.Slice(videos, func(i, j int) bool { return weight(videos[i]) < weight(videos[j]) })
	for _, s := range videos {
		err := call(s)
		if err != nil {
			logger.Warnw("failed to execute function for video", "sd_hash", s.SDHash, "err", err)
			continue
		}
		furloughedSize += uint64(s.Size)
		logger.Debugf("processed: %v, left: %v", furloughedSize, totalSize-furloughedSize)
		if maxSize >= totalSize-furloughedSize {
			break
		}
	}

	return
}
