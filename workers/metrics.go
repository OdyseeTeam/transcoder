package workers

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	StorageLocal  = "local"
	StorageRemote = "remote"
)

var (
	once = sync.Once{}

	TranscodingRunning = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "transcoding_running",
	})
	TranscodingDownloading = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "transcoding_downloading",
	})
	TranscodingSpentSeconds = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "transcoded_spent_seconds",
	})

	TranscodedSeconds = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "transcoded_seconds",
	})
	TranscodedSizeMB = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "transcoded_size_mb",
	})
	TranscodedCount = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "transcoded_count",
	})

	TranscodingErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "transcoding_error_count",
	}, []string{"stage"})
)

func RegisterMetrics() {
	once.Do(func() {
		prometheus.MustRegister(
			TranscodingRunning, TranscodingSpentSeconds,
			TranscodedSeconds, TranscodedSizeMB, TranscodedCount, TranscodingErrors,
		)
	})

}
