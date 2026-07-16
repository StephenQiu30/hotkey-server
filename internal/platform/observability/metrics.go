package observability

import (
	stdhttp "net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Metrics struct {
	Registry         *prometheus.Registry
	httpRequests     *prometheus.CounterVec
	httpDuration     *prometheus.HistogramVec
	httpPanics       *prometheus.CounterVec
	dependencyHealth *prometheus.GaugeVec
	collectionOps    *prometheus.CounterVec
}

func NewMetrics() (*Metrics, error) {
	metrics := &Metrics{
		Registry: prometheus.NewRegistry(),
		httpRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "hotkey_http_requests_total",
			Help: "Total HTTP requests handled by HotKey.",
		}, []string{"method", "route", "status"}),
		httpDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "hotkey_http_request_duration_seconds",
			Help: "HTTP request duration handled by HotKey.",
		}, []string{"method", "route", "status"}),
		httpPanics: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "hotkey_http_panics_total",
			Help: "Total recovered HTTP panics in HotKey.",
		}, []string{"route"}),
		dependencyHealth: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "hotkey_dependency_health",
			Help: "Health of a HotKey dependency, where 1 is healthy and 0 is unhealthy.",
		}, []string{"dependency"}),
		collectionOps: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "hotkey_collection_operations_total",
			Help: "Total collection administration operations by stable operation and outcome.",
		}, []string{"operation", "outcome"}),
	}
	if err := metrics.Registry.Register(metrics.httpRequests); err != nil {
		return nil, err
	}
	if err := metrics.Registry.Register(metrics.httpDuration); err != nil {
		return nil, err
	}
	if err := metrics.Registry.Register(metrics.httpPanics); err != nil {
		return nil, err
	}
	if err := metrics.Registry.Register(metrics.dependencyHealth); err != nil {
		return nil, err
	}
	if err := metrics.Registry.Register(metrics.collectionOps); err != nil {
		return nil, err
	}
	return metrics, nil
}

func (metrics *Metrics) Handler() stdhttp.Handler {
	return promhttp.HandlerFor(metrics.Registry, promhttp.HandlerOpts{})
}

func (metrics *Metrics) RecordHTTPRequest(method, route string, status int, duration time.Duration) {
	labels := []string{method, route, strconv.Itoa(status)}
	metrics.httpRequests.WithLabelValues(labels...).Inc()
	metrics.httpDuration.WithLabelValues(labels...).Observe(duration.Seconds())
}

func (metrics *Metrics) RecordPanic(route string) {
	metrics.httpPanics.WithLabelValues(route).Inc()
}

func (metrics *Metrics) SetDependencyHealth(dependency string, healthy float64) {
	metrics.dependencyHealth.WithLabelValues(dependency).Set(healthy)
}

// RecordCollectionOperation intentionally accepts only application-owned,
// low-cardinality labels. Callers must never supply source IDs, query text,
// endpoint values or arbitrary upstream diagnostics.
func (metrics *Metrics) RecordCollectionOperation(operation, outcome string) {
	metrics.collectionOps.WithLabelValues(operation, outcome).Inc()
}
