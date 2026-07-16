package domain

import (
	"context"
	"time"

	sourcedomain "github.com/StephenQiu30/hotkey-server/internal/modules/source/domain"
)

// ContentRepository is ingestion's persistence boundary. Its methods are
// intentionally concrete to Content/asset lifecycle facts; it does not expose
// arbitrary filters or tables owned by another module.
type ContentRepository interface {
	Upsert(ctx context.Context, content NormalizedContent, decision DedupeDecision) (Content, bool, error)
	AppendMetricSnapshot(ctx context.Context, contentID int64, capturedAt time.Time, metrics sourcedomain.SourceMetrics) error
	CreateAsset(ctx context.Context, asset ContentAsset) error
	MarkAssetStatus(ctx context.Context, objectKey string, status AssetStatus) error
	ListEvidenceAssets(ctx context.Context, sourceConnectionID, contentID int64) ([]ContentAsset, error)
	ListAssetObjectKeys(ctx context.Context, sourceConnectionID int64) ([]string, error)
	ListActive(ctx context.Context, query ContentListQuery) (ContentPage, error)
	GetActive(ctx context.Context, contentID int64) (Content, error)
	MarkDeleted(ctx context.Context, sourceConnectionID int64, externalID string) (Content, bool, error)
	ExpireBefore(ctx context.Context, before time.Time) (int, error)
}

// RelevanceRepository keeps auditable match snapshots and feedback facts in
// ingestion. It owns the cross-table validation needed for Content and Monitor
// references, without exposing the AI module's tables as a general query API.
type RelevanceRepository interface {
	UpsertSnapshot(context.Context, RelevanceSnapshotInput) (RelevanceSnapshot, bool, error)
	ApplySuccessfulReview(context.Context, SuccessfulReviewInput) (RelevanceSnapshot, error)
	MarkReviewUnavailable(context.Context, int64, int64, string) (RelevanceSnapshot, error)
	ListLatestSnapshots(context.Context, int64, RelevanceSnapshotListQuery) (RelevanceSnapshotPage, error)
	UpsertFeedback(context.Context, RelevanceFeedbackInput) (RelevanceFeedback, error)
	UpsertPendingSuggestion(context.Context, RelevanceSuggestionInput) (RelevanceSuggestion, bool, error)
	ReviewSuggestion(context.Context, int64, int64, int64, SuggestionStatus) (RelevanceSuggestion, error)
}

// RelevanceCandidateReader is the bounded read side of relevance matching.
// Implementations must return only active Monitors whose current config is
// published and must never turn these calls into an unrestricted Monitor list.
type RelevanceCandidateReader interface {
	SourceCandidates(context.Context, int64, int) ([]RelevanceCandidateHit, error)
	LexicalCandidates(context.Context, []string, int) ([]RelevanceCandidateHit, error)
	LoadRelevanceCandidates(context.Context, []int64) ([]RelevanceCandidate, error)
}

// EvidenceStore is ingestion's only object-storage boundary. Implementations
// must not leak provider SDK types into application or domain code.
type EvidenceStore interface {
	PutText(ctx context.Context, object EvidenceObject) (EvidenceReceipt, error)
	Delete(ctx context.Context, objectKey string) error
	ListPrefix(ctx context.Context, prefix string) ([]EvidenceReceipt, error)
}
