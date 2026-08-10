package bootstrap

import (
	"context"
	"strings"
	"testing"
	"time"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	intelligenceapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/intelligence/application"
)

type projectedEmbeddingExecutorFake struct {
	input  intelligenceapplication.ProjectedEmbeddingExecutionInput
	result intelligenceapplication.ProjectedEmbeddingExecutionResult
	mode   string
}

func (fake *projectedEmbeddingExecutorFake) ExecuteProjectedEmbedding(ctx context.Context, input intelligenceapplication.ProjectedEmbeddingExecutionInput, sink intelligenceapplication.ProjectedEmbeddingSink) (intelligenceapplication.ProjectedEmbeddingExecutionResult, error) {
	fake.input = input
	switch fake.mode {
	case "commit":
		vector := make([]float32, 1024)
		vector[0] = 1
		if err := sink.CommitGeneratedEmbedding(ctx, intelligenceapplication.GeneratedEmbeddingDTO{
			TargetType: "document_version", TargetID: input.TargetID,
			ModelProfileID: 81, ModelProfileVersion: 2, ModelVersion: "embedding-v1",
			AIRunID: 91, InputHash: input.InputHash, Vector: vector,
			CreatedAt: time.Date(2026, time.August, 10, 8, 0, 0, 0, time.UTC),
		}); err != nil {
			return intelligenceapplication.ProjectedEmbeddingExecutionResult{}, err
		}
	case "verify":
		if err := sink.VerifyGeneratedEmbedding(ctx, intelligenceapplication.GeneratedEmbeddingVerificationQuery{
			TargetType: "document_version", TargetID: input.TargetID,
			ModelProfileID: 81, ModelProfileVersion: 2, ModelVersion: "embedding-v1",
			AIRunID: 91, InputHash: input.InputHash,
		}); err != nil {
			return intelligenceapplication.ProjectedEmbeddingExecutionResult{}, err
		}
	}
	return fake.result, nil
}

type documentEmbeddingProjectionWriterFake struct {
	persisted ingestionapplication.PersistDocumentEmbeddingReceiptCommand
	result    ingestionapplication.DocumentEmbeddingReceiptResult
	reads     int
}

func (*documentEmbeddingProjectionWriterFake) PersistDocumentSearchProjection(context.Context, ingestionapplication.PersistDocumentSearchProjectionCommand) (ingestionapplication.DocumentSearchProjectionResult, error) {
	panic("search projection is not used by document embedding adapter")
}

func (fake *documentEmbeddingProjectionWriterFake) PersistDocumentEmbeddingReceipt(_ context.Context, command ingestionapplication.PersistDocumentEmbeddingReceiptCommand) (ingestionapplication.DocumentEmbeddingReceiptResult, error) {
	fake.persisted = command
	return fake.result, nil
}

func (fake *documentEmbeddingProjectionWriterFake) ReadDocumentEmbeddingReceipt(_ context.Context, _ ingestionapplication.ReadDocumentEmbeddingReceiptQuery) (ingestionapplication.DocumentEmbeddingReceiptResult, error) {
	fake.reads++
	return fake.result, nil
}

func TestDocumentEmbeddingProducerAdapterMapsGeneratedVectorToExactIngestionReceipt(t *testing.T) {
	command := documentEmbeddingCommandFixture()
	receipt := documentEmbeddingReceiptFixture(command)
	writer := &documentEmbeddingProjectionWriterFake{result: receipt}
	projections, err := ingestionapplication.NewDocumentRecallProjectionService(writer)
	if err != nil {
		t.Fatal(err)
	}
	executor := &projectedEmbeddingExecutorFake{mode: "commit", result: intelligenceapplication.ProjectedEmbeddingExecutionResult{
		Status: "succeeded", TargetID: command.DocumentVersionID, InputHash: command.NormalizedTextSHA256,
		ModelProfileID: 81, ModelProfileVersion: 2, ModelVersion: "embedding-v1", AIRunID: 91,
	}}
	adapter, err := newDocumentEmbeddingProducerAdapterWithExecutor(executor, projections)
	if err != nil {
		t.Fatal(err)
	}

	result, err := adapter.ProduceDocumentEmbedding(context.Background(), command)
	if err != nil {
		t.Fatalf("ProduceDocumentEmbedding(): %v", err)
	}
	if result.Availability != ingestionapplication.SourceDocumentAvailable || result.Receipt == nil || result.Receipt.EmbeddingID != receipt.EmbeddingID {
		t.Fatalf("result=%#v, want exact available receipt", result)
	}
	if executor.input.TargetType != "document_version" || executor.input.TargetID != command.DocumentVersionID ||
		executor.input.InputHash != command.NormalizedTextSHA256 || executor.input.EvidenceSetHash != command.NormalizedTextSHA256 ||
		executor.input.Input != command.Plaintext || writer.persisted.EmbedLocalRightsDecisionID != command.EmbedLocalRightsDecisionID ||
		writer.persisted.RetainRightsDecisionID != command.RetainRightsDecisionID || len(writer.persisted.Embedding) != 1024 {
		t.Fatalf("execution input=%#v persisted=%#v", executor.input, writer.persisted)
	}
}

func TestDocumentEmbeddingProducerAdapterVerifiesReuseAndRepresentsModelDegradation(t *testing.T) {
	command := documentEmbeddingCommandFixture()
	for _, test := range []struct {
		name         string
		executor     *projectedEmbeddingExecutorFake
		availability ingestionapplication.SourceDocumentAvailability
		reason       string
		reads        int
	}{
		{
			name: "reuse", executor: &projectedEmbeddingExecutorFake{mode: "verify", result: intelligenceapplication.ProjectedEmbeddingExecutionResult{
				Status: "succeeded", Reused: true, TargetID: command.DocumentVersionID, InputHash: command.NormalizedTextSHA256,
				ModelProfileID: 81, ModelProfileVersion: 2, ModelVersion: "embedding-v1", AIRunID: 91,
			}}, availability: ingestionapplication.SourceDocumentAvailable, reads: 1,
		},
		{
			name: "degraded", executor: &projectedEmbeddingExecutorFake{result: intelligenceapplication.ProjectedEmbeddingExecutionResult{
				Status: "degraded", ReasonCode: intelligenceapplication.ProjectedEmbeddingReasonModelUnavailable,
				TargetID: command.DocumentVersionID, InputHash: command.NormalizedTextSHA256,
			}}, availability: ingestionapplication.SourceDocumentUnavailable,
			reason: ingestionapplication.DocumentEmbeddingReasonModelUnavailable,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			writer := &documentEmbeddingProjectionWriterFake{result: documentEmbeddingReceiptFixture(command)}
			projections, _ := ingestionapplication.NewDocumentRecallProjectionService(writer)
			adapter, _ := newDocumentEmbeddingProducerAdapterWithExecutor(test.executor, projections)
			result, err := adapter.ProduceDocumentEmbedding(context.Background(), command)
			if err != nil {
				t.Fatalf("ProduceDocumentEmbedding(): %v", err)
			}
			if result.Availability != test.availability || result.UnavailableReason != test.reason || writer.reads != test.reads {
				t.Fatalf("result=%#v reads=%d", result, writer.reads)
			}
		})
	}
}

func documentEmbeddingCommandFixture() ingestionapplication.ProduceDocumentEmbeddingCommand {
	return ingestionapplication.ProduceDocumentEmbeddingCommand{
		DocumentVersionID: 31, SourceConnectionID: 7,
		EmbedLocalRightsDecisionID: 71, RetainRightsDecisionID: 72,
		NormalizedTextSHA256: strings.Repeat("a", 64), Plaintext: "Canonical source document",
	}
}

func documentEmbeddingReceiptFixture(command ingestionapplication.ProduceDocumentEmbeddingCommand) ingestionapplication.DocumentEmbeddingReceiptResult {
	createdAt := time.Date(2026, time.August, 10, 8, 0, 0, 0, time.UTC)
	return ingestionapplication.DocumentEmbeddingReceiptResult{
		EmbeddingID: 101, DocumentVersionID: command.DocumentVersionID, SourceConnectionID: command.SourceConnectionID,
		EmbedLocalRightsDecisionID: command.EmbedLocalRightsDecisionID, RetainRightsDecisionID: command.RetainRightsDecisionID,
		ModelProfileID: 81, ModelProfileVersion: 2, ModelVersion: "embedding-v1",
		NormalizedTextSHA256: command.NormalizedTextSHA256, AIRunID: 91,
		CreatedAt: createdAt, RetentionUntil: createdAt.Add(24 * time.Hour),
		LifecycleState: ingestionapplication.RecallAssetLifecycleActive, Created: true,
	}
}
