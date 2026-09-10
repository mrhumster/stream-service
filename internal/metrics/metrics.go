// Package metrics holds stream-service business metric counters.
package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
	Uploads = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "stream_uploads_total",
		Help: "Total number of upload requests by stage.",
	}, []string{"stage"})

	Lifecycle = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "stream_lifecycle_total",
		Help: "Total number of stream lifecycle events.",
	}, []string{"event"})

	HLSRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "stream_hls_requests_total",
		Help: "Total number of HLS segment requests by HTTP status.",
	}, []string{"status"})
)

func init() {
	prometheus.MustRegister(Uploads, Lifecycle, HLSRequests)
}