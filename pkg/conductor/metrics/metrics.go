package metrics

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	LabelWorkerName string = "worker_name"
	LabelStage             = "stage"

	StageAccepted     = "accepted"
	StageDownloading  = "downloading"
	StageEncoding     = "encoding"
	StageUploading    = "uploading"
	StageMetadataFill = "metadata_fill"
	StageLibraryAdd   = "library_add"
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
		)
	})
}
