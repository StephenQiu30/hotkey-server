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

// EvidenceStore is ingestion's only object-storage boundary. Implementations
// must not leak provider SDK types into application or domain code.
type EvidenceStore interface {
	PutText(ctx context.Context, object EvidenceObject) (EvidenceReceipt, error)
	Delete(ctx context.Context, objectKey string) error
	ListPrefix(ctx context.Context, prefix string) ([]EvidenceReceipt, error)
}
