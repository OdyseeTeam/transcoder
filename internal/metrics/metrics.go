package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	StorageLocal  = "local"
	StorageRemote = "remote"
)

var (
	TranscodingRunning = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "transcoding_running",
	})

	TranscodingSpentSeconds = promauto.NewCounter(prometheus.CounterOpts{
		Name: "transcoded_spent_seconds",
	})
	TranscodedSizeMB = promauto.NewCounter(prometheus.CounterOpts{
		Name: "transcoded_size_mb",
	})
	TranscodedCount = promauto.NewCounter(prometheus.CounterOpts{
		Name: "transcoded_count",
	})
	DownloadedSizeMB = promauto.NewCounter(prometheus.CounterOpts{
		Name: "downloaded_size_mb",
	})
	S3UploadedSizeMB = promauto.NewCounter(prometheus.CounterOpts{
		Name: "s3_uploaded_size_mb",
	})

	EncodedDurationSeconds = promauto.NewCounter(prometheus.CounterOpts{
		Name: "encoded_duration_seconds",
	})
	EncodedBitrateMbit = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "encoded_bitrate_mbit",
			Buckets: []float64{0.5, 1, 1.5, 2, 2.5, 3, 4, 5, 6, 7, 8, 10, 15, 20, 25, 30},
		},
		[]string{"resolution"},
	)

	StreamsRequestedCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "streams_requested_count",
	}, []string{"storage"})

	HTTPAPIRequests = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_api_requests",
			Help:    "Method call latency distributions",
			Buckets: []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.4, 1, 2, 5, 10},
		},
		[]string{"status_code"},
	)
)
