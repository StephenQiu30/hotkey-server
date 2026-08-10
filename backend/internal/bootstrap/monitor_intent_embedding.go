package bootstrap

import (
	"context"
	"fmt"

	intelligenceapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/intelligence/application"
	monitorapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/monitor/application"
	monitorpostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/monitor/infrastructure/postgres"
)

const (
	compiledIntentEmbeddingPromptVersion = "compiled-intent-embedding-v1"
	compiledIntentEmbeddingInputSchema   = "compiled-intent-input-v1"
	compiledIntentEmbeddingOutputSchema  = "embedding-output-v1"
	compiledIntentEmbeddingParameters    = "compiled-intent-nfc-1024-v1"
)

type compiledIntentEmbeddingProducerAdapter struct {
	executor    projectedEmbeddingExecutor
	projections *monitorapplication.CompiledIntentEmbeddingProjectionService
}

var _ monitorapplication.CompiledIntentEmbeddingProducer = (*compiledIntentEmbeddingProducerAdapter)(nil)

func newCompiledIntentEmbeddingProjectionService(repository *monitorpostgres.IntentRepository) (*monitorapplication.CompiledIntentEmbeddingProjectionService, error) {
	return monitorapplication.NewCompiledIntentEmbeddingProjectionService(repository)
}

func newCompiledIntentEmbeddingProducerAdapter(embeddings *intelligenceapplication.EmbeddingService, projections *monitorapplication.CompiledIntentEmbeddingProjectionService) (*compiledIntentEmbeddingProducerAdapter, error) {
	return newCompiledIntentEmbeddingProducerAdapterWithExecutor(embeddings, projections)
}

func newCompiledIntentEmbeddingProducerAdapterWithExecutor(executor projectedEmbeddingExecutor, projections *monitorapplication.CompiledIntentEmbeddingProjectionService) (*compiledIntentEmbeddingProducerAdapter, error) {
	if executor == nil || projections == nil {
		return nil, fmt.Errorf("compiled intent embedding model and projection services are required")
	}
	return &compiledIntentEmbeddingProducerAdapter{executor: executor, projections: projections}, nil
}

func (adapter *compiledIntentEmbeddingProducerAdapter) ProduceCompiledIntentEmbedding(ctx context.Context, command monitorapplication.ProduceCompiledIntentEmbeddingCommand) (monitorapplication.ProduceCompiledIntentEmbeddingResult, error) {
	if adapter == nil || adapter.executor == nil || adapter.projections == nil || command.CompiledProfileID <= 0 ||
		command.ConfigVersionID <= 0 || command.InputHash == "" || command.Input == "" {
		return monitorapplication.ProduceCompiledIntentEmbeddingResult{}, monitorapplication.ErrInvalidIntentContract
	}
	sink := &compiledIntentEmbeddingProjectionSink{command: command, projections: adapter.projections}
	executed, err := adapter.executor.ExecuteProjectedEmbedding(ctx, intelligenceapplication.ProjectedEmbeddingExecutionInput{
		TargetType: intelligenceapplication.ProjectedEmbeddingTargetMonitorCompiledProfile, TargetID: command.CompiledProfileID,
		PromptVersion: compiledIntentEmbeddingPromptVersion, InputSchemaVersion: compiledIntentEmbeddingInputSchema,
		SchemaVersion: compiledIntentEmbeddingOutputSchema, ParametersVersion: compiledIntentEmbeddingParameters,
		InputHash: command.InputHash, EvidenceSetHash: command.InputHash, Input: command.Input,
	}, sink)
	if err != nil {
		return monitorapplication.ProduceCompiledIntentEmbeddingResult{}, err
	}
	if executed.TargetID != command.CompiledProfileID || executed.InputHash != command.InputHash {
		return monitorapplication.ProduceCompiledIntentEmbeddingResult{}, monitorapplication.ErrCompiledIntentProfileConflict
	}
	if executed.Status == "degraded" {
		if executed.ReasonCode != intelligenceapplication.ProjectedEmbeddingReasonModelUnavailable || sink.receipt != nil {
			return monitorapplication.ProduceCompiledIntentEmbeddingResult{}, monitorapplication.ErrCompiledIntentProfileConflict
		}
		return monitorapplication.ProduceCompiledIntentEmbeddingResult{
			CompiledProfileID: command.CompiledProfileID, Availability: monitorapplication.IntentEmbeddingAvailabilityDegraded,
			UnavailableReason: monitorapplication.IntentSemanticModelUnavailable,
		}, nil
	}
	if executed.Status != "succeeded" || sink.receipt == nil || executed.AIRunID != sink.receipt.AIRunID ||
		executed.ModelProfileID != sink.receipt.ModelProfileID || executed.ModelProfileVersion != sink.receipt.ModelProfileVersion ||
		executed.ModelVersion != sink.receipt.ModelVersion {
		return monitorapplication.ProduceCompiledIntentEmbeddingResult{}, monitorapplication.ErrCompiledIntentProfileConflict
	}
	return monitorapplication.ProduceCompiledIntentEmbeddingResult{
		CompiledProfileID: command.CompiledProfileID, Availability: monitorapplication.IntentEmbeddingAvailabilityReady,
		Receipt: sink.receipt,
	}, nil
}

type compiledIntentEmbeddingProjectionSink struct {
	command     monitorapplication.ProduceCompiledIntentEmbeddingCommand
	projections *monitorapplication.CompiledIntentEmbeddingProjectionService
	receipt     *monitorapplication.CompiledIntentEmbeddingReceiptDTO
}

func (sink *compiledIntentEmbeddingProjectionSink) CommitGeneratedEmbedding(ctx context.Context, generated intelligenceapplication.GeneratedEmbeddingDTO) error {
	if sink == nil || sink.projections == nil || generated.TargetType != intelligenceapplication.ProjectedEmbeddingTargetMonitorCompiledProfile ||
		generated.TargetID != sink.command.CompiledProfileID || generated.InputHash != sink.command.InputHash {
		return monitorapplication.ErrCompiledIntentProfileConflict
	}
	receipt, err := sink.projections.Persist(ctx, monitorapplication.PersistCompiledIntentEmbeddingCommand{
		CompiledProfileID: sink.command.CompiledProfileID, ConfigVersionID: sink.command.ConfigVersionID,
		ModelProfileID: generated.ModelProfileID, ModelProfileVersion: generated.ModelProfileVersion,
		ModelVersion: generated.ModelVersion, InputHash: sink.command.InputHash, Embedding: generated.Vector,
		AIRunID: generated.AIRunID, CreatedAt: generated.CreatedAt,
	})
	if err != nil {
		return err
	}
	sink.receipt = &receipt
	return nil
}

func (sink *compiledIntentEmbeddingProjectionSink) VerifyGeneratedEmbedding(ctx context.Context, generated intelligenceapplication.GeneratedEmbeddingVerificationQuery) error {
	if sink == nil || sink.projections == nil || generated.TargetType != intelligenceapplication.ProjectedEmbeddingTargetMonitorCompiledProfile ||
		generated.TargetID != sink.command.CompiledProfileID || generated.InputHash != sink.command.InputHash {
		return monitorapplication.ErrCompiledIntentProfileConflict
	}
	receipt, err := sink.projections.Read(ctx, monitorapplication.ReadCompiledIntentEmbeddingQuery{
		CompiledProfileID: sink.command.CompiledProfileID, ConfigVersionID: sink.command.ConfigVersionID,
		ModelProfileID: generated.ModelProfileID, ModelProfileVersion: generated.ModelProfileVersion,
		ModelVersion: generated.ModelVersion, InputHash: sink.command.InputHash, AIRunID: generated.AIRunID,
	})
	if err != nil {
		return err
	}
	sink.receipt = &receipt
	return nil
}
