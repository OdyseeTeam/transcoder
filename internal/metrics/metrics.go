package metrics

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

	TranscodingSpentSeconds = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "transcoded_spent_seconds",
	})
	TranscodedSizeMB = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "transcoded_size_mb",
	})
	TranscodedCount = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "transcoded_count",
	})
	DownloadedSizeMB = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "downloaded_size_mb",
	})
	S3UploadedSizeMB = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "s3_uploaded_size_mb",
	})

	EncodedDurationSeconds = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "encoded_duration_seconds",
	})
	EncodedBitrateMbit = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "encoded_bitrate_mbit",
			Buckets: []float64{0.5, 1, 1.5, 2, 2.5, 3, 4, 5, 6, 7, 8, 10, 15, 20, 25, 30},
		},
		[]string{"resolution"},
	)

	StreamsRequestedCount = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "streams_requested_count",
	}, []string{"storage"})

	HTTPAPIRequests = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_api_requests",
			Help:    "Method call latency distributions",
			Buckets: []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.4, 1, 2, 5, 10},
		},
		[]string{"status_code"},
	)
)

func RegisterMetrics() {
	once.Do(func() {
		prometheus.MustRegister(
			TranscodingRunning, TranscodingSpentSeconds, TranscodedSizeMB, TranscodedCount,
			DownloadedSizeMB, S3UploadedSizeMB, EncodedDurationSeconds, EncodedBitrateMbit,
			StreamsRequestedCount, HTTPAPIRequests,
		)
	})

}
