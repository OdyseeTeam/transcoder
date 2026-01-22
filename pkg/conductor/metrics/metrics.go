package metrics

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	LabelWorkerName   string = "worker_name"
	LabelStage        string = "stage"
	StageAccepted     string = "accepted"
	StageDownloading  string = "downloading"
	StageEncoding     string = "encoding"
	StageUploading    string = "uploading"
	StageMetadataFill string = "metadata_fill"
	StageLibraryAdd   string = "library_add"
)

var (
	once = sync.Once{}

	RequestsPublished = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "requests_published",
	})
	RequestsCompleted = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "requests_completed",
	}, []string{LabelWorkerName})
	Capacity = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "capacity",
	}, []string{LabelWorkerName})
	Running = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "running",
	}, []string{LabelWorkerName})

	TranscodedSeconds = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "transcoded_seconds",
	})
	TranscodedCount = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "transcoded_count",
	})
	SpentSeconds = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "spent_seconds",
	}, []string{LabelStage})
	StageRunning = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "stage_running",
	}, []string{LabelStage})

	InputBytes = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "input_bytes",
	})
	OutputBytes = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "output_bytes",
	})

	ErrorsCount = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "errors_count",
	}, []string{LabelStage})

	DiskUsagePercent = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "transcoder_worker_disk_usage_percent",
		Help: "Current disk usage percentage on the monitored path",
	})
	DiskWaitTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "transcoder_worker_disk_wait_total",
		Help: "Number of times a job had to wait for disk space",
	})
	DiskWaitTimeoutTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "transcoder_worker_disk_wait_timeout_total",
		Help: "Number of times disk wait timed out",
	})
)

func RegisterConductorMetrics() {
	once.Do(func() {
		prometheus.MustRegister(
			RequestsPublished, RequestsCompleted, Capacity, Running)
	})
}

func RegisterWorkerMetrics() {
	once.Do(func() {
		prometheus.MustRegister(
			TranscodedSeconds, TranscodedCount,
			SpentSeconds, StageRunning,
			InputBytes, OutputBytes,
			ErrorsCount,
			DiskUsagePercent, DiskWaitTotal, DiskWaitTimeoutTotal,
		)
	})
}
