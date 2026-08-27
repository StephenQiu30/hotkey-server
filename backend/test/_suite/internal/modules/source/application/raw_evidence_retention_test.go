package application

import (
	"context"
	"errors"
	"testing"
	"time"
)

type rawEvidenceRetentionRepositoryFake struct {
	candidates []RawEvidenceRetentionCandidateDTO
	claimedAt  time.Time
	limit      int
	completed  []CompleteRawEvidenceDeletionCommand
	failed     []FailRawEvidenceDeletionCommand
}

func (repository *rawEvidenceRetentionRepositoryFake) ClaimExpired(_ context.Context, at time.Time, limit int) ([]RawEvidenceRetentionCandidateDTO, error) {
	repository.claimedAt, repository.limit = at, limit
	return append([]RawEvidenceRetentionCandidateDTO(nil), repository.candidates...), nil
}

func (repository *rawEvidenceRetentionRepositoryFake) CompleteDeletion(_ context.Context, command CompleteRawEvidenceDeletionCommand) error {
	repository.completed = append(repository.completed, command)
	return nil
}

func (repository *rawEvidenceRetentionRepositoryFake) FailDeletion(_ context.Context, command FailRawEvidenceDeletionCommand) error {
	repository.failed = append(repository.failed, command)
	return nil
}

type rawEvidenceObjectDeleterFake struct {
	result DeleteRawEvidenceObjectResult
	err    error
	calls  []DeleteRawEvidenceObjectCommand
}

func (deleter *rawEvidenceObjectDeleterFake) DeleteIfMatches(_ context.Context, command DeleteRawEvidenceObjectCommand) (DeleteRawEvidenceObjectResult, error) {
	deleter.calls = append(deleter.calls, command)
	return deleter.result, deleter.err
}

func TestRawEvidenceRetentionDeletesExpiredCandidatesAndTombstonesMetadata(t *testing.T) {
	at := time.Date(2026, 8, 27, 9, 0, 0, 0, time.UTC)
	candidate := RawEvidenceRetentionCandidateDTO{
		SnapshotID: 41, SourceConnectionID: 7, AttemptNo: 2,
		PayloadSHA256: repeatRetentionHex("b"), EvidenceKey: repeatRetentionHex("c"),
		RetentionUntil: at.Add(-time.Minute), RetentionPolicyID: 3, RetentionPolicyVersion: 4,
	}
	candidate.ObjectKey = RawEvidenceObjectKey(candidate.SourceConnectionID, candidate.EvidenceKey)
	repository := &rawEvidenceRetentionRepositoryFake{candidates: []RawEvidenceRetentionCandidateDTO{candidate}}
	deleter := &rawEvidenceObjectDeleterFake{result: DeleteRawEvidenceObjectResult{
		ObjectKey: candidate.ObjectKey, PayloadSHA256: candidate.PayloadSHA256, Deleted: true,
	}}
	service, err := NewRawEvidenceRetentionService(RawEvidenceRetentionDependencies{Repository: repository, Deleter: deleter})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Run(context.Background(), RunRawEvidenceRetentionCommand{At: at, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if result.Claimed != 1 || result.Deleted != 1 || result.Failed != 0 || len(repository.completed) != 1 || len(repository.failed) != 0 {
		t.Fatalf("unexpected retention result: %#v completed=%#v failed=%#v", result, repository.completed, repository.failed)
	}
	if len(deleter.calls) != 1 || deleter.calls[0].SnapshotID != candidate.SnapshotID || repository.completed[0].AttemptNo != candidate.AttemptNo || !repository.completed[0].DeletedAt.Equal(at) {
		t.Fatalf("candidate identity was not conserved: calls=%#v completed=%#v", deleter.calls, repository.completed)
	}
}

func TestRawEvidenceRetentionRecordsDeleteFailureWithoutTombstoning(t *testing.T) {
	at := time.Date(2026, 8, 27, 9, 0, 0, 0, time.UTC)
	candidate := RawEvidenceRetentionCandidateDTO{
		SnapshotID: 42, SourceConnectionID: 8, AttemptNo: 1,
		PayloadSHA256: repeatRetentionHex("e"), EvidenceKey: repeatRetentionHex("f"),
		RetentionUntil: at.Add(-time.Hour), RetentionPolicyID: 5, RetentionPolicyVersion: 6,
	}
	candidate.ObjectKey = RawEvidenceObjectKey(candidate.SourceConnectionID, candidate.EvidenceKey)
	repository := &rawEvidenceRetentionRepositoryFake{candidates: []RawEvidenceRetentionCandidateDTO{candidate}}
	deleter := &rawEvidenceObjectDeleterFake{err: errors.New("object store unavailable")}
	service, err := NewRawEvidenceRetentionService(RawEvidenceRetentionDependencies{Repository: repository, Deleter: deleter})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Run(context.Background(), RunRawEvidenceRetentionCommand{At: at, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if result.Claimed != 1 || result.Deleted != 0 || result.Failed != 1 || len(repository.completed) != 0 || len(repository.failed) != 1 {
		t.Fatalf("delete failure was not preserved for retry: %#v completed=%#v failed=%#v", result, repository.completed, repository.failed)
	}
	if repository.failed[0].FailureCode != RawEvidenceDeleteObjectFailed || !repository.failed[0].FailedAt.Equal(at) {
		t.Fatalf("unexpected stable failure receipt: %#v", repository.failed[0])
	}
}

func TestRawEvidenceRetentionRejectsMismatchedDeleteReceipt(t *testing.T) {
	at := time.Date(2026, 8, 27, 9, 0, 0, 0, time.UTC)
	candidate := RawEvidenceRetentionCandidateDTO{
		SnapshotID: 43, SourceConnectionID: 9, AttemptNo: 1,
		PayloadSHA256: repeatRetentionHex("2"), EvidenceKey: repeatRetentionHex("3"),
		RetentionUntil: at.Add(-time.Hour), RetentionPolicyID: 7, RetentionPolicyVersion: 8,
	}
	candidate.ObjectKey = RawEvidenceObjectKey(candidate.SourceConnectionID, candidate.EvidenceKey)
	repository := &rawEvidenceRetentionRepositoryFake{candidates: []RawEvidenceRetentionCandidateDTO{candidate}}
	deleter := &rawEvidenceObjectDeleterFake{result: DeleteRawEvidenceObjectResult{
		ObjectKey: candidate.ObjectKey, PayloadSHA256: repeatRetentionHex("4"), Deleted: true,
	}}
	service, err := NewRawEvidenceRetentionService(RawEvidenceRetentionDependencies{Repository: repository, Deleter: deleter})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Run(context.Background(), RunRawEvidenceRetentionCommand{At: at, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if result.Failed != 1 || len(repository.completed) != 0 || len(repository.failed) != 1 || repository.failed[0].FailureCode != RawEvidenceDeleteIntegrityFailed {
		t.Fatalf("mismatched delete receipt was not failed closed: %#v failed=%#v", result, repository.failed)
	}
}

func repeatRetentionHex(value string) string {
	result := ""
	for len(result) < 64 {
		result += value
	}
	return result[:64]
}
