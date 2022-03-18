package library

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	LibraryBytes = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "library_total_bytes",
	})
	LibraryRetiredBytes = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "library_retired_bytes",
	})
)

func RegisterMetrics() {
	prometheus.MustRegister(
		LibraryBytes, LibraryRetiredBytes,
	)
}
