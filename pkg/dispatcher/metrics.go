package dispatcher

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	failureForbidden  = "forbidden"
	failureNotFound   = "not_found"
	resultUnderway    = "underway"
	resultFound       = "found"
	resultDownloading = "downloading"
	resultLocalCache  = "local_cache"
)

var (
	once = sync.Once{}

	DispatcherQueueLength = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "dispatcher_queue_length",
	})
	DispatcherTasksActive = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "dispatcher_tasks_active",
	})
	DispatcherTasksQueued = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "dispatcher_tasks_queued",
	})
	DispatcherTasksDropped = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "dispatcher_tasks_dropped",
	})
	DispatcherTasksDone = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "dispatcher_tasks_done",
	}, []string{"worker_id"})
	DispatcherTasksFailed = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "dispatcher_tasks_failed",
	}, []string{"worker_id"})
)

func RegisterMetrics() {
	once.Do(func() {
		prometheus.MustRegister(
			DispatcherQueueLength, DispatcherTasksActive, DispatcherTasksQueued,
			DispatcherTasksDropped, DispatcherTasksDone, DispatcherTasksFailed,
		)
	})
}
