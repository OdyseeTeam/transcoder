package video

import (
	"fmt"
	"time"

	"github.com/c2h5oh/datasize"
)

func toGB(s uint64) string {
	return fmt.Sprintf("%.2fGB", datasize.ByteSize(s).GBytes())
}

func remoteFurlough(lib *Library) {
	logger.Infow("starting library retirement procedure", "max_remote_size", toGB(lib.maxRemoteSize))
	totalSize, retiredSize, err := RetireVideos(lib, lib.maxRemoteSize)
	ll := logger.With("total_gb", toGB(totalSize), "retired_gb", toGB(retiredSize))
	LibraryBytes.Set(float64(totalSize))
	LibraryRetiredBytes.Add(float64(retiredSize))
	if err != nil {
		ll.Infow("error retiring videos", "err", err)
	} else if retiredSize > 0 {
		ll.Infow("retired some videos")
	} else {
		ll.Infow("failed to retire any videos")
	}
}

func SpawnLibraryCleaning(lib *Library) chan<- interface{} {
	mls := toGB(lib.maxLocalSize)
	mrs := toGB(lib.maxRemoteSize)
	logger.Infow(
		"starting library maintenance",
		"max_local_size", mls,
		"max_remote_size", mrs,
	)
	furloughTicker := time.NewTicker(5 * time.Minute)
	retireTicker := time.NewTicker(24 * time.Hour)
	stopChan := make(chan interface{})

	go func() {
		for {
			select {
			case <-furloughTicker.C:
				if lib.maxLocalSize == 0 {
					continue
				}
				logger.Infow("starting furloughing procedure", "max_local_size", mls)

				totalSize, freedSize, err := FurloughVideos(lib, lib.maxLocalSize)
				ll := logger.With("total_size", toGB(totalSize), "freed_size", toGB(freedSize))

				if err != nil {
					ll.Infow("error furloughing videos", "err", err)
				} else if freedSize > 0 {
					ll.Infow("furloughed some videos")
				} else {
					ll.Infow("failed to furlough any videos")
				}
			case <-retireTicker.C:
				if lib.maxRemoteSize == 0 {
					continue
				}

				logger.Infow("starting retirement procedure", "max_remote_size", mrs)
				totalSize, freedSize, err := RetireVideos(lib, lib.maxRemoteSize)
				ll := logger.With("total_size", toGB(totalSize), "freed_size", toGB(freedSize))
				if err != nil {
					ll.Infow("error retiring videos", "err", err)
				} else if freedSize > 0 {
					ll.Infow("retired some videos")
				} else {
					ll.Infow("failed to retire any videos")
				}
			case <-stopChan:
				logger.Info("stopping library maintenance")
				return
			}
		}
	}()

	return stopChan
}

func SpawnRemoteLibraryCleaning(lib *Library) chan struct{} {
	stopChan := make(chan struct{})
	if lib.maxRemoteSize == 0 {
		return stopChan
	}
	logger.Infow(
		"starting remote library maintenance",
		"max_remote_size", toGB(lib.maxRemoteSize),
	)
	remoteFurlough(lib)

	retireTicker := time.NewTicker(1 * time.Hour)

	go func() {
		for {
			select {
			case <-retireTicker.C:
				remoteFurlough(lib)
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
