package client

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	resultForbidden   = "forbidden"
	resultNotFound    = "not_found"
	resultUnderway    = "underway"
	resultFound       = "found"
	resultDownloading = "downloading"
	resultLocalCache  = "local_cache"

	fetchSourceRemote = "remote"
	fetchSourceLocal  = "local"
)

var (
	TranscodedCacheSizeBytes = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "transcoded_cache_size_bytes",
	})
	TranscodedCacheItemsCount = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "transcoded_cache_items_count",
	})
	TranscodedResult = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "transcoded_request_result",
	}, []string{"type"})

	TranscodedCacheQueryCount = promauto.NewCounter(prometheus.CounterOpts{
		Name: "transcoded_cache_query_count",
	})
	TranscodedCacheMiss = promauto.NewCounter(prometheus.CounterOpts{
		Name: "transcoded_cache_miss",
	})

	FetchSizeBytes = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "fetch_size_bytes",
	}, []string{"source"})
	FetchCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "fetch_count",
	}, []string{"source"})
	FetchFailureCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "fetch_failure_count",
	}, []string{"source", "http_code"})
)
