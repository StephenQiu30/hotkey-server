package application

import (
	"context"
	"strings"
	"testing"
	"time"
)

type claimEvidenceRepositoryFake struct {
	target EvidenceStateTargetDTO
	quote  ClaimEvidenceTargetDTO
	commit CommitClaimEvidenceCommand
	state  CommitEvidenceStateSnapshotCommand
}

func (fake *claimEvidenceRepositoryFake) ReadClaimEvidenceTarget(context.Context, ClaimEvidenceTargetQuery) (ClaimEvidenceTargetDTO, error) {
	return fake.quote, nil
}
func (fake *claimEvidenceRepositoryFake) CommitClaimEvidence(_ context.Context, command CommitClaimEvidenceCommand) (RecordClaimEvidenceResult, error) {
	fake.commit = command
	return RecordClaimEvidenceResult{Created: true, Claim: ClaimDTO{ID: 1, Version: 1, MicroEventID: command.Target.MicroEventID,
		MicroEventVersion: command.Target.MicroEventVersion, ClaimHash: command.ClaimHash, Subject: command.Subject,
		Predicate: command.Predicate, Object: command.Object, Qualifiers: command.Qualifiers},
		Evidence: ClaimEvidenceVersionDTO{ID: 2, Version: 1, ClaimID: 1, DocumentVersionID: command.Target.DocumentVersionID,
			TextQuoteSelectorID: command.Target.TextQuoteSelectorID, ContentFamilyID: command.Target.ContentFamilyID,
			LineageRootID: command.Target.LineageRootID, Relation: command.Relation,
			ExtractionSchemaVersion: command.ExtractionSchemaVersion, Origin: command.Origin}}, nil
}
func (fake *claimEvidenceRepositoryFake) ReadEvidenceStateTarget(context.Context, EvidenceStateTargetQuery) (EvidenceStateTargetDTO, error) {
	return fake.target, nil
}
func (fake *claimEvidenceRepositoryFake) CommitEvidenceStateSnapshot(_ context.Context, command CommitEvidenceStateSnapshotCommand) (EvidenceStateSnapshotDTO, error) {
	fake.state = command
	return EvidenceStateSnapshotDTO{ID: 3, Version: 1, MicroEventID: command.MicroEventID, EventVersion: command.EventVersion,
		ProfileID: command.ProfileID, AlgorithmVersion: command.AlgorithmVersion, EvidenceSetHash: command.EvidenceSetHash,
		State: command.State, IndependentOriginCount: command.IndependentOriginCount, ReasonCodes: command.ReasonCodes,
		ClaimEvidenceVersionIDs: command.ClaimEvidenceVersionIDs, CalculatedAt: command.CalculatedAt, Created: true}, nil
}
func (fake *claimEvidenceRepositoryFake) ReadClaimEvidenceCorrectionTarget(context.Context, ClaimEvidenceCorrectionTargetQuery) (ClaimEvidenceCorrectionTargetDTO, error) {
	return ClaimEvidenceCorrectionTargetDTO{}, nil
}
func (fake *claimEvidenceRepositoryFake) CommitClaimEvidenceCorrection(context.Context, CommitClaimEvidenceCorrectionCommand) (CorrectClaimEvidenceResult, error) {
	return CorrectClaimEvidenceResult{}, nil
}

func TestClaimEvidenceServiceRecordsCanonicalPOJOFacts(t *testing.T) {
	now := time.Now().UTC()
	modelRunID := int64(7)
	fake := &claimEvidenceRepositoryFake{quote: ClaimEvidenceTargetDTO{MicroEventID: 1, MicroEventVersion: 2,
		DocumentVersionID: 3, TextQuoteSelectorID: 4, ContentFamilyID: 5, LineageRootID: 6,
		QuoteSHA256: strings.Repeat("a", 64), PlaintextSHA256: strings.Repeat("b", 64), SelectorVersion: "selector-v1",
		SelectorRetentionUntil: now.Add(time.Hour), CurrentlyCitable: true}}
	service, _ := NewClaimEvidenceService(fake)
	result, err := service.Record(context.Background(), RecordClaimEvidenceCommand{MicroEventID: 1, ExpectedEventVersion: 2,
		DocumentVersionID: 3, TextQuoteSelectorID: 4, Subject: "  Alice  ", Predicate: " announced ", Object: " launch ",
		Qualifiers: []ClaimQualifierDTO{{Key: "time", Value: "today"}}, Relation: "asserts", ModelRunID: &modelRunID,
		ExtractionSchemaVersion: CanonicalClaimExtractionSchemaVersion, Origin: "automatic",
		IdempotencyKey: "claim-evidence-1", DecisionAt: now})
	if err != nil || result.Evidence.ID != 2 || fake.commit.Subject != "Alice" || len(fake.commit.CommandFingerprint) != 64 {
		t.Fatalf("Record() = %#v, command=%#v, error=%v", result, fake.commit, err)
	}
}

func TestClaimEvidenceServiceCanonicalizesMissingQualifiersToEmptyArray(t *testing.T) {
	now := time.Now().UTC()
	actorID := int64(8)
	fake := &claimEvidenceRepositoryFake{quote: ClaimEvidenceTargetDTO{MicroEventID: 1, MicroEventVersion: 2,
		DocumentVersionID: 3, TextQuoteSelectorID: 4, ContentFamilyID: 5, LineageRootID: 6,
		QuoteSHA256: strings.Repeat("a", 64), PlaintextSHA256: strings.Repeat("b", 64), SelectorVersion: "selector-v1",
		SelectorRetentionUntil: now.Add(time.Hour), CurrentlyCitable: true}}
	service, _ := NewClaimEvidenceService(fake)
	_, err := service.Record(context.Background(), RecordClaimEvidenceCommand{MicroEventID: 1, ExpectedEventVersion: 2,
		DocumentVersionID: 3, TextQuoteSelectorID: 4, Subject: "Alice", Predicate: "announced", Object: "launch",
		Relation: "asserts", ExtractionSchemaVersion: CanonicalClaimExtractionSchemaVersion, Origin: "manual",
		ActorUserID: &actorID, IdempotencyKey: "claim-evidence-empty-qualifiers", DecisionAt: now})
	if err != nil {
		t.Fatalf("Record() error = %v", err)
	}
	if fake.commit.Qualifiers == nil || len(fake.commit.Qualifiers) != 0 {
		t.Fatalf("canonical qualifiers = %#v, want non-nil empty array", fake.commit.Qualifiers)
	}
}

func TestClaimEvidenceServiceCalculatesEvidenceStateSnapshot(t *testing.T) {
	now := time.Now().UTC()
	fake := &claimEvidenceRepositoryFake{target: EvidenceStateTargetDTO{MicroEventID: 10, EventVersion: 4, ProfileID: 9,
		AlgorithmVersion: CanonicalEvidenceStateAlgorithmVersion, Items: []EvidenceStateItemDTO{
			{ClaimEvidenceVersionID: 2, LineageRootID: 20, Relation: "asserts", Citable: true},
			{ClaimEvidenceVersionID: 1, LineageRootID: 10, Relation: "asserts", Citable: true},
		}}}
	service, _ := NewClaimEvidenceService(fake)
	result, err := service.CalculateState(context.Background(), CalculateEvidenceStateCommand{MicroEventID: 10,
		ExpectedEventVersion: 4, AlgorithmVersion: CanonicalEvidenceStateAlgorithmVersion, CalculatedAt: now})
	if err != nil || result.Snapshot.State != "multiple_origins" || result.Snapshot.IndependentOriginCount != 2 ||
		fake.state.ClaimEvidenceVersionIDs[0] != 1 || len(fake.state.EvidenceSetHash) != 64 {
		t.Fatalf("CalculateState() = %#v, command=%#v, error=%v", result, fake.state, err)
	}
}
