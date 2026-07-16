package domain

import (
	"fmt"
	"strings"
	"time"
)

const CapturedItemVersionV1 = "v1"

type RawPayloadDisposition string

const (
	RawPayloadDiscarded        RawPayloadDisposition = "discarded"
	RawPayloadCapturedItemOnly RawPayloadDisposition = "captured_item_only"
)

func (disposition RawPayloadDisposition) Valid() bool {
	return disposition == RawPayloadDiscarded || disposition == RawPayloadCapturedItemOnly
}

// FetchRequest is the protocol-neutral request for one shared collection run.
// It intentionally carries no Monitor target identity: targets consume the
// resulting captured items after the one external request has completed.
type FetchRequest struct {
	CollectionRunID    int64
	SourceConnectionID int64
	QuerySignature     string
	Query              string
	Languages          []string
	Regions            []string
	WindowStart        time.Time
	WindowEnd          time.Time
	RequestCursor      string
	ETag               string
	LastModified       string
	Limit              int
}

func (request FetchRequest) Validate() error {
	if request.CollectionRunID <= 0 || request.SourceConnectionID <= 0 {
		return fmt.Errorf("collection run and source connection are required")
	}
	if !validSHA256(request.QuerySignature) {
		return fmt.Errorf("query signature must be a SHA-256 hex value")
	}
	if request.WindowStart.IsZero() || request.WindowEnd.IsZero() || !request.WindowEnd.After(request.WindowStart) {
		return fmt.Errorf("collection window is invalid")
	}
	if request.Limit < 1 || request.Limit > 1000 {
		return fmt.Errorf("collection fetch limit must be 1-1000")
	}
	return nil
}

// SourceItem is the stable Connector output. RawPayload exists only while a
// Connector call is being processed; CapturePolicy intentionally never copies
// it into a CapturedItem.
type SourceItem struct {
	SourceCode  string
	ExternalID  string
	ContentType string
	Title       string
	Body        string
	Language    string
	URL         string
	Author      string
	PublishedAt *time.Time
	ObservedAt  time.Time
	Metrics     SourceMetrics
	RawPayload  []byte
}

type SourceMetrics struct {
	ViewCount    int64
	LikeCount    int64
	CommentCount int64
	ShareCount   int64
}

func (metrics SourceMetrics) Validate() error {
	if metrics.ViewCount < 0 || metrics.LikeCount < 0 || metrics.CommentCount < 0 || metrics.ShareCount < 0 {
		return fmt.Errorf("source metrics cannot be negative")
	}
	return nil
}

func NormalizeSourceItem(item SourceItem) (SourceItem, error) {
	item.SourceCode = strings.ToLower(strings.TrimSpace(item.SourceCode))
	item.ExternalID = strings.TrimSpace(item.ExternalID)
	item.ContentType = strings.ToLower(strings.TrimSpace(item.ContentType))
	item.Title = strings.TrimSpace(item.Title)
	item.Body = strings.TrimSpace(item.Body)
	item.Language = strings.TrimSpace(item.Language)
	item.URL = strings.TrimSpace(item.URL)
	item.Author = strings.TrimSpace(item.Author)
	if item.SourceCode == "" || len(item.SourceCode) > 64 {
		return SourceItem{}, fmt.Errorf("source code must be 1-64 bytes")
	}
	if item.ExternalID == "" || len(item.ExternalID) > 512 {
		return SourceItem{}, fmt.Errorf("source item requires a stable external ID")
	}
	if item.ContentType == "" || len(item.ContentType) > 32 {
		return SourceItem{}, fmt.Errorf("source item content type is invalid")
	}
	if item.ObservedAt.IsZero() {
		return SourceItem{}, fmt.Errorf("source item observed time is required")
	}
	if item.Language != "" {
		languages, err := normalizeLanguages([]string{item.Language}, 1, 1)
		if err != nil {
			return SourceItem{}, fmt.Errorf("normalize source item language: %w", err)
		}
		item.Language = languages[0]
	}
	if err := item.Metrics.Validate(); err != nil {
		return SourceItem{}, err
	}
	return item, nil
}

// CapturePolicy centralizes the durable, versioned projection from a transient
// SourceItem. Source connectors therefore cannot select their own persistence
// shape or retain raw upstream bytes.
type CapturePolicy struct {
	Version               string
	AllowBodyStorage      bool
	RawPayloadDisposition RawPayloadDisposition
}

func (policy CapturePolicy) Validate() error {
	if policy.Version != CapturedItemVersionV1 {
		return fmt.Errorf("unsupported captured item version %q", policy.Version)
	}
	if !policy.RawPayloadDisposition.Valid() {
		return fmt.Errorf("raw payload disposition is invalid")
	}
	return nil
}

type CapturedItem struct {
	Version               string
	SourceCode            string
	ExternalID            string
	ContentType           string
	Title                 string
	Body                  string
	Language              string
	URL                   string
	Author                string
	PublishedAt           *time.Time
	ObservedAt            time.Time
	Metrics               SourceMetrics
	RawPayloadDisposition RawPayloadDisposition
	RawPayload            []byte
}

func (policy CapturePolicy) Capture(item SourceItem) (CapturedItem, error) {
	if err := policy.Validate(); err != nil {
		return CapturedItem{}, err
	}
	normalized, err := NormalizeSourceItem(item)
	if err != nil {
		return CapturedItem{}, err
	}
	captured := CapturedItem{
		Version: policy.Version, SourceCode: normalized.SourceCode, ExternalID: normalized.ExternalID,
		ContentType: normalized.ContentType, Title: normalized.Title, Language: normalized.Language,
		URL: normalized.URL, Author: normalized.Author, ObservedAt: normalized.ObservedAt,
		Metrics: normalized.Metrics, RawPayloadDisposition: policy.RawPayloadDisposition,
	}
	if normalized.PublishedAt != nil {
		publishedAt := normalized.PublishedAt.UTC()
		captured.PublishedAt = &publishedAt
	}
	if policy.AllowBodyStorage {
		captured.Body = normalized.Body
	}
	return captured, nil
}

type CollectionTerm struct {
	Value    string
	Excluded bool
}

// PublishedCollectionTarget is the Source-facing projection of one immutable
// published Monitor association. It is deliberately not a Monitor record.
type PublishedCollectionTarget struct {
	MonitorSourceID        int64
	MonitorConfigVersionID int64
	SourceConnectionID     int64
	QuerySignature         string
	QueryOverride          string
	Terms                  []CollectionTerm
	Languages              []string
	Regions                []string
	CollectionInterval     time.Duration
	Checkpoint             CollectionCheckpoint
}

func (target PublishedCollectionTarget) Validate() error {
	if target.MonitorSourceID <= 0 || target.MonitorConfigVersionID <= 0 || target.SourceConnectionID <= 0 {
		return fmt.Errorf("published collection target ownership is required")
	}
	if !validSHA256(target.QuerySignature) {
		return fmt.Errorf("published collection target requires a query signature")
	}
	if target.CollectionInterval < 5*time.Minute || target.CollectionInterval > 24*time.Hour || target.CollectionInterval%time.Minute != 0 {
		return fmt.Errorf("published collection interval is invalid")
	}
	if target.Checkpoint.MonitorSourceID != target.MonitorSourceID {
		return fmt.Errorf("checkpoint must belong to the published monitor source")
	}
	if target.Checkpoint.QueryHash != target.QuerySignature {
		return fmt.Errorf("checkpoint query hash must match the published query signature")
	}
	if err := target.Checkpoint.Validate(); err != nil {
		return err
	}
	for _, term := range target.Terms {
		if strings.TrimSpace(term.Value) == "" {
			return fmt.Errorf("collection term is required")
		}
	}
	return nil
}

type CollectionRequest struct {
	SourceConnectionID int64
	QuerySignature     string
	Query              string
	Languages          []string
	Regions            []string
	WindowStart        time.Time
	WindowEnd          time.Time
	Targets            []PublishedCollectionTarget
}

func (request CollectionRequest) Validate() error {
	if request.SourceConnectionID <= 0 || !validSHA256(request.QuerySignature) {
		return fmt.Errorf("collection request source and query signature are required")
	}
	if strings.TrimSpace(request.Query) == "" {
		return fmt.Errorf("collection request query is required")
	}
	if request.WindowStart.IsZero() || request.WindowEnd.IsZero() || !request.WindowEnd.After(request.WindowStart) {
		return fmt.Errorf("collection request window is invalid")
	}
	if len(request.Targets) == 0 {
		return fmt.Errorf("collection request requires at least one target")
	}
	for _, target := range request.Targets {
		if err := target.Validate(); err != nil {
			return err
		}
		if target.SourceConnectionID != request.SourceConnectionID || target.QuerySignature != request.QuerySignature {
			return fmt.Errorf("collection request target does not share request identity")
		}
	}
	return nil
}

type CollectionRunStatus string

const (
	CollectionRunQueued    CollectionRunStatus = "queued"
	CollectionRunRunning   CollectionRunStatus = "running"
	CollectionRunSucceeded CollectionRunStatus = "succeeded"
	CollectionRunFailed    CollectionRunStatus = "failed"
	CollectionRunCancelled CollectionRunStatus = "cancelled"
)

type CollectionRun struct {
	ID                 int64
	SourceConnectionID int64
	QuerySignature     string
	RequestCursor      string
	NextCursor         string
	ETag               string
	LastModified       string
	RetryAfter         *time.Time
	PageCount          int
	WindowStart        time.Time
	WindowEnd          time.Time
	Status             CollectionRunStatus
}

type CollectionTarget struct {
	ID                     int64
	CollectionRunID        int64
	MonitorSourceID        int64
	MonitorConfigVersionID int64
	Status                 CollectionRunStatus
}

func (target CollectionTarget) Validate() error {
	if target.CollectionRunID <= 0 || target.MonitorSourceID <= 0 || target.MonitorConfigVersionID <= 0 {
		return fmt.Errorf("collection target requires run and immutable published configuration")
	}
	return nil
}

type CollectionTargetItem struct {
	ID                    int64
	CollectionRunID       int64
	CollectionRunTargetID int64
	CollectionRunItemID   int64
	Outcome               string
	ReasonCode            string
}

type CollectionCheckpoint struct {
	ID                  int64
	Version             int64
	MonitorSourceID     int64
	QueryHash           string
	CursorValue         string
	ETag                string
	LastModified        string
	HighWatermark       *time.Time
	LastSuccessfulRunID *int64
	LastFetchedAt       *time.Time
	NextPollAt          time.Time
	ConsecutiveFailures int
}

func (checkpoint CollectionCheckpoint) Validate() error {
	if checkpoint.MonitorSourceID <= 0 || !validSHA256(checkpoint.QueryHash) || checkpoint.NextPollAt.IsZero() {
		return fmt.Errorf("collection checkpoint is incomplete")
	}
	if checkpoint.ConsecutiveFailures < 0 {
		return fmt.Errorf("collection checkpoint failures cannot be negative")
	}
	return nil
}

func validSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if !(character >= '0' && character <= '9' || character >= 'a' && character <= 'f') {
			return false
		}
	}
	return true
}
