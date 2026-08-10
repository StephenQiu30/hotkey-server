package application

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

type documentRecallProjectionWriterStub struct {
	searchCommand    PersistDocumentSearchProjectionCommand
	embeddingCommand PersistDocumentEmbeddingReceiptCommand
	searchResult     DocumentSearchProjectionResult
	embeddingResult  DocumentEmbeddingReceiptResult
	searchCalls      int
	embeddingCalls   int
}

func (stub *documentRecallProjectionWriterStub) PersistDocumentSearchProjection(_ context.Context, command PersistDocumentSearchProjectionCommand) (DocumentSearchProjectionResult, error) {
	stub.searchCalls++
	stub.searchCommand = command
	return stub.searchResult, nil
}

func (stub *documentRecallProjectionWriterStub) PersistDocumentEmbeddingReceipt(_ context.Context, command PersistDocumentEmbeddingReceiptCommand) (DocumentEmbeddingReceiptResult, error) {
	stub.embeddingCalls++
	stub.embeddingCommand = command
	return stub.embeddingResult, nil
}

func (stub *documentRecallProjectionWriterStub) ReadDocumentEmbeddingReceipt(_ context.Context, _ ReadDocumentEmbeddingReceiptQuery) (DocumentEmbeddingReceiptResult, error) {
	return stub.embeddingResult, nil
}

func TestDocumentRecallProjectionServiceNormalizesAndPersistsSensitiveSearchProjection(t *testing.T) {
	plaintext := "Canonical hot event body"
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte(plaintext)))
	indexedAt := time.Date(2026, 8, 9, 8, 0, 0, 0, time.UTC)
	command := PersistDocumentSearchProjectionCommand{
		DocumentVersionID: 31, DerivedArtifactID: 41,
		StoreDerivedRightsDecisionID: 51, RetainRightsDecisionID: 52,
		NormalizationProfileVersion: "canonical-search-v1", NormalizedTextSHA256: digest,
		Plaintext: plaintext, EntityKeys: []string{" OpenAI ", "openai"}, ActionKeys: []string{"LAUNCH"},
		LocationKeys: []string{" San Francisco "}, RegionKeys: []string{"US"}, IndexedAt: indexedAt,
	}
	writer := &documentRecallProjectionWriterStub{searchResult: DocumentSearchProjectionResult{
		ProjectionID: 61, DocumentVersionID: command.DocumentVersionID, SourceConnectionID: 7,
		DerivedArtifactID:            command.DerivedArtifactID,
		StoreDerivedRightsDecisionID: command.StoreDerivedRightsDecisionID,
		RetainRightsDecisionID:       command.RetainRightsDecisionID,
		NormalizationProfileVersion:  command.NormalizationProfileVersion,
		NormalizedTextSHA256:         digest, IndexedAt: indexedAt, RetentionUntil: indexedAt.Add(24 * time.Hour),
		LifecycleState: RecallAssetLifecycleActive, Created: true,
	}}
	service, err := NewDocumentRecallProjectionService(writer)
	if err != nil {
		t.Fatalf("NewDocumentRecallProjectionService() error = %v", err)
	}

	result, err := service.PersistSearchProjection(context.Background(), command)
	if err != nil {
		t.Fatalf("PersistSearchProjection() error = %v", err)
	}
	if result.ProjectionID != 61 || writer.searchCalls != 1 {
		t.Fatalf("result = %#v, calls = %d", result, writer.searchCalls)
	}
	if got := writer.searchCommand.EntityKeys; len(got) != 1 || got[0] != "openai" {
		t.Fatalf("normalized entity keys = %#v", got)
	}
	if got := writer.searchCommand.ActionKeys; len(got) != 1 || got[0] != "launch" {
		t.Fatalf("normalized action keys = %#v", got)
	}
	if writer.searchCommand.Plaintext != plaintext {
		t.Fatalf("plaintext changed before synchronous persistence")
	}
}

func TestDocumentRecallProjectionServiceRejectsMismatchedSearchProjectionReceipt(t *testing.T) {
	plaintext := "Canonical receipt identity"
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte(plaintext)))
	indexedAt := time.Date(2026, 8, 9, 8, 0, 0, 0, time.UTC)
	command := PersistDocumentSearchProjectionCommand{
		DocumentVersionID: 31, DerivedArtifactID: 41,
		StoreDerivedRightsDecisionID: 51, RetainRightsDecisionID: 52,
		NormalizationProfileVersion: "canonical-search-v1", NormalizedTextSHA256: digest,
		Plaintext: plaintext, IndexedAt: indexedAt,
	}
	valid := DocumentSearchProjectionResult{
		ProjectionID: 61, DocumentVersionID: 31, SourceConnectionID: 7, DerivedArtifactID: 41,
		StoreDerivedRightsDecisionID: 51, RetainRightsDecisionID: 52,
		NormalizationProfileVersion: "canonical-search-v1", NormalizedTextSHA256: digest,
		RetentionUntil: indexedAt.Add(24 * time.Hour), IndexedAt: indexedAt,
		LifecycleState: RecallAssetLifecycleActive,
	}
	tests := []struct {
		name   string
		mutate func(*DocumentSearchProjectionResult)
	}{
		{name: "source connection", mutate: func(result *DocumentSearchProjectionResult) { result.SourceConnectionID = 0 }},
		{name: "derived artifact", mutate: func(result *DocumentSearchProjectionResult) { result.DerivedArtifactID++ }},
		{name: "store derived rights", mutate: func(result *DocumentSearchProjectionResult) { result.StoreDerivedRightsDecisionID++ }},
		{name: "retain rights", mutate: func(result *DocumentSearchProjectionResult) { result.RetainRightsDecisionID++ }},
		{name: "indexed at", mutate: func(result *DocumentSearchProjectionResult) { result.IndexedAt = result.IndexedAt.Add(time.Second) }},
		{name: "expired retention", mutate: func(result *DocumentSearchProjectionResult) { result.RetentionUntil = indexedAt }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			receipt := valid
			test.mutate(&receipt)
			writer := &documentRecallProjectionWriterStub{searchResult: receipt}
			service, _ := NewDocumentRecallProjectionService(writer)
			if _, err := service.PersistSearchProjection(context.Background(), command); !errors.Is(err, sharedrepository.ErrConflict) {
				t.Fatalf("PersistSearchProjection() error = %v, want conflict", err)
			}
		})
	}
}

func TestDocumentRecallProjectionServiceRejectsDigestMismatchBeforeWriter(t *testing.T) {
	writer := &documentRecallProjectionWriterStub{}
	service, _ := NewDocumentRecallProjectionService(writer)
	_, err := service.PersistSearchProjection(context.Background(), PersistDocumentSearchProjectionCommand{
		DocumentVersionID: 1, DerivedArtifactID: 2, StoreDerivedRightsDecisionID: 3, RetainRightsDecisionID: 4,
		NormalizationProfileVersion: "canonical-search-v1", NormalizedTextSHA256: strings.Repeat("a", 64),
		Plaintext: "different", IndexedAt: time.Now().UTC(),
	})
	if !errors.Is(err, sharedrepository.ErrInvalidInput) || writer.searchCalls != 0 {
		t.Fatalf("error = %v, calls = %d", err, writer.searchCalls)
	}
}

func TestDocumentRecallProjectionServicePersistsExactEmbeddingReceipt(t *testing.T) {
	vector := make([]float32, 1024)
	vector[7] = 1
	createdAt := time.Date(2026, 8, 9, 8, 5, 0, 0, time.UTC)
	command := PersistDocumentEmbeddingReceiptCommand{
		DocumentVersionID: 31, EmbedLocalRightsDecisionID: 71, RetainRightsDecisionID: 72,
		ModelProfileID: 81, ModelProfileVersion: 2, ModelVersion: "qwen-embedding-v1",
		NormalizedTextSHA256: strings.Repeat("b", 64), Embedding: vector, AIRunID: 91, CreatedAt: createdAt,
	}
	writer := &documentRecallProjectionWriterStub{embeddingResult: DocumentEmbeddingReceiptResult{
		EmbeddingID: 101, DocumentVersionID: command.DocumentVersionID, SourceConnectionID: 7,
		EmbedLocalRightsDecisionID: command.EmbedLocalRightsDecisionID,
		RetainRightsDecisionID:     command.RetainRightsDecisionID,
		ModelProfileID:             command.ModelProfileID, ModelProfileVersion: command.ModelProfileVersion,
		ModelVersion: command.ModelVersion, NormalizedTextSHA256: command.NormalizedTextSHA256,
		AIRunID: command.AIRunID, CreatedAt: createdAt, RetentionUntil: createdAt.Add(24 * time.Hour),
		LifecycleState: RecallAssetLifecycleActive, Created: true,
	}}
	service, _ := NewDocumentRecallProjectionService(writer)

	result, err := service.PersistEmbeddingReceipt(context.Background(), command)
	if err != nil {
		t.Fatalf("PersistEmbeddingReceipt() error = %v", err)
	}
	if result.EmbeddingID != 101 || writer.embeddingCalls != 1 || len(writer.embeddingCommand.Embedding) != 1024 {
		t.Fatalf("result = %#v, calls = %d", result, writer.embeddingCalls)
	}
	command.Embedding[7] = 0
	if writer.embeddingCommand.Embedding[7] != 1 {
		t.Fatalf("writer command retained caller-owned vector")
	}
}

func TestDocumentRecallProjectionServiceRejectsMismatchedWriterReceipt(t *testing.T) {
	vector := make([]float32, 1024)
	vector[0] = 1
	writer := &documentRecallProjectionWriterStub{embeddingResult: DocumentEmbeddingReceiptResult{
		EmbeddingID: 5, DocumentVersionID: 999, SourceConnectionID: 2,
		EmbedLocalRightsDecisionID: 3, RetainRightsDecisionID: 4,
		ModelProfileID: 8, ModelProfileVersion: 1,
		ModelVersion: "model-v1", NormalizedTextSHA256: strings.Repeat("c", 64), LifecycleState: RecallAssetLifecycleActive,
		AIRunID: 9, CreatedAt: time.Now().UTC(), RetentionUntil: time.Now().UTC().Add(time.Hour),
	}}
	service, _ := NewDocumentRecallProjectionService(writer)
	_, err := service.PersistEmbeddingReceipt(context.Background(), PersistDocumentEmbeddingReceiptCommand{
		DocumentVersionID: 7, EmbedLocalRightsDecisionID: 3, RetainRightsDecisionID: 4,
		ModelProfileID: 8, ModelProfileVersion: 1, ModelVersion: "model-v1",
		NormalizedTextSHA256: strings.Repeat("c", 64), Embedding: vector, AIRunID: 9, CreatedAt: time.Now().UTC(),
	})
	if !errors.Is(err, sharedrepository.ErrConflict) {
		t.Fatalf("error = %v, want conflict", err)
	}
}
