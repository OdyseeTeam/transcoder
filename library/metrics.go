package library

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	LibraryBytes = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "library_total_bytes",
	})
	LibraryRetiredGB = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "library_retired_gb",
	})
	LibraryRetiredDuration = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "library_retired_duration_seconds",
	})
)

func RegisterMetrics() {
	prometheus.MustRegister(
		LibraryBytes, LibraryRetiredGB, LibraryRetiredDuration,
	)
}
