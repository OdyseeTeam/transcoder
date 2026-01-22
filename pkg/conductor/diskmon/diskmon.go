package diskmon

import (
	"context"
	"errors"
	"time"

	"github.com/OdyseeTeam/transcoder/pkg/conductor/metrics"
	"github.com/OdyseeTeam/transcoder/pkg/logging"

	"golang.org/x/sys/unix"
)

var (
	ErrTimeout = errors.New("disk pressure wait timeout")

	statfsFunc = unix.Statfs
)

type Config struct {
	Enabled       bool
	Path          string
	Threshold     int
	CheckInterval time.Duration
	MaxWait       time.Duration
}

func GetDiskUsage(path string) (float64, error) {
	var stat unix.Statfs_t
	err := statfsFunc(path, &stat)
	if err != nil {
		return 0, err
	}

	if stat.Blocks == 0 || stat.Bsize <= 0 {
		return 0, nil
	}

	bsize := uint64(stat.Bsize) // #nosec G115 -- checked non-negative above
	total := stat.Blocks * bsize
	available := stat.Bavail * bsize
	used := total - available
	usagePercent := float64(used) / float64(total) * 100

	return usagePercent, nil
}

func WaitForDiskSpace(ctx context.Context, cfg Config, logger logging.KVLogger) error {
	threshold := cfg.Threshold
	if threshold <= 0 || threshold > 100 {
		threshold = 90
	}

	checkInterval := cfg.CheckInterval
	if checkInterval <= 0 {
		checkInterval = 10 * time.Second
	}

	maxWait := cfg.MaxWait
	if maxWait <= 0 {
		maxWait = 5 * time.Minute
	}

	usage, err := GetDiskUsage(cfg.Path)
	if err != nil {
		logger.Warn("disk usage check failed, proceeding anyway", "path", cfg.Path, "err", err)
		return nil
	}

	metrics.DiskUsagePercent.Set(usage)

	thresholdFloat := float64(threshold)
	if usage <= thresholdFloat {
		return nil
	}

	metrics.DiskWaitTotal.Inc()
	logger.Warn("waiting for disk space", "usage_percent", usage, "threshold", threshold, "path", cfg.Path)
	waitStart := time.Now()

	timeoutCtx, timeoutCancel := context.WithTimeout(ctx, maxWait)
	defer timeoutCancel()

	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-timeoutCtx.Done():
			if errors.Is(timeoutCtx.Err(), context.DeadlineExceeded) {
				logger.Error("disk wait timeout, returning task to queue", "usage_percent", usage, "waited", time.Since(waitStart))
				metrics.DiskWaitTimeoutTotal.Inc()
				return ErrTimeout
			}
			return timeoutCtx.Err()
		case <-ticker.C:
			usage, err = GetDiskUsage(cfg.Path)
			if err != nil {
				logger.Warn("disk usage check failed, proceeding anyway", "path", cfg.Path, "err", err)
				return nil
			}

			metrics.DiskUsagePercent.Set(usage)

			if usage <= thresholdFloat {
				logger.Info("disk space available, proceeding", "usage_percent", usage, "waited", time.Since(waitStart))
				return nil
			}

			logger.Debug("still waiting for disk space", "usage_percent", usage, "waited", time.Since(waitStart))
		}
	}
}
