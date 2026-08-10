package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
	"golang.org/x/text/unicode/norm"
)

type ContentStatus string

const (
	ContentStatusActive    ContentStatus = "active"
	ContentStatusInvalid   ContentStatus = "invalid"
	ContentStatusDuplicate ContentStatus = "duplicate"
	ContentStatusDeleted   ContentStatus = "deleted"
	ContentStatusExpired   ContentStatus = "expired"
)

func (status ContentStatus) Valid() bool {
	switch status {
	case ContentStatusActive, ContentStatusInvalid, ContentStatusDuplicate, ContentStatusDeleted, ContentStatusExpired:
		return true
	default:
		return false
	}
}

const (
	DedupeReasonExactURL  = "exact_url"
	DedupeReasonExactHash = "exact_hash"
	DedupeReasonNearText  = "near_text"

	DedupeVersionExactURL  = "exact-url-v1"
	DedupeVersionExactHash = "exact-hash-v1"
	DedupeVersionNearText  = "near_text-v1"
)

// DedupeDecision records only a deterministic Content relationship. Active
// Content deliberately has no duplicate target, reason, or algorithm version.
type DedupeDecision struct {
	Status        ContentStatus
	DuplicateOfID *int64
	Reason        string
	Version       string
}

func (decision DedupeDecision) Validate() error {
	if !decision.Status.Valid() {
		return NewError(ErrorCodeInvalidDedupeDecision)
	}
	if decision.Status == ContentStatusDuplicate {
		if decision.DuplicateOfID == nil || *decision.DuplicateOfID <= 0 || !validDedupeReasonVersion(decision.Reason, decision.Version) {
			return NewError(ErrorCodeInvalidDedupeDecision)
		}
		return nil
	}
	if decision.DuplicateOfID != nil || decision.Reason != "" || decision.Version != "" {
		return NewError(ErrorCodeInvalidDedupeDecision)
	}
	return nil
}

func validDedupeReasonVersion(reason, version string) bool {
	switch reason {
	case DedupeReasonExactURL:
		return version == DedupeVersionExactURL
	case DedupeReasonExactHash:
		return version == DedupeVersionExactHash
	case DedupeReasonNearText:
		return version == DedupeVersionNearText
	default:
		return false
	}
}

type NormalizedAuthor struct {
	ExternalID  string
	DisplayName string
}

// NormalizeExternalID is the one opaque source-item identity canonicalization
// shared by capture normalization and Content persistence. It deliberately
// does not lowercase, parse, or otherwise reinterpret an upstream ID.
func NormalizeExternalID(value string) string {
	return strings.TrimSpace(norm.NFC.String(value))
}

// NormalizedContent is the pure projection from one persisted CapturedItem.
// Body remains available only because Source had already persisted it under its
// capture policy; no normalizer path fetches source data.
type NormalizedContent struct {
	SourceConnectionID int64
	ExternalID         string
	ContentType        string
	Title              string
	Excerpt            string
	Body               string
	ArchivedMarkdown   string
	CanonicalURL       string
	Language           string
	Author             NormalizedAuthor
	PublishedAt        time.Time
	FetchedAt          time.Time
	ContentHash        string
	Metrics            domain.SourceMetrics
}

func (content NormalizedContent) Validate() error {
	if content.SourceConnectionID <= 0 || strings.TrimSpace(content.ExternalID) == "" || strings.TrimSpace(content.ContentType) == "" || strings.TrimSpace(content.CanonicalURL) == "" || content.PublishedAt.IsZero() || content.FetchedAt.IsZero() || !validSHA256(content.ContentHash) {
		return NewError(ErrorCodeInvalidNormalizedContent)
	}
	if strings.TrimSpace(content.Title) == "" && strings.TrimSpace(content.Body) == "" {
		return NewError(ErrorCodeInvalidNormalizedContent)
	}
	return nil
}

// ContentCandidate is the bounded, already-normalized fact needed for a
// deterministic duplicate decision. Completeness is the repository-derived
// count of non-empty presentation facts; SourceExternalIDStable records that
// the source's external ID is a stable publisher/item identifier. They let a
// duplicate target be selected without touching another module's tables.
type ContentCandidate struct {
	ID                     int64
	SourceConnectionID     int64
	PublishedAt            time.Time
	TitleTokens            []string
	BodyTokens             []string
	CanonicalURL           string
	DedupeKey              string
	Completeness           int
	SourceExternalIDStable bool
}

func (candidate ContentCandidate) Validate() error {
	if candidate.ID <= 0 || candidate.SourceConnectionID <= 0 || candidate.PublishedAt.IsZero() || candidate.Completeness < 0 {
		return NewError(ErrorCodeInvalidContentCandidate)
	}
	return nil
}

type AssetStatus string

const (
	AssetStatusPending       AssetStatus = "pending"
	AssetStatusAvailable     AssetStatus = "available"
	AssetStatusMissing       AssetStatus = "missing"
	AssetStatusDeletePending AssetStatus = "delete_pending"
	AssetStatusDeleted       AssetStatus = "deleted"
)

type ContentAsset struct {
	ID          int64
	Version     int64
	ContentID   int64
	AssetType   string
	ObjectKey   string
	OriginalURL string
	MIMEType    string
	SHA256      string
	SizeBytes   int64
	CapturedAt  time.Time
	Status      AssetStatus
}

type Content struct {
	ID                 int64
	Version            int64
	SourceConnectionID int64
	// SourceType and SourceName are a safe, application-enriched read
	// projection. They are never persisted by ingestion or supplied by HTTP.
	SourceType    domain.SourceType
	SourceName    string
	ExternalID    string
	Author        NormalizedAuthor
	ContentType   string
	Title         string
	Excerpt       string
	CanonicalURL  string
	Language      string
	PublishedAt   time.Time
	FetchedAt     time.Time
	ContentHash   string
	Metrics       domain.SourceMetrics
	Status        ContentStatus
	DuplicateOfID *int64
	DedupeReason  string
	DedupeVersion string
	DeletedAt     *time.Time
	// Relevance and Event are optional safe query projections. They are not
	// persisted on Content and remain nil when no matching context exists.
	RelevanceScore *float64
	MatchDecision  *MatchDecision
	EventID        *int64
	EventTitle     string
	// DocumentVersionID pins the exact immutable readable document currently
	// associated with this legacy Content projection. It remains nil when the
	// association or current display authorization cannot be proven.
	DocumentVersionID *int64
}

type ContentListQuery struct {
	Cursor             string
	Limit              int
	Keyword            string
	SourceConnectionID *int64
	PublishedFrom      *time.Time
	PublishedTo        *time.Time
	MonitorID          *int64
	Decision           *MatchDecision
	Sort               ContentSort
}

type ContentSort string

const (
	ContentSortLatest    ContentSort = "latest"
	ContentSortRelevance ContentSort = "relevance"
)

func (sortValue ContentSort) Valid() bool {
	return sortValue == ContentSortLatest || sortValue == ContentSortRelevance
}

func (query ContentListQuery) Normalized() ContentListQuery {
	query.Keyword = strings.TrimSpace(query.Keyword)
	if query.Sort == "" {
		query.Sort = ContentSortLatest
	}
	if query.PublishedFrom != nil {
		value := query.PublishedFrom.UTC()
		query.PublishedFrom = &value
	}
	if query.PublishedTo != nil {
		value := query.PublishedTo.UTC()
		query.PublishedTo = &value
	}
	return query
}

func (query ContentListQuery) Validate() error {
	query = query.Normalized()
	if query.Limit < 1 || query.Limit > 200 || !query.Sort.Valid() || utf8.RuneCountInString(query.Keyword) > 100 {
		return fmt.Errorf("invalid content list query")
	}
	if query.SourceConnectionID != nil && *query.SourceConnectionID <= 0 || query.MonitorID != nil && *query.MonitorID <= 0 {
		return fmt.Errorf("invalid content list reference")
	}
	if query.PublishedFrom != nil && query.PublishedFrom.IsZero() || query.PublishedTo != nil && query.PublishedTo.IsZero() ||
		query.PublishedFrom != nil && query.PublishedTo != nil && query.PublishedFrom.After(*query.PublishedTo) {
		return fmt.Errorf("invalid content list time range")
	}
	if query.Decision != nil && (!query.Decision.Valid() || query.MonitorID == nil) || query.Sort == ContentSortRelevance && query.MonitorID == nil {
		return fmt.Errorf("invalid content relevance query")
	}
	return nil
}

func (query ContentListQuery) ShapeFingerprint() (string, error) {
	query = query.Normalized()
	if err := query.Validate(); err != nil {
		return "", err
	}
	value := func(reference *int64) string {
		if reference == nil {
			return ""
		}
		return strconv.FormatInt(*reference, 10)
	}
	instant := func(reference *time.Time) string {
		if reference == nil {
			return ""
		}
		return reference.UTC().Format(time.RFC3339Nano)
	}
	decision := ""
	if query.Decision != nil {
		decision = string(*query.Decision)
	}
	parts := []string{
		"active-content-v2", strings.ToLower(query.Keyword), value(query.SourceConnectionID),
		instant(query.PublishedFrom), instant(query.PublishedTo), value(query.MonitorID), decision, string(query.Sort),
	}
	digest := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return hex.EncodeToString(digest[:]), nil
}

type ContentPage struct {
	Items      []Content
	NextCursor string
}

type EvidenceObject struct {
	SourceConnectionID int64
	ObjectKey          string
	Text               string
	MIMEType           string
	SHA256             string
}

type EvidenceReceipt struct {
	ObjectKey string
	MIMEType  string
	SHA256    string
	SizeBytes int64
}

type EvidenceText struct {
	Text      string
	MIMEType  string
	SHA256    string
	SizeBytes int64
}

type ContentDocumentAvailability string

const (
	ContentDocumentReady       ContentDocumentAvailability = "ready"
	ContentDocumentNotCaptured ContentDocumentAvailability = "not_captured"
	ContentDocumentUnavailable ContentDocumentAvailability = "unavailable"
)

type ContentDocumentUnavailableReason string

const (
	ContentDocumentReasonPending         ContentDocumentUnavailableReason = "pending"
	ContentDocumentReasonMissing         ContentDocumentUnavailableReason = "missing"
	ContentDocumentReasonDeleting        ContentDocumentUnavailableReason = "deleting"
	ContentDocumentReasonReadFailed      ContentDocumentUnavailableReason = "read_failed"
	ContentDocumentReasonIntegrityFailed ContentDocumentUnavailableReason = "integrity_failed"
)

type ContentDocument struct {
	ContentID         int64
	Title             string
	SourceName        string
	CanonicalURL      string
	Language          string
	PublishedAt       time.Time
	Availability      ContentDocumentAvailability
	UnavailableReason ContentDocumentUnavailableReason
	Markdown          string
	SHA256            string
	CapturedAt        time.Time
}

const NewEventFreshnessWindow = 7 * 24 * time.Hour

// EligibleForNewEvent keeps Content retention independent from hotspot
// freshness. Stale Content remains readable evidence but cannot start the
// downstream cluster, heat, and alert chain.
func EligibleForNewEvent(publishedAt, evaluatedAt time.Time) bool {
	if publishedAt.IsZero() || evaluatedAt.IsZero() {
		return false
	}
	return !publishedAt.UTC().Before(evaluatedAt.UTC().Add(-NewEventFreshnessWindow))
}

func validSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if !(character >= '0' && character <= '9') && !(character >= 'a' && character <= 'f') {
			return false
		}
	}
	return true
}
