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
)
