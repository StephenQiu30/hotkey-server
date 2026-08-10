package bootstrap

import (
	"context"
	"strings"
	"testing"
	"time"

	intelligenceapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/intelligence/application"
	monitorapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/monitor/application"
)

type compiledIntentEmbeddingExecutorFake struct {
	input     intelligenceapplication.ProjectedEmbeddingExecutionInput
	result    intelligenceapplication.ProjectedEmbeddingExecutionResult
	generated intelligenceapplication.GeneratedEmbeddingDTO
}

func (fake *compiledIntentEmbeddingExecutorFake) ExecuteProjectedEmbedding(ctx context.Context, input intelligenceapplication.ProjectedEmbeddingExecutionInput, sink intelligenceapplication.ProjectedEmbeddingSink) (intelligenceapplication.ProjectedEmbeddingExecutionResult, error) {
	fake.input = input
	if fake.generated.AIRunID > 0 {
		fake.generated.TargetType = input.TargetType
		fake.generated.TargetID = input.TargetID
		fake.generated.InputHash = input.InputHash
		if err := sink.CommitGeneratedEmbedding(ctx, fake.generated); err != nil {
			return intelligenceapplication.ProjectedEmbeddingExecutionResult{}, err
		}
	}
	return fake.result, nil
}

type compiledIntentEmbeddingRepositoryFake struct {
	command monitorapplication.PersistCompiledIntentEmbeddingCommand
	receipt monitorapplication.CompiledIntentEmbeddingReceiptDTO
}

func (fake *compiledIntentEmbeddingRepositoryFake) PersistCompiledIntentEmbedding(_ context.Context, command monitorapplication.PersistCompiledIntentEmbeddingCommand) (monitorapplication.CompiledIntentEmbeddingReceiptDTO, error) {
	fake.command = command
	return fake.receipt, nil
}

func (fake *compiledIntentEmbeddingRepositoryFake) ReadCompiledIntentEmbedding(context.Context, monitorapplication.ReadCompiledIntentEmbeddingQuery) (monitorapplication.CompiledIntentEmbeddingReceiptDTO, error) {
	return fake.receipt, nil
}

func TestCompiledIntentEmbeddingProducerAdapterPersistsExactProjectedVector(t *testing.T) {
	createdAt := time.Date(2026, time.August, 10, 10, 0, 0, 0, time.UTC)
	inputHash := strings.Repeat("b", 64)
	vector := make([]float32, 1024)
	vector[3] = 1
	repository := &compiledIntentEmbeddingRepositoryFake{receipt: monitorapplication.CompiledIntentEmbeddingReceiptDTO{
		EmbeddingID: 101, CompiledProfileID: 71, ConfigVersionID: 31,
		ModelProfileID: 81, ModelProfileVersion: 2, ModelVersion: "embedding-v1",
		InputHash: inputHash, AIRunID: 91, CreatedAt: createdAt, Created: true,
	}}
	projections, err := monitorapplication.NewCompiledIntentEmbeddingProjectionService(repository)
	if err != nil {
		t.Fatal(err)
	}
	executor := &compiledIntentEmbeddingExecutorFake{
		generated: intelligenceapplication.GeneratedEmbeddingDTO{
			ModelProfileID: 81, ModelProfileVersion: 2, ModelVersion: "embedding-v1",
			AIRunID: 91, Vector: vector, CreatedAt: createdAt,
		},
		result: intelligenceapplication.ProjectedEmbeddingExecutionResult{
			Status: "succeeded", TargetID: 71, ModelProfileID: 81, ModelProfileVersion: 2,
			ModelVersion: "embedding-v1", AIRunID: 91, InputHash: inputHash,
		},
	}
	adapter, err := newCompiledIntentEmbeddingProducerAdapterWithExecutor(executor, projections)
	if err != nil {
		t.Fatal(err)
	}
	result, err := adapter.ProduceCompiledIntentEmbedding(context.Background(), monitorapplication.ProduceCompiledIntentEmbeddingCommand{
		CompiledProfileID: 71, ConfigVersionID: 31, InputHash: inputHash, Input: "objective\nlaunch\nhotkey",
	})
	if err != nil {
		t.Fatalf("ProduceCompiledIntentEmbedding(): %v", err)
	}
	if result.Availability != monitorapplication.IntentEmbeddingAvailabilityReady || result.Receipt == nil || result.Receipt.EmbeddingID != 101 ||
		executor.input.TargetType != intelligenceapplication.ProjectedEmbeddingTargetMonitorCompiledProfile || executor.input.TargetID != 71 || executor.input.InputHash != inputHash ||
		repository.command.ConfigVersionID != 31 || len(repository.command.Embedding) != 1024 {
		t.Fatalf("result=%#v input=%#v command=%#v", result, executor.input, repository.command)
	}
}

func TestCompiledIntentEmbeddingProducerAdapterRepresentsModelUnavailable(t *testing.T) {
	repository := &compiledIntentEmbeddingRepositoryFake{}
	projections, _ := monitorapplication.NewCompiledIntentEmbeddingProjectionService(repository)
	inputHash := strings.Repeat("c", 64)
	executor := &compiledIntentEmbeddingExecutorFake{result: intelligenceapplication.ProjectedEmbeddingExecutionResult{
		Status: "degraded", ReasonCode: intelligenceapplication.ProjectedEmbeddingReasonModelUnavailable,
		TargetID: 71, InputHash: inputHash,
	}}
	adapter, _ := newCompiledIntentEmbeddingProducerAdapterWithExecutor(executor, projections)
	result, err := adapter.ProduceCompiledIntentEmbedding(context.Background(), monitorapplication.ProduceCompiledIntentEmbeddingCommand{
		CompiledProfileID: 71, ConfigVersionID: 31, InputHash: inputHash, Input: "objective\nlaunch",
	})
	if err != nil || result.Availability != monitorapplication.IntentEmbeddingAvailabilityDegraded ||
		result.UnavailableReason != monitorapplication.IntentSemanticModelUnavailable || result.Receipt != nil {
		t.Fatalf("result=%#v error=%v", result, err)
	}
}
