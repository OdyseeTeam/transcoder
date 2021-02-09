package video

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/c2h5oh/datasize"
)

var maxLocalStorage = int64(math.Pow(1000, 4) * 1.8) // 1.8T
var maxS3Storage = int64(math.Pow(1000, 4) * 100)    // 100T

func tailVideos(items []*Video, maxSize uint64, call func(v *Video) error) (totalSize uint64, furloughedSize uint64, err error) {
	for _, v := range items {
		totalSize += uint64(v.GetSize())
	}
	if maxSize >= totalSize {
		return
	}

	sort.Slice(items, func(i, j int) bool { return items[i].GetWeight() < items[j].GetWeight() })
	for _, s := range items {
		err := call(s)
		if err != nil {
			return totalSize, furloughedSize, err
		}
		furloughedSize += uint64(s.GetSize())
		logger.Debugf("furloughed: %v, left: %v", furloughedSize, totalSize-furloughedSize)
		if maxSize >= totalSize-furloughedSize {
			break
		}
	}

	return
}

// FurloughVideos deletes older videos locally, leaving them only on S3, keeping total size of
// local videos at maxSize.
func FurloughVideos(lib *Library, maxSize uint64) (uint64, uint64, error) {
	items, err := lib.ListLocal()
	if err != nil {
		return 0, 0, err
	}
	return tailVideos(items, maxSize, lib.Furlough)
}

// RetireVideos deletes older videos from S3, keeping total size of remote videos at maxSize.
func RetireVideos(lib *Library, maxSize uint64) (uint64, uint64, error) {
	items, err := lib.ListRemoteOnly()
	if err != nil {
		return 0, 0, err
	}
	return tailVideos(items, maxSize, lib.Retire)
}

func SpawnMaintenance(lib *Library) chan<- bool {
	logger.Infow(
		"starting library maintenance",
		"max_local_size", fmt.Sprintf("%vGB", datasize.ByteSize(lib.maxLocalSize).GBytes()),
		"max_remote_size", fmt.Sprintf("%vGB", datasize.ByteSize(lib.maxRemoteSize).GBytes()),
	)
	furloughTicker := time.NewTicker(30 * time.Minute)
	retireTicker := time.NewTicker(24 * time.Hour)
	stopChan := make(chan bool)

	go func() {
		for {
			select {
			case <-furloughTicker.C:
				if lib.maxLocalSize == 0 {
					continue
				}
				_, freedSize, err := FurloughVideos(lib, lib.maxLocalSize)
				if err != nil {
					logger.Infow("failed to furlough videos", "size", freedSize, "err", err)
				} else if freedSize > 0 {
					logger.Infow("furloughed some videos", "size", freedSize)
				}
			case <-retireTicker.C:
				if lib.maxRemoteSize == 0 {
					continue
				}
				_, freedSize, err := RetireVideos(lib, lib.maxRemoteSize)
				if err != nil {
					logger.Infow("failed to retire videos", "size", freedSize, "err", err)
				} else if freedSize > 0 {
					logger.Infow("retired some videos", "size", freedSize)
				}
			case <-stopChan:
				logger.Info("stopping library maintenance")
				return
			}
		}
	}()

	return stopChan
}
