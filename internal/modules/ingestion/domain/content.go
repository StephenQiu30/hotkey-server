package domain

import (
	"strings"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/modules/source/domain"
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
	ExternalID         string
	Author             NormalizedAuthor
	ContentType        string
	Title              string
	Excerpt            string
	CanonicalURL       string
	Language           string
	PublishedAt        time.Time
	FetchedAt          time.Time
	ContentHash        string
	Metrics            domain.SourceMetrics
	Status             ContentStatus
	DuplicateOfID      *int64
	DedupeReason       string
	DedupeVersion      string
	DeletedAt          *time.Time
}

type ContentListQuery struct {
	Cursor string
	Limit  int
}

type ContentPage struct {
	Items      []Content
	NextCursor string
}

type EvidenceObject struct {
	ObjectKey string
	Text      string
	SHA256    string
}

type EvidenceReceipt struct {
	ObjectKey string
	SHA256    string
	SizeBytes int64
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
