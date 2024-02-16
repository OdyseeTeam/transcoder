package dispatcher

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
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
	}, []string{"agent_id"})
	DispatcherTasksFailed = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "dispatcher_tasks_failed",
	}, []string{"agent_id"})
)

func RegisterMetrics() {
	once.Do(func() {
		prometheus.MustRegister(
			DispatcherQueueLength, DispatcherTasksActive, DispatcherTasksQueued,
			DispatcherTasksDropped, DispatcherTasksDone, DispatcherTasksFailed,
		)
	})
}
