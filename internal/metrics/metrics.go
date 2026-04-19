package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
	RequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cerbai_requests_total",
			Help: "Total number of HTTP requests processed by the proxy.",
		},
		[]string{"method", "path", "status"},
	)

	RequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "cerbai_request_duration_seconds",
			Help:    "HTTP request latency in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)

	RequestsInFlight = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "cerbai_requests_in_flight",
		Help: "Current number of requests being processed by the proxy.",
	})
)

func init() {
	prometheus.MustRegister(RequestsTotal, RequestDuration, RequestsInFlight)
}
