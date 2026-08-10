package application

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const (
	IntentSemanticStateReady            = "ready"
	IntentSemanticStateUnavailable      = "unavailable"
	IntentSemanticModelUnavailable      = "semantic_model_unavailable"
	IntentEmbeddingAvailabilityReady    = "ready"
	IntentEmbeddingAvailabilityDegraded = "degraded"
)

type ProduceCompiledIntentEmbeddingCommand struct {
	CompiledProfileID int64
	ConfigVersionID   int64
	InputHash         string
	Input             string
}

type CompiledIntentEmbeddingReceiptDTO struct {
	EmbeddingID         int64
	CompiledProfileID   int64
	ConfigVersionID     int64
	ModelProfileID      int64
	ModelProfileVersion int64
	ModelVersion        string
	InputHash           string
	AIRunID             int64
	CreatedAt           time.Time
	Created             bool
}

type ProduceCompiledIntentEmbeddingResult struct {
	CompiledProfileID int64
	Availability      string
	UnavailableReason string
	Receipt           *CompiledIntentEmbeddingReceiptDTO
}

type PersistCompiledIntentEmbeddingCommand struct {
	CompiledProfileID   int64
	ConfigVersionID     int64
	ModelProfileID      int64
	ModelProfileVersion int64
	ModelVersion        string
	InputHash           string
	Embedding           []float32
	AIRunID             int64
	CreatedAt           time.Time
}

type ReadCompiledIntentEmbeddingQuery struct {
	CompiledProfileID   int64
	ConfigVersionID     int64
	ModelProfileID      int64
	ModelProfileVersion int64
	ModelVersion        string
	InputHash           string
	AIRunID             int64
}

type CompletePreviewCompiledProfileDTO struct {
	CompiledProfileID         int64
	ConfigVersionID           int64
	IntentRevisionID          int64
	ProfileHash               string
	SemanticState             string
	SemanticUnavailableReason string
	ReadyAt                   time.Time
}

type CompletePreviewCompiledProfileReceiptDTO struct {
	CompiledProfileID         int64
	Status                    string
	SemanticState             string
	SemanticUnavailableReason string
	Reused                    bool
}

type CompiledIntentEmbeddingProducer interface {
	ProduceCompiledIntentEmbedding(context.Context, ProduceCompiledIntentEmbeddingCommand) (ProduceCompiledIntentEmbeddingResult, error)
}

type CompiledIntentEmbeddingRepository interface {
	PersistCompiledIntentEmbedding(context.Context, PersistCompiledIntentEmbeddingCommand) (CompiledIntentEmbeddingReceiptDTO, error)
	ReadCompiledIntentEmbedding(context.Context, ReadCompiledIntentEmbeddingQuery) (CompiledIntentEmbeddingReceiptDTO, error)
}

type CompiledIntentEmbeddingProjectionService struct {
	repository CompiledIntentEmbeddingRepository
}

func NewCompiledIntentEmbeddingProjectionService(repository CompiledIntentEmbeddingRepository) (*CompiledIntentEmbeddingProjectionService, error) {
	if repository == nil {
		return nil, fmt.Errorf("%w: compiled intent embedding repository is required", ErrInvalidIntentContract)
	}
	return &CompiledIntentEmbeddingProjectionService{repository: repository}, nil
}

func (service *CompiledIntentEmbeddingProjectionService) Persist(ctx context.Context, command PersistCompiledIntentEmbeddingCommand) (CompiledIntentEmbeddingReceiptDTO, error) {
	if service == nil || service.repository == nil || !validPersistedCompiledIntentEmbedding(command) {
		return CompiledIntentEmbeddingReceiptDTO{}, ErrInvalidIntentContract
	}
	prepared := command
	prepared.Embedding = append([]float32(nil), command.Embedding...)
	prepared.CreatedAt = command.CreatedAt.UTC().Truncate(time.Microsecond)
	receipt, err := service.repository.PersistCompiledIntentEmbedding(ctx, prepared)
	if err != nil {
		return CompiledIntentEmbeddingReceiptDTO{}, err
	}
	if !sameCompiledIntentEmbeddingReceipt(receipt, prepared) {
		return CompiledIntentEmbeddingReceiptDTO{}, ErrCompiledIntentProfileConflict
	}
	return receipt, nil
}

func (service *CompiledIntentEmbeddingProjectionService) Read(ctx context.Context, query ReadCompiledIntentEmbeddingQuery) (CompiledIntentEmbeddingReceiptDTO, error) {
	if service == nil || service.repository == nil || query.CompiledProfileID <= 0 || query.ConfigVersionID <= 0 ||
		query.ModelProfileID <= 0 || query.ModelProfileVersion <= 0 || !intentProfilePattern.MatchString(query.ModelVersion) ||
		!validIntentApplicationSHA256(query.InputHash) || query.AIRunID <= 0 {
		return CompiledIntentEmbeddingReceiptDTO{}, ErrInvalidIntentContract
	}
	receipt, err := service.repository.ReadCompiledIntentEmbedding(ctx, query)
	if err != nil {
		return CompiledIntentEmbeddingReceiptDTO{}, err
	}
	if receipt.EmbeddingID <= 0 || receipt.CompiledProfileID != query.CompiledProfileID || receipt.ConfigVersionID != query.ConfigVersionID ||
		receipt.ModelProfileID != query.ModelProfileID || receipt.ModelProfileVersion != query.ModelProfileVersion ||
		receipt.ModelVersion != query.ModelVersion || receipt.InputHash != query.InputHash || receipt.AIRunID != query.AIRunID || receipt.CreatedAt.IsZero() {
		return CompiledIntentEmbeddingReceiptDTO{}, ErrCompiledIntentProfileConflict
	}
	return receipt, nil
}

func validPersistedCompiledIntentEmbedding(command PersistCompiledIntentEmbeddingCommand) bool {
	return command.CompiledProfileID > 0 && command.ConfigVersionID > 0 && command.ModelProfileID > 0 &&
		command.ModelProfileVersion > 0 && intentProfilePattern.MatchString(command.ModelVersion) &&
		validIntentApplicationSHA256(command.InputHash) && len(command.Embedding) == 1024 && command.AIRunID > 0 && !command.CreatedAt.IsZero()
}

func sameCompiledIntentEmbeddingReceipt(receipt CompiledIntentEmbeddingReceiptDTO, command PersistCompiledIntentEmbeddingCommand) bool {
	return receipt.EmbeddingID > 0 && receipt.CompiledProfileID == command.CompiledProfileID &&
		receipt.ConfigVersionID == command.ConfigVersionID && receipt.ModelProfileID == command.ModelProfileID &&
		receipt.ModelProfileVersion == command.ModelProfileVersion && receipt.ModelVersion == command.ModelVersion &&
		receipt.InputHash == command.InputHash && receipt.AIRunID == command.AIRunID &&
		!receipt.CreatedAt.IsZero() && receipt.CreatedAt.Equal(command.CreatedAt)
}

func validateProducedCompiledIntentEmbedding(command ProduceCompiledIntentEmbeddingCommand, result ProduceCompiledIntentEmbeddingResult) error {
	if command.CompiledProfileID <= 0 || command.ConfigVersionID <= 0 || !validIntentApplicationSHA256(command.InputHash) ||
		strings.TrimSpace(command.Input) == "" || result.CompiledProfileID != command.CompiledProfileID {
		return ErrInvalidIntentContract
	}
	switch result.Availability {
	case IntentEmbeddingAvailabilityReady:
		if result.UnavailableReason != "" || result.Receipt == nil || result.Receipt.CompiledProfileID != command.CompiledProfileID ||
			result.Receipt.ConfigVersionID != command.ConfigVersionID || result.Receipt.InputHash != command.InputHash ||
			result.Receipt.EmbeddingID <= 0 || result.Receipt.ModelProfileID <= 0 || result.Receipt.ModelProfileVersion <= 0 ||
			result.Receipt.ModelVersion == "" || result.Receipt.AIRunID <= 0 || result.Receipt.CreatedAt.IsZero() {
			return ErrCompiledIntentProfileConflict
		}
	case IntentEmbeddingAvailabilityDegraded:
		if result.Receipt != nil || result.UnavailableReason != IntentSemanticModelUnavailable {
			return ErrCompiledIntentProfileConflict
		}
	default:
		return ErrCompiledIntentProfileConflict
	}
	return nil
}
