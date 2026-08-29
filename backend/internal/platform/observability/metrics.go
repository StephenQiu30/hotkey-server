package observability

import (
	stdhttp "net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Metrics struct {
	Registry          *prometheus.Registry
	httpRequests      *prometheus.CounterVec
	httpDuration      *prometheus.HistogramVec
	httpPanics        *prometheus.CounterVec
	dependencyHealth  *prometheus.GaugeVec
	operationalAlerts *prometheus.GaugeVec
	collectionOps     *prometheus.CounterVec
	contentQueries    *prometheus.CounterVec
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
		operationalAlerts: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "hotkey_operational_alert",
			Help: "Active bounded HotKey operational alerts, where 1 is active and 0 is clear.",
		}, []string{"alert_id", "policy_version", "severity", "owner", "silence_key"}),
		collectionOps: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "hotkey_collection_operations_total",
			Help: "Total collection administration operations by stable operation and outcome.",
		}, []string{"operation", "outcome"}),
		contentQueries: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "hotkey_content_query_operations_total",
			Help: "Total safe Content query operations by stable operation and outcome.",
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
	if err := metrics.Registry.Register(metrics.operationalAlerts); err != nil {
		return nil, err
	}
	if err := metrics.Registry.Register(metrics.collectionOps); err != nil {
		return nil, err
	}
	if err := metrics.Registry.Register(metrics.contentQueries); err != nil {
		return nil, err
	}
	return metrics, nil
}

func (metrics *Metrics) Handler() stdhttp.Handler {
	return promhttp.HandlerFor(metrics.Registry, promhttp.HandlerOpts{})
}

func (metrics *Metrics) RegisterCollector(collector prometheus.Collector) error {
	if metrics == nil || metrics.Registry == nil || collector == nil {
		return nil
	}
	return metrics.Registry.Register(collector)
}

func (metrics *Metrics) RecordHTTPRequest(method, route string, status int, duration time.Duration) {
	labels := []string{SafeHTTPMethod(method), SafeHTTPRoute(route), SafeHTTPStatus(status)}
	metrics.httpRequests.WithLabelValues(labels...).Inc()
	metrics.httpDuration.WithLabelValues(labels...).Observe(duration.Seconds())
}

func (metrics *Metrics) RecordPanic(route string) {
	metrics.httpPanics.WithLabelValues(SafeHTTPRoute(route)).Inc()
}

func (metrics *Metrics) SetDependencyHealth(dependency string, healthy float64) {
	if !allowedDependencyLabels[dependency] {
		dependency = "unknown"
	}
	if healthy != 1 {
		healthy = 0
	}
	metrics.dependencyHealth.WithLabelValues(dependency).Set(healthy)
}

// SetOperationalAlert accepts only finite policy-owned labels. Correlation IDs,
// dependency errors and user-controlled values must remain outside Metric labels.
func (metrics *Metrics) SetOperationalAlert(alertID, policyVersion, severity, owner string, active bool) {
	alertID = boundedLabel(alertID, allowedOperationalAlertIDs)
	policyVersion = boundedLabel(policyVersion, allowedOperationalAlertPolicyVersions)
	severity = boundedLabel(severity, allowedOperationalAlertSeverities)
	owner = boundedLabel(owner, allowedOperationalAlertOwners)
	value := float64(0)
	if active {
		value = 1
	}
	metrics.operationalAlerts.WithLabelValues(alertID, policyVersion, severity, owner, alertID).Set(value)
}

// RecordCollectionOperation intentionally accepts only application-owned,
// low-cardinality labels. Callers must never supply source IDs, query text,
// endpoint values or arbitrary upstream diagnostics.
func (metrics *Metrics) RecordCollectionOperation(operation, outcome string) {
	metrics.collectionOps.WithLabelValues(
		boundedLabel(operation, allowedCollectionOperations),
		boundedLabel(outcome, allowedCollectionOutcomes),
	).Inc()
}

// RecordContentQuery accepts only stable operation/outcome labels. Content
// IDs, source names, URL fragments and error text are deliberately excluded.
func (metrics *Metrics) RecordContentQuery(operation, outcome string) {
	metrics.contentQueries.WithLabelValues(
		boundedLabel(operation, allowedContentQueryOperations),
		boundedLabel(outcome, allowedContentQueryOutcomes),
	).Inc()
}

var allowedAPIRouteFamilies = map[string]bool{
	"ai": true, "auth": true, "capabilities": true, "collection-runs": true,
	"content-lineage-decisions": true, "contents": true, "document-versions": true,
	"hotspots": true, "knowledge": true, "metric-capability-profiles": true,
	"micro-events": true, "monitors": true, "notifications": true, "operations": true,
	"reports": true, "search": true, "source-connections": true, "source-endpoints": true,
	"source-presets": true, "source-webhooks": true, "users": true,
}

var allowedDependencyLabels = map[string]bool{
	"runtime": true, "database": true, "redis": true, "minio": true, "vault": true,
	"worker": true, "agent": true, "codex": true, "smtp": true,
}

var allowedOperationalAlertIDs = map[string]bool{
	"ALERT-DB-UNAVAILABLE": true, "ALERT-RIVER-JOB-FAILED": true, "ALERT-RIVER-NO-WORKER": true,
	"ALERT-SOURCE-AUTH": true, "ALERT-MINIO-WRITE": true, "ALERT-CODEX-FAILURE": true,
	"ALERT-VAULT-CONFLICT": true, "ALERT-BACKUP-FAILED": true, "ALERT-SEARCH-BACKLOG": true,
	"ALERT-DELIVERY-UNKNOWN": true,
}
var allowedOperationalAlertPolicyVersions = map[string]bool{"p0-operational-alerts-v1": true}
var allowedOperationalAlertSeverities = map[string]bool{"p0": true, "p1": true, "p2": true, "p3": true}
var allowedOperationalAlertOwners = map[string]bool{"hotkey-oncall": true}

var allowedCollectionOperations = map[string]bool{"list": true, "manual": true, "retry": true, "health": true}
var allowedCollectionOutcomes = map[string]bool{"success": true, "error": true, "healthy": true, "unhealthy": true}
var allowedContentQueryOperations = map[string]bool{
	"list_active": true, "get_active": true, "get_document": true, "delete_active": true,
}
var allowedContentQueryOutcomes = map[string]bool{
	"success": true, "invalid": true, "not_found": true, "unavailable": true, "error": true,
}

// SafeHTTPMethod collapses arbitrary request methods before they reach a
// searchable log field or Prometheus label.
func SafeHTTPMethod(method string) string {
	switch method {
	case stdhttp.MethodGet, stdhttp.MethodPost, stdhttp.MethodPut, stdhttp.MethodPatch,
		stdhttp.MethodDelete, stdhttp.MethodOptions, stdhttp.MethodHead:
		return method
	default:
		return "OTHER"
	}
}

// SafeHTTPRoute returns a finite API family instead of a concrete URL, path
// parameter, query string or unmatched user-controlled path.
func SafeHTTPRoute(route string) string {
	if strings.Contains(route, "://") {
		return "unmatched"
	}
	if boundary := strings.IndexAny(route, "?#"); boundary >= 0 {
		route = route[:boundary]
	}
	switch route {
	case "/healthz", "/readyz", "/metrics", "/openapi.json", "/docs", "/docs/*any":
		return "/platform"
	}
	const prefix = "/api/v1/"
	if !strings.HasPrefix(route, prefix) {
		return "unmatched"
	}
	family := strings.SplitN(strings.TrimPrefix(route, prefix), "/", 2)[0]
	if !allowedAPIRouteFamilies[family] {
		return "/api/v1/other"
	}
	return prefix + family
}

// SafeHTTPStatus converts the finite HTTP status space to a stable string.
func SafeHTTPStatus(status int) string {
	if status < 100 || status > 599 {
		return "other"
	}
	return strconv.Itoa(status)
}

func boundedLabel(value string, allowed map[string]bool) string {
	if allowed[value] {
		return value
	}
	return "unknown"
}
