package manager

import (
	"net/http"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/stretchr/testify/suite"
)

type httpSuite struct {
	suite.Suite
}

func TestHttp(t *testing.T) {
	suite.Run(t, new(httpSuite))
}

func (s *httpSuite) SetupSuite() {
}

func (s *httpSuite) TestPromHttp() {
	QueueLength.With(prometheus.Labels{"queue": "common"}).Inc()
	QueueItemAge.With(prometheus.Labels{"queue": "common"}).Observe(125.12)
	QueueHits.With(prometheus.Labels{"queue": "common"}).Inc()
	RegisterMetrics()
	s.HTTPBodyContains(promhttp.Handler().ServeHTTP, http.MethodGet, "/metrics", nil, "transcoding_queue_item_age_seconds")
	s.HTTPBodyContains(promhttp.Handler().ServeHTTP, http.MethodGet, "/metrics", nil, "transcoding_queue_length")
	s.HTTPBodyContains(promhttp.Handler().ServeHTTP, http.MethodGet, "/metrics", nil, "transcoding_queue_hits")
}
