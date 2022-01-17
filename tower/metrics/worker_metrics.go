package metrics

import "github.com/prometheus/client_golang/prometheus"

const (
	WorkerStatusAvailable = "available"
	WorkerStatusCapacity  = "capacity"
)

var (
	WorkerCapability = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "worker_capability",
	}, []string{"status"})

	TranscodedSeconds = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "transcoded_seconds",
	})

	PipelineSpentSeconds = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "pipeline_spent_seconds",
	}, []string{LabelStage})
	PipelineStagesRunning = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "pipeline_stages_running",
	}, []string{LabelStage})

	InputBytes = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "input_bytes",
	})
	OutputBytes = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "output_bytes",
	})

	TranscodedStreamsCount = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "done_streams_count",
	})
	TranscodingErrorsCount = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "errors_count",
	}, []string{LabelStage})
)

func RegisterWorkerMetrics() {
	once.Do(func() {
		prometheus.MustRegister(
			WorkerCapability,
			TranscodedSeconds,
			PipelineStagesRunning, PipelineSpentSeconds,
			InputBytes, OutputBytes,
			TranscodedStreamsCount, TranscodingErrorsCount,
		)
	})
}
