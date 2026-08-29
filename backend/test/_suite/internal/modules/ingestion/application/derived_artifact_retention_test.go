package application

import (
	"context"
	"strings"
	"testing"
	"time"

	knowledgeapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/application"
)

func TestDerivedArtifactRetentionDeletesExactProjectionAndTombstonesMetadata(t *testing.T) {
	at := time.Date(2026, 8, 30, 4, 0, 0, 0, time.UTC)
	candidate := derivedArtifactRetentionCandidate(at, DerivedArtifactDeleteRightsRevoked)
	repository := &derivedArtifactRetentionRepositoryFake{candidates: []DerivedArtifactRetentionCandidateDTO{candidate}}
	deleter := &derivedArtifactProjectionDeleterFake{result: knowledgeapplication.ProjectionDeleteReceiptDTO{
		RelativePath: candidate.VaultRelativePath, SHA256: candidate.SHA256, SizeBytes: candidate.SizeBytes, Deleted: true,
	}}
	service, err := NewDerivedArtifactRetentionService(DerivedArtifactRetentionDependencies{Repository: repository, Deleter: deleter})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Run(context.Background(), RunDerivedArtifactRetentionCommand{At: at, Limit: 10})
	if err != nil || result.Claimed != 1 || result.Deleted != 1 || result.Failed != 0 || len(repository.completed) != 1 || len(repository.failed) != 0 {
		t.Fatalf("Run() = %#v/%v completed=%#v failed=%#v", result, err, repository.completed, repository.failed)
	}
	if repository.completed[0].ArtifactID != candidate.ArtifactID || repository.completed[0].VaultRelativePath != candidate.VaultRelativePath ||
		repository.completed[0].SHA256 != candidate.SHA256 || repository.completed[0].SizeBytes != candidate.SizeBytes {
		t.Fatalf("completion identity = %#v", repository.completed[0])
	}
}

func TestDerivedArtifactRetentionRecordsIntegrityFailureWithoutTombstoning(t *testing.T) {
	at := time.Date(2026, 8, 30, 4, 0, 0, 0, time.UTC)
	candidate := derivedArtifactRetentionCandidate(at, DerivedArtifactDeleteRetentionExpired)
	repository := &derivedArtifactRetentionRepositoryFake{candidates: []DerivedArtifactRetentionCandidateDTO{candidate}}
	deleter := &derivedArtifactProjectionDeleterFake{result: knowledgeapplication.ProjectionDeleteReceiptDTO{
		RelativePath: candidate.VaultRelativePath, SHA256: strings.Repeat("f", 64), SizeBytes: candidate.SizeBytes, Deleted: true,
	}}
	service, err := NewDerivedArtifactRetentionService(DerivedArtifactRetentionDependencies{Repository: repository, Deleter: deleter})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Run(context.Background(), RunDerivedArtifactRetentionCommand{At: at, Limit: 10})
	if err != nil || result.Failed != 1 || result.Deleted != 0 || len(repository.completed) != 0 || len(repository.failed) != 1 ||
		repository.failed[0].FailureCode != DerivedArtifactDeleteIntegrityFailed {
		t.Fatalf("Run() = %#v/%v completed=%#v failed=%#v", result, err, repository.completed, repository.failed)
	}
}

func derivedArtifactRetentionCandidate(at time.Time, reason string) DerivedArtifactRetentionCandidateDTO {
	profile := strings.Repeat("a", 64)
	return DerivedArtifactRetentionCandidateDTO{
		ArtifactID: 41, SourceConnectionID: 7, DocumentID: 11, DocumentVersionID: 13, AttemptNo: 2,
		ArtifactType: "markdown", TransformerProfileSHA256: profile,
		VaultRelativePath: "documents/11/13/markdown/" + profile + ".md",
		MIMEType:          "text/markdown; charset=utf-8", SHA256: strings.Repeat("b", 64), SizeBytes: 123,
		RetentionUntil: at.Add(-time.Hour), RetentionPolicyID: 17, RetentionPolicyVersion: 3, ReasonCode: reason,
	}
}

type derivedArtifactRetentionRepositoryFake struct {
	candidates []DerivedArtifactRetentionCandidateDTO
	completed  []CompleteDerivedArtifactDeletionCommand
	failed     []FailDerivedArtifactDeletionCommand
}

func (repository *derivedArtifactRetentionRepositoryFake) ClaimExpired(context.Context, time.Time, int) ([]DerivedArtifactRetentionCandidateDTO, error) {
	return append([]DerivedArtifactRetentionCandidateDTO(nil), repository.candidates...), nil
}

func (repository *derivedArtifactRetentionRepositoryFake) CompleteDeletion(_ context.Context, command CompleteDerivedArtifactDeletionCommand) error {
	repository.completed = append(repository.completed, command)
	return nil
}

func (repository *derivedArtifactRetentionRepositoryFake) FailDeletion(_ context.Context, command FailDerivedArtifactDeletionCommand) error {
	repository.failed = append(repository.failed, command)
	return nil
}

type derivedArtifactProjectionDeleterFake struct {
	result knowledgeapplication.ProjectionDeleteReceiptDTO
	err    error
}

func (deleter *derivedArtifactProjectionDeleterFake) DeleteProjection(context.Context, knowledgeapplication.DeleteStoredProjectionCommand) (knowledgeapplication.ProjectionDeleteReceiptDTO, error) {
	return deleter.result, deleter.err
}
