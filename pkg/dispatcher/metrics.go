package dispatcher

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
	DispatcherQueueLength = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "dispatcher_queue_length",
	})
	DispatcherTasksActive = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "dispatcher_tasks_active",
	})
	DispatcherTasksQueued = promauto.NewCounter(prometheus.CounterOpts{
		Name: "dispatcher_tasks_queued",
	})
	DispatcherTasksDropped = promauto.NewCounter(prometheus.CounterOpts{
		Name: "dispatcher_tasks_dropped",
	})
	DispatcherTasksDone = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "dispatcher_tasks_done",
	}, []string{"worker_id"})
	DispatcherTasksFailed = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "dispatcher_tasks_failed",
	}, []string{"worker_id"})
)
