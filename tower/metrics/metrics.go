package metrics

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	StorageLocal  = "local"
	StorageRemote = "remote"

	LabelWorkerName = "worker_name"
	LabelStage      = "stage"
)

var (
	once = sync.Once{}

	WorkersCapacity = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "workers_capacity",
	})
	WorkersAvailable = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "workers_available",
	})
	WorkersHeartbeats = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "workers_heartbeats",
	}, []string{LabelWorkerName})

	WorkersSpentSeconds = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "workers_spent_seconds",
	}, []string{LabelWorkerName, LabelStage})

	TranscodedSeconds = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "transcoded_seconds",
	}, []string{LabelWorkerName})
	TranscodedSizeMB = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "transcoded_size_mb",
	}, []string{LabelWorkerName})

	TranscodingRequestsPublished = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "transcoding_requests_published",
	})
	TranscodingRequestsRunning = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "transcoding_requests_running",
	}, []string{LabelWorkerName})
	TranscodingRequestsDone = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "transcoding_requests_done",
	}, []string{LabelWorkerName})

	TranscodingRequestsRetries = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "transcoding_requests_retries",
	}, []string{LabelWorkerName})
	TranscodingRequestsErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "transcoding_requests_errors",
	}, []string{LabelWorkerName, LabelStage})
)

func RegisterMetrics() {
	once.Do(func() {
		prometheus.MustRegister(
			WorkersCapacity, WorkersAvailable, WorkersSpentSeconds,
			TranscodedSeconds, TranscodedSizeMB,
			TranscodingRequestsRunning, TranscodingRequestsRetries, TranscodingRequestsErrors, TranscodingRequestsDone,
		)
	})

}
