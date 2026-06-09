package adapter

import (
	"errors"
	"time"
)

// Provider identifies a content platform type.
type Provider string

const (
	ProviderRSS         Provider = "rss"
	ProviderPublicPage  Provider = "public_page"
	ProviderOfficialAPI Provider = "official_api"
	ProviderWeibo       Provider = "weibo"
	ProviderZhihu       Provider = "zhihu"
	ProviderXiaohongshu Provider = "xiaohongshu"
	ProviderYouTube     Provider = "youtube"
	ProviderBilibili    Provider = "bilibili"
)

// HealthStatus describes adapter operational readiness.
type HealthStatus string

const (
	HealthStatusHealthy   HealthStatus = "healthy"
	HealthStatusDegraded  HealthStatus = "degraded"
	HealthStatusUnhealthy HealthStatus = "unhealthy"
)

// FailureClass categorizes adapter errors for upstream handling.
type FailureClass string

const (
	FailureClassAuth       FailureClass = "auth"
	FailureClassRateLimit  FailureClass = "rate_limit"
	FailureClassTransient  FailureClass = "transient"
	FailureClassPermanent  FailureClass = "permanent"
	FailureClassParseError FailureClass = "parse_error"
)

// AdapterError wraps an error with a failure classification.
type AdapterError struct {
	Class   FailureClass
	Message string
	Cause   error
}

func (e *AdapterError) Error() string {
	if e.Cause != nil {
		return e.Message + ": " + e.Cause.Error()
	}
	return e.Message
}

func (e *AdapterError) Unwrap() error {
	return e.Cause
}

// NewAdapterError creates an AdapterError.
func NewAdapterError(class FailureClass, message string, cause error) *AdapterError {
	return &AdapterError{Class: class, Message: message, Cause: cause}
}

// IsAdapterError checks if an error is an AdapterError with the given class.
func IsAdapterError(err error, class FailureClass) bool {
	var ae *AdapterError
	if !errors.As(err, &ae) {
		return false
	}
	return ae.Class == class
}

// Capabilities describes what an adapter supports.
type Capabilities struct {
	SupportsIncremental bool
	MaxItemsPerFetch    int
	RateLimitPerHour    int
}

// CollectInput is the input for a collection operation.
type CollectInput struct {
	SourceID       string
	Provider       Provider
	URL            string
	Since          *time.Time
	IdempotencyKey string
}

// NormalizedItem is the unified content schema produced by adapters.
type NormalizedItem struct {
	Title          string
	URL            string
	Snippet        string
	ExternalID     string
	PublishedAt    *time.Time
	Language       string
	IdempotencyKey string
	// Metadata holds platform-specific key-value pairs (author, score, etc.).
	Metadata     map[string]string
	MetadataOnly bool
}

// CollectOutput is the result of a collection operation.
type CollectOutput struct {
	Items     []NormalizedItem
	HasMore   bool
	NextSince *time.Time
}

// HealthInfo reports adapter health status.
type HealthInfo struct {
	Status        HealthStatus
	LastError     string
	LastCheckedAt time.Time
}

// Adapter is the unified interface for all content platform adapters.
type Adapter interface {
	// Name returns a human-readable adapter name.
	Name() string

	// Provider returns the provider type this adapter handles.
	Provider() Provider

	// Collect fetches content items from the platform.
	Collect(input CollectInput) (CollectOutput, error)

	// Health returns the current health status of the adapter.
	Health() HealthInfo

	// Capabilities returns what this adapter supports.
	Capabilities() Capabilities
}
