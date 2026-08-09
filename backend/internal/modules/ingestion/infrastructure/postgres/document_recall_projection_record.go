package postgres

import (
	"time"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
)

// documentSearchProjectionRecord contains no canonical plaintext or derived
// lexemes. Sensitive input is consumed inside one SQL statement and only this
// non-sensitive receipt crosses the mapper boundary.
type documentSearchProjectionRecord struct {
	ID                           int64
	DocumentVersionID            int64
	SourceConnectionID           int64
	DerivedArtifactID            int64
	StoreDerivedRightsDecisionID int64
	RetainRightsDecisionID       int64
	NormalizationProfileVersion  string
	NormalizedTextSHA256         string
	RetentionUntil               time.Time
	IndexedAt                    time.Time
	LifecycleState               string
	Created                      bool
}

func documentSearchProjectionResult(record documentSearchProjectionRecord) ingestionapplication.DocumentSearchProjectionResult {
	return ingestionapplication.DocumentSearchProjectionResult{
		ProjectionID: record.ID, DocumentVersionID: record.DocumentVersionID,
		SourceConnectionID: record.SourceConnectionID, DerivedArtifactID: record.DerivedArtifactID,
		StoreDerivedRightsDecisionID: record.StoreDerivedRightsDecisionID,
		RetainRightsDecisionID:       record.RetainRightsDecisionID,
		NormalizationProfileVersion:  record.NormalizationProfileVersion,
		NormalizedTextSHA256:         record.NormalizedTextSHA256,
		RetentionUntil:               record.RetentionUntil, IndexedAt: record.IndexedAt,
		LifecycleState: record.LifecycleState, Created: record.Created,
	}
}

type documentEmbeddingReceiptRecord struct {
	ID                         int64
	DocumentVersionID          int64
	SourceConnectionID         int64
	EmbedLocalRightsDecisionID int64
	RetainRightsDecisionID     int64
	ModelProfileID             int64
	ModelProfileVersion        int64
	ModelVersion               string
	NormalizedTextSHA256       string
	AIRunID                    int64
	RetentionUntil             time.Time
	CreatedAt                  time.Time
	LifecycleState             string
	Created                    bool
}

func documentEmbeddingReceiptResult(record documentEmbeddingReceiptRecord) ingestionapplication.DocumentEmbeddingReceiptResult {
	return ingestionapplication.DocumentEmbeddingReceiptResult{
		EmbeddingID: record.ID, DocumentVersionID: record.DocumentVersionID,
		SourceConnectionID:         record.SourceConnectionID,
		EmbedLocalRightsDecisionID: record.EmbedLocalRightsDecisionID,
		RetainRightsDecisionID:     record.RetainRightsDecisionID,
		ModelProfileID:             record.ModelProfileID, ModelProfileVersion: record.ModelProfileVersion,
		ModelVersion: record.ModelVersion, NormalizedTextSHA256: record.NormalizedTextSHA256,
		AIRunID: record.AIRunID, RetentionUntil: record.RetentionUntil, CreatedAt: record.CreatedAt,
		LifecycleState: record.LifecycleState, Created: record.Created,
	}
}
