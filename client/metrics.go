package client

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	resultForbidden   = ""
	resultNotFound    = ""
	resultUnderway    = ""
	resultFound       = ""
	resultDownloading = ""
	resultLocalCache  = ""
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
