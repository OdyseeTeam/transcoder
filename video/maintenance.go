package video

import (
	"fmt"
	"time"

	"github.com/c2h5oh/datasize"
	"github.com/lbryio/transcoder/formats"
	"github.com/lbryio/transcoder/queue"
)

func toGB(s uint64) string {
	return fmt.Sprintf("%vGB", datasize.ByteSize(s).GBytes())
}

func SpawnLibraryCleaning(lib *Library) chan<- bool {
	mls := toGB(lib.maxLocalSize)
	mrs := toGB(lib.maxRemoteSize)
	logger.Infow(
		"starting library maintenance",
		"max_local_size", mls,
		"max_remote_size", mrs,
	)
	furloughTicker := time.NewTicker(5 * time.Minute)
	retireTicker := time.NewTicker(24 * time.Hour)
	stopChan := make(chan bool)

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

// PopularQueuingOpts sets additional options for SpawnPopularQueuing routine.
type PopularQueuingOpts struct {
	// TopNumber limits the number of top viewed videos that will be added to queue every time.
	TopNumber int
	// LowerBound sets a lower limit for video views number to be considered.
	LowerBound int
	// Interval is the interval at which sweeping routine will be run.
	Interval time.Duration
}

// SpawnPopularQueuing will tally up the count of rejected videos and pick top N of them
// to be added to the queue.
// For it to work, `lib.IncViews(url, sdHash)` should be called somewhere for every video that is requested but rejected.
func SpawnPopularQueuing(lib *Library, q *queue.Queue, opts PopularQueuingOpts) chan<- bool {
	sweepTicker := time.NewTicker(opts.Interval)
	stopChan := make(chan bool)
	ll := logger.Named("sweeper")
	ll.Infow(
		"starting",
	)

	go func() {
		for {
			select {
			case <-sweepTicker.C:
				items := lib.sweeper.Top(opts.TopNumber, opts.LowerBound)
				added := []string{}
				for _, i := range items {
					q.Add(i.URL, i.SDHash, formats.TypeHLS)
					added = append(added, fmt.Sprintf("{%v}%v", i.Count, i.URL))
				}
				lib.sweeper.Sweep(items)
				if len(added) > 0 {
					ll.Infow("added popular videos to queue", "urls", added)
				} else {
					lowItems := lib.sweeper.Top(3, 0)
					lowURLs := []string{}
					for _, i := range lowItems {
						lowURLs = append(lowURLs, fmt.Sprintf("{%v}%v", i.Count, i.URL))
					}
					ll.Infow("not enough popular videos", "top3_urls", lowURLs)
				}
			case <-stopChan:
				ll.Info("stopping")
				return
			}
		}
	}()

	return stopChan
}
