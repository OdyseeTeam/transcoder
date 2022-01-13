package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
	WorkersCapacity = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "workers_capacity",
	})

	TranscodedSeconds = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "transcoded_seconds",
	})
	EncodingSpentSeconds = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "encoding_spent_seconds",
	})

	InputBytes = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "input_bytes",
	})
	OutputBytes = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "output_bytes",
	})

	TranscodedStreamsCount = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "transcoded_streams_count",
	})

	TranscodingErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "transcoding_error_count",
	}, []string{LabelStage})

	PipelineStagesRunning = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "transcoding_pipeline_stages_running",
	}, []string{LabelStage})
)

func RegisterWorkerMetrics() {
	once.Do(func() {
		prometheus.MustRegister(
			WorkersCapacity, PipelineStagesRunning,
			EncodingSpentSeconds, TranscodedSeconds,
			InputBytes, OutputBytes,
			TranscodedStreamsCount, TranscodingErrors,
		)
	})
}
