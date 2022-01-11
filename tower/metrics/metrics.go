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

	WorkersCapacity = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "workers_capacity",
	}, []string{LabelWorkerName})

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

	TranscodingRequestsBackupDone = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "transcoding_requests_backup_done",
	}, []string{LabelWorkerName})

	TranscodingRequestsRetries = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "transcoding_requests_retries",
	}, []string{LabelWorkerName})
	TranscodingRequestsErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "transcoding_requests_errors",
	}, []string{LabelWorkerName})

	PipelineStagesRunning = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "transcoding_pipeline_stages_running",
	}, []string{LabelWorkerName, LabelStage})
)

func RegisterTowerMetrics() {
	once.Do(func() {
		prometheus.MustRegister(
			WorkersAvailable, WorkersSpentSeconds,
			TranscodedSeconds, TranscodedSizeMB,
			TranscodingRequestsRunning, TranscodingRequestsRetries, TranscodingRequestsErrors, TranscodingRequestsDone, TranscodingRequestsBackupDone,
		)
	})
}

func RegisterWorkerMetrics() {
	once.Do(func() {
		prometheus.MustRegister(
			WorkersCapacity, PipelineStagesRunning,
		)
	})
}
