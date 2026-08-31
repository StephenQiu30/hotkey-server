package application

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
	"golang.org/x/text/unicode/norm"
)

const RecallAssetLifecycleActive = "active"

// PersistDocumentSearchProjectionCommand is a synchronous use-case POJO. The
// canonical plaintext is memory-only input: adapters may derive tsvector and
// trigram signals from it but must never put it in a persistence Row, job or
// log field.
type PersistDocumentSearchProjectionCommand struct {
	DocumentVersionID            int64
	DerivedArtifactID            int64
	StoreDerivedRightsDecisionID int64
	RetainRightsDecisionID       int64
	NormalizationProfileVersion  string
	NormalizedTextSHA256         string
	Plaintext                    string
	EntityKeys                   []string
	ActionKeys                   []string
	LocationKeys                 []string
	RegionKeys                   []string
	IndexedAt                    time.Time
}

type DocumentSearchProjectionResult struct {
	ProjectionID                 int64
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

// PersistDocumentEmbeddingReceiptCommand accepts only the output of one exact
// succeeded embedding run. []float32 deliberately keeps pgvector out of the
// public Application contract.
type PersistDocumentEmbeddingReceiptCommand struct {
	DocumentVersionID          int64
	EmbedLocalRightsDecisionID int64
	RetainRightsDecisionID     int64
	ModelProfileID             int64
	ModelProfileVersion        int64
	ModelVersion               string
	NormalizedTextSHA256       string
	Embedding                  []float32
	AIRunID                    int64
	CreatedAt                  time.Time
}

type DocumentEmbeddingReceiptResult struct {
	EmbeddingID                int64
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

type ReadDocumentEmbeddingReceiptQuery struct {
	DocumentVersionID          int64
	EmbedLocalRightsDecisionID int64
	RetainRightsDecisionID     int64
	ModelProfileID             int64
	ModelProfileVersion        int64
	ModelVersion               string
	NormalizedTextSHA256       string
	AIRunID                    int64
}

type DocumentRecallProjectionWriter interface {
	PersistDocumentSearchProjection(context.Context, PersistDocumentSearchProjectionCommand) (DocumentSearchProjectionResult, error)
	PersistDocumentEmbeddingReceipt(context.Context, PersistDocumentEmbeddingReceiptCommand) (DocumentEmbeddingReceiptResult, error)
	ReadDocumentEmbeddingReceipt(context.Context, ReadDocumentEmbeddingReceiptQuery) (DocumentEmbeddingReceiptResult, error)
}

type DocumentRecallProjectionService struct {
	writer DocumentRecallProjectionWriter
}

func NewDocumentRecallProjectionService(writer DocumentRecallProjectionWriter) (*DocumentRecallProjectionService, error) {
	if writer == nil {
		return nil, fmt.Errorf("%w: document recall projection writer is required", sharedrepository.ErrInvalidInput)
	}
	return &DocumentRecallProjectionService{writer: writer}, nil
}

func (service *DocumentRecallProjectionService) PersistSearchProjection(ctx context.Context, command PersistDocumentSearchProjectionCommand) (DocumentSearchProjectionResult, error) {
	if service == nil || service.writer == nil {
		return DocumentSearchProjectionResult{}, sharedrepository.ErrUnavailable
	}
	prepared, err := prepareDocumentSearchProjection(command)
	if err != nil {
		return DocumentSearchProjectionResult{}, fmt.Errorf("%w: %w", sharedrepository.ErrInvalidInput, err)
	}
	result, err := service.writer.PersistDocumentSearchProjection(ctx, prepared)
	if err != nil {
		return DocumentSearchProjectionResult{}, err
	}
	indexedAtValid := !result.IndexedAt.IsZero() && ((result.Created && result.IndexedAt.Equal(prepared.IndexedAt)) ||
		(!result.Created && !result.IndexedAt.After(prepared.IndexedAt)))
	if result.ProjectionID <= 0 || result.DocumentVersionID != prepared.DocumentVersionID || result.SourceConnectionID <= 0 ||
		result.DerivedArtifactID != prepared.DerivedArtifactID ||
		result.StoreDerivedRightsDecisionID != prepared.StoreDerivedRightsDecisionID ||
		result.RetainRightsDecisionID != prepared.RetainRightsDecisionID ||
		result.NormalizationProfileVersion != prepared.NormalizationProfileVersion ||
		result.NormalizedTextSHA256 != prepared.NormalizedTextSHA256 || !indexedAtValid ||
		!result.RetentionUntil.After(prepared.IndexedAt) ||
		result.LifecycleState != RecallAssetLifecycleActive {
		return DocumentSearchProjectionResult{}, fmt.Errorf("%w: search projection receipt identity changed", sharedrepository.ErrConflict)
	}
	return result, nil
}

func (service *DocumentRecallProjectionService) PersistEmbeddingReceipt(ctx context.Context, command PersistDocumentEmbeddingReceiptCommand) (DocumentEmbeddingReceiptResult, error) {
	if service == nil || service.writer == nil {
		return DocumentEmbeddingReceiptResult{}, sharedrepository.ErrUnavailable
	}
	prepared, err := prepareDocumentEmbeddingReceipt(command)
	if err != nil {
		return DocumentEmbeddingReceiptResult{}, fmt.Errorf("%w: %w", sharedrepository.ErrInvalidInput, err)
	}
	result, err := service.writer.PersistDocumentEmbeddingReceipt(ctx, prepared)
	if err != nil {
		return DocumentEmbeddingReceiptResult{}, err
	}
	createdAtValid := !result.CreatedAt.IsZero() && ((result.Created && result.CreatedAt.Equal(prepared.CreatedAt)) ||
		(!result.Created && !result.CreatedAt.After(prepared.CreatedAt)))
	if result.EmbeddingID <= 0 || result.DocumentVersionID != prepared.DocumentVersionID || result.SourceConnectionID <= 0 ||
		result.EmbedLocalRightsDecisionID != prepared.EmbedLocalRightsDecisionID ||
		result.RetainRightsDecisionID != prepared.RetainRightsDecisionID ||
		result.ModelProfileID != prepared.ModelProfileID || result.ModelProfileVersion != prepared.ModelProfileVersion ||
		result.ModelVersion != prepared.ModelVersion || result.NormalizedTextSHA256 != prepared.NormalizedTextSHA256 ||
		result.AIRunID != prepared.AIRunID || !createdAtValid ||
		!result.RetentionUntil.After(prepared.CreatedAt) ||
		result.LifecycleState != RecallAssetLifecycleActive {
		return DocumentEmbeddingReceiptResult{}, fmt.Errorf("%w: embedding receipt identity changed", sharedrepository.ErrConflict)
	}
	return result, nil
}

func (service *DocumentRecallProjectionService) ReadEmbeddingReceipt(ctx context.Context, query ReadDocumentEmbeddingReceiptQuery) (DocumentEmbeddingReceiptResult, error) {
	if service == nil || service.writer == nil || query.DocumentVersionID <= 0 || query.EmbedLocalRightsDecisionID <= 0 ||
		query.RetainRightsDecisionID <= 0 || query.ModelProfileID <= 0 || query.ModelProfileVersion <= 0 ||
		!semanticVersionPattern.MatchString(query.ModelVersion) || !validLowerHexSHA256(query.NormalizedTextSHA256) || query.AIRunID <= 0 {
		return DocumentEmbeddingReceiptResult{}, fmt.Errorf("%w: invalid document embedding receipt query", sharedrepository.ErrInvalidInput)
	}
	result, err := service.writer.ReadDocumentEmbeddingReceipt(ctx, query)
	if err != nil {
		return DocumentEmbeddingReceiptResult{}, err
	}
	if result.EmbeddingID <= 0 || result.DocumentVersionID != query.DocumentVersionID || result.SourceConnectionID <= 0 ||
		result.EmbedLocalRightsDecisionID != query.EmbedLocalRightsDecisionID || result.RetainRightsDecisionID != query.RetainRightsDecisionID ||
		result.ModelProfileID != query.ModelProfileID || result.ModelProfileVersion != query.ModelProfileVersion ||
		result.ModelVersion != query.ModelVersion || result.NormalizedTextSHA256 != query.NormalizedTextSHA256 || result.AIRunID != query.AIRunID ||
		result.CreatedAt.IsZero() || !result.RetentionUntil.After(result.CreatedAt) || result.LifecycleState != RecallAssetLifecycleActive {
		return DocumentEmbeddingReceiptResult{}, fmt.Errorf("%w: stored document embedding receipt changed", sharedrepository.ErrConflict)
	}
	return result, nil
}

func prepareDocumentSearchProjection(command PersistDocumentSearchProjectionCommand) (PersistDocumentSearchProjectionCommand, error) {
	if command.DocumentVersionID <= 0 || command.DerivedArtifactID <= 0 || command.StoreDerivedRightsDecisionID <= 0 ||
		command.RetainRightsDecisionID <= 0 || !semanticVersionPattern.MatchString(command.NormalizationProfileVersion) ||
		!validLowerHexSHA256(command.NormalizedTextSHA256) || command.IndexedAt.IsZero() || command.Plaintext == "" ||
		len(command.Plaintext) > MaximumCanonicalSourceBodyBytes || !utf8.ValidString(command.Plaintext) ||
		strings.ContainsRune(command.Plaintext, '\r') || norm.NFC.String(command.Plaintext) != command.Plaintext {
		return PersistDocumentSearchProjectionCommand{}, fmt.Errorf("search projection identity or plaintext is invalid")
	}
	if fmt.Sprintf("%x", sha256.Sum256([]byte(command.Plaintext))) != command.NormalizedTextSHA256 {
		return PersistDocumentSearchProjectionCommand{}, fmt.Errorf("search projection plaintext digest does not match")
	}
	var err error
	if command.EntityKeys, err = normalizedRecallProjectionKeys(command.EntityKeys); err != nil {
		return PersistDocumentSearchProjectionCommand{}, err
	}
	if command.ActionKeys, err = normalizedRecallProjectionKeys(command.ActionKeys); err != nil {
		return PersistDocumentSearchProjectionCommand{}, err
	}
	if command.LocationKeys, err = normalizedRecallProjectionKeys(command.LocationKeys); err != nil {
		return PersistDocumentSearchProjectionCommand{}, err
	}
	if command.RegionKeys, err = normalizedRecallProjectionKeys(command.RegionKeys); err != nil {
		return PersistDocumentSearchProjectionCommand{}, err
	}
	command.IndexedAt = command.IndexedAt.UTC().Truncate(time.Microsecond)
	return command, nil
}

func prepareDocumentEmbeddingReceipt(command PersistDocumentEmbeddingReceiptCommand) (PersistDocumentEmbeddingReceiptCommand, error) {
	if command.DocumentVersionID <= 0 || command.EmbedLocalRightsDecisionID <= 0 || command.RetainRightsDecisionID <= 0 ||
		command.ModelProfileID <= 0 || command.ModelProfileVersion <= 0 || !semanticVersionPattern.MatchString(command.ModelVersion) ||
		!validLowerHexSHA256(command.NormalizedTextSHA256) || !validRecallVector(command.Embedding) || command.AIRunID <= 0 || command.CreatedAt.IsZero() {
		return PersistDocumentEmbeddingReceiptCommand{}, fmt.Errorf("embedding receipt is invalid")
	}
	command.Embedding = append([]float32(nil), command.Embedding...)
	command.CreatedAt = command.CreatedAt.UTC().Truncate(time.Microsecond)
	return command, nil
}

func normalizedRecallProjectionKeys(values []string) ([]string, error) {
	if len(values) > 512 {
		return nil, fmt.Errorf("structured recall projection exceeds its bound")
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = norm.NFC.String(strings.ToLower(strings.TrimSpace(value)))
		if value == "" || !utf8.ValidString(value) || len(value) > 640 {
			return nil, fmt.Errorf("structured recall projection key is invalid")
		}
		if _, duplicate := seen[value]; !duplicate {
			seen[value] = struct{}{}
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result, nil
}
