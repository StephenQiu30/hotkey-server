package bootstrap

import (
	"context"
	"fmt"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	intelligenceapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/intelligence/application"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

const (
	documentEmbeddingPromptVersion = "document-embedding-v1"
	documentEmbeddingInputSchema   = "document-embedding-input-v1"
	documentEmbeddingOutputSchema  = "embedding-output-v1"
	documentEmbeddingParameters    = "nfc-plaintext-1024-v1"
)

type documentEmbeddingProducerAdapter struct {
	embeddings  projectedEmbeddingExecutor
	projections *ingestionapplication.DocumentRecallProjectionService
}

type projectedEmbeddingExecutor interface {
	ExecuteProjectedEmbedding(context.Context, intelligenceapplication.ProjectedEmbeddingExecutionInput, intelligenceapplication.ProjectedEmbeddingSink) (intelligenceapplication.ProjectedEmbeddingExecutionResult, error)
}

var _ ingestionapplication.DocumentEmbeddingProducer = (*documentEmbeddingProducerAdapter)(nil)

func newDocumentEmbeddingProducerAdapter(embeddings *intelligenceapplication.EmbeddingService, projections *ingestionapplication.DocumentRecallProjectionService) (*documentEmbeddingProducerAdapter, error) {
	return newDocumentEmbeddingProducerAdapterWithExecutor(embeddings, projections)
}

func newDocumentEmbeddingProducerAdapterWithExecutor(embeddings projectedEmbeddingExecutor, projections *ingestionapplication.DocumentRecallProjectionService) (*documentEmbeddingProducerAdapter, error) {
	if embeddings == nil || projections == nil {
		return nil, fmt.Errorf("document embedding model and projection services are required")
	}
	return &documentEmbeddingProducerAdapter{embeddings: embeddings, projections: projections}, nil
}

func (adapter *documentEmbeddingProducerAdapter) ProduceDocumentEmbedding(ctx context.Context, command ingestionapplication.ProduceDocumentEmbeddingCommand) (ingestionapplication.ProduceDocumentEmbeddingResult, error) {
	if adapter == nil || adapter.embeddings == nil || adapter.projections == nil || command.DocumentVersionID <= 0 ||
		command.SourceConnectionID <= 0 || command.EmbedLocalRightsDecisionID <= 0 || command.RetainRightsDecisionID <= 0 ||
		command.NormalizedTextSHA256 == "" || command.Plaintext == "" {
		return ingestionapplication.ProduceDocumentEmbeddingResult{}, fmt.Errorf("%w: invalid document embedding command", sharedrepository.ErrInvalidInput)
	}
	sink := &documentEmbeddingProjectionSink{command: command, projections: adapter.projections}
	executed, err := adapter.embeddings.ExecuteProjectedEmbedding(ctx, intelligenceapplication.ProjectedEmbeddingExecutionInput{
		TargetType: "document_version", TargetID: command.DocumentVersionID,
		PromptVersion: documentEmbeddingPromptVersion, InputSchemaVersion: documentEmbeddingInputSchema,
		SchemaVersion: documentEmbeddingOutputSchema, ParametersVersion: documentEmbeddingParameters,
		InputHash: command.NormalizedTextSHA256, EvidenceSetHash: command.NormalizedTextSHA256, Input: command.Plaintext,
	}, sink)
	if err != nil {
		return ingestionapplication.ProduceDocumentEmbeddingResult{}, err
	}
	if executed.TargetID != command.DocumentVersionID || executed.InputHash != command.NormalizedTextSHA256 {
		return ingestionapplication.ProduceDocumentEmbeddingResult{}, fmt.Errorf("%w: document embedding execution identity changed", sharedrepository.ErrConflict)
	}
	if executed.Status == "degraded" {
		if executed.ReasonCode != intelligenceapplication.ProjectedEmbeddingReasonModelUnavailable || sink.receipt != nil {
			return ingestionapplication.ProduceDocumentEmbeddingResult{}, fmt.Errorf("%w: invalid degraded document embedding", sharedrepository.ErrConflict)
		}
		return ingestionapplication.ProduceDocumentEmbeddingResult{
			DocumentVersionID: command.DocumentVersionID, Availability: ingestionapplication.SourceDocumentUnavailable,
			UnavailableReason: ingestionapplication.DocumentEmbeddingReasonModelUnavailable,
		}, nil
	}
	if executed.Status != "succeeded" || sink.receipt == nil || executed.AIRunID != sink.receipt.AIRunID ||
		executed.ModelProfileID != sink.receipt.ModelProfileID || executed.ModelProfileVersion != sink.receipt.ModelProfileVersion ||
		executed.ModelVersion != sink.receipt.ModelVersion {
		return ingestionapplication.ProduceDocumentEmbeddingResult{}, fmt.Errorf("%w: document embedding projection receipt changed", sharedrepository.ErrConflict)
	}
	return ingestionapplication.ProduceDocumentEmbeddingResult{
		DocumentVersionID: command.DocumentVersionID, Availability: ingestionapplication.SourceDocumentAvailable,
		Receipt: sink.receipt,
	}, nil
}

type documentEmbeddingProjectionSink struct {
	command     ingestionapplication.ProduceDocumentEmbeddingCommand
	projections *ingestionapplication.DocumentRecallProjectionService
	receipt     *ingestionapplication.DocumentEmbeddingReceiptResult
}

func (sink *documentEmbeddingProjectionSink) CommitGeneratedEmbedding(ctx context.Context, generated intelligenceapplication.GeneratedEmbeddingDTO) error {
	if sink == nil || sink.projections == nil || generated.TargetType != "document_version" || generated.TargetID != sink.command.DocumentVersionID ||
		generated.InputHash != sink.command.NormalizedTextSHA256 || generated.ModelProfileID <= 0 || generated.ModelProfileVersion <= 0 ||
		generated.ModelVersion == "" || generated.AIRunID <= 0 || generated.CreatedAt.IsZero() {
		return fmt.Errorf("%w: generated document embedding identity changed", sharedrepository.ErrConflict)
	}
	receipt, err := sink.projections.PersistEmbeddingReceipt(ctx, ingestionapplication.PersistDocumentEmbeddingReceiptCommand{
		DocumentVersionID: sink.command.DocumentVersionID, EmbedLocalRightsDecisionID: sink.command.EmbedLocalRightsDecisionID,
		RetainRightsDecisionID: sink.command.RetainRightsDecisionID, ModelProfileID: generated.ModelProfileID,
		ModelProfileVersion: generated.ModelProfileVersion, ModelVersion: generated.ModelVersion,
		NormalizedTextSHA256: sink.command.NormalizedTextSHA256, Embedding: generated.Vector,
		AIRunID: generated.AIRunID, CreatedAt: generated.CreatedAt,
	})
	if err != nil {
		return err
	}
	sink.receipt = &receipt
	return nil
}

func (sink *documentEmbeddingProjectionSink) VerifyGeneratedEmbedding(ctx context.Context, generated intelligenceapplication.GeneratedEmbeddingVerificationQuery) error {
	if sink == nil || sink.projections == nil || generated.TargetType != "document_version" || generated.TargetID != sink.command.DocumentVersionID ||
		generated.InputHash != sink.command.NormalizedTextSHA256 {
		return fmt.Errorf("%w: reused document embedding identity changed", sharedrepository.ErrConflict)
	}
	receipt, err := sink.projections.ReadEmbeddingReceipt(ctx, ingestionapplication.ReadDocumentEmbeddingReceiptQuery{
		DocumentVersionID: sink.command.DocumentVersionID, EmbedLocalRightsDecisionID: sink.command.EmbedLocalRightsDecisionID,
		RetainRightsDecisionID: sink.command.RetainRightsDecisionID, ModelProfileID: generated.ModelProfileID,
		ModelProfileVersion: generated.ModelProfileVersion, ModelVersion: generated.ModelVersion,
		NormalizedTextSHA256: sink.command.NormalizedTextSHA256, AIRunID: generated.AIRunID,
	})
	if err != nil {
		return err
	}
	sink.receipt = &receipt
	return nil
}
