package domain

import (
	"context"
	"time"
)

// Connector is deliberately small and protocol-neutral. Implementations may
// perform source-specific HTTP work, but can only return normalized SourceItem
// values and safe request metadata to the Source application.
type Connector interface {
	Validate(context.Context, SourceConnection) error
	Fetch(context.Context, FetchRequest) (FetchResult, error)
	Health(context.Context, SourceConnection) HealthResult
}

type FetchResult struct {
	Items        []SourceItem
	NextCursor   string
	ETag         string
	LastModified string
	HasMore      bool
	RateLimit    RateLimit
	ServerTime   *time.Time
	Diagnostics  []FetchDiagnostic
}

// FetchDiagnostic carries only a stable code and optional source item ID. It
// deliberately has no message/raw-response field, so transport details cannot
// cross the Connector boundary as diagnostics.
type FetchDiagnostic struct {
	Code             string
	SourceExternalID string
}

type RateLimit struct {
	Remaining  int
	ResetAt    *time.Time
	RetryAfter *time.Time
}

type HealthResult struct {
	Healthy        bool
	CheckedAt      time.Time
	ErrorKind      CollectionErrorKind
	DiagnosticCode string
}
