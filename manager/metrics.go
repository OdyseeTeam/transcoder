package manager

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	once = sync.Once{}

	QueueLength = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "transcoding_queue_length",
		Help: "Video queue length",
	}, []string{"queue"})

	QueueHits = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "transcoding_queue_hits",
		Help: "Video queue hits",
	}, []string{"queue"})

	QueueItemAge = prometheus.NewSummaryVec(prometheus.SummaryOpts{
		Name: "transcoding_queue_item_age_seconds",
		Help: "Age of queue items before they get processed",
	}, []string{"queue"})
)

func RegisterMetrics() {
	once.Do(func() {
		prometheus.MustRegister(QueueLength, QueueHits, QueueItemAge)
	})
}
