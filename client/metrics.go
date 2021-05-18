package client

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	resultForbidden = "forbidden"
	resultNotFound  = "not_found"
	resultUnderway  = "underway"
	resultFound     = "found"

	fetchSourceRemote = "remote"
	fetchSourceLocal  = "local"
)

var (
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
	}, []string{"source", "http_code"})
)

func RegisterMetrics() {
	once := sync.Once{}
	once.Do(func() {
		prometheus.MustRegister(
			TranscodedCacheSizeBytes, TranscodedCacheItemsCount, TranscodedResult,
			TranscodedCacheQueryCount, TranscodedCacheMiss, FetchSizeBytes, FetchDurationSeconds,
			FetchCount, FetchFailureCount,
		)
	})
}
