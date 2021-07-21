package client

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	failureForbidden   = "forbidden"
	failureNotFound    = "not_found"
	failureTransport   = "transport_error"
	failureServerError = "server_error"

	resultUnderway = "underway"
	resultFound    = "found"

	fetchSourceRemote = "remote"
	fetchSourceLocal  = "local"
)

var (
	once = sync.Once{}

	TranscodedCacheSizeBytes = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "transcoded_cache_size_bytes",
	})
	TranscodedCacheItemsCount = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "transcoded_cache_items_count",
	})
	TranscodedResult = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "transcoded_request_result",
	}, []string{"type"})

	TranscodedCacheQueryCount = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "transcoded_cache_query_count",
	})
	TranscodedCacheMiss = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "transcoded_cache_miss",
	})
	TranscodedCacheRetry = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "transcoded_cache_retry",
	})

	FetchSizeBytes = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "fetch_size_bytes",
	}, []string{"source"})
	FetchDurationSeconds = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "fetch_duration_seconds",
	}, []string{"source"})
	FetchCount = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "fetch_count",
	}, []string{"source"})
	FetchFailureCount = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "fetch_failure_count",
	}, []string{"source", "type"})
)

func RegisterMetrics() {
	once.Do(func() {
		prometheus.MustRegister(
			TranscodedCacheSizeBytes, TranscodedCacheItemsCount, TranscodedResult,
			TranscodedCacheQueryCount, TranscodedCacheMiss, TranscodedCacheRetry,
			FetchSizeBytes, FetchDurationSeconds,
			FetchCount, FetchFailureCount,
		)
	})
}
