package application

import (
	"context"
	"strings"
	"testing"
	"time"

	ingestiondomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/domain"
)

type contentFamilyRepositoryFake struct {
	candidates []ContentFamilyCandidateDTO
	query      FindContentFamilyCandidatesQuery
	command    CommitContentFamilyDecisionCommand
	mutate     func(ContentFamilyDecisionDTO) ContentFamilyDecisionDTO
	reused     bool
}

type contentFamilyQualityReaderFake struct{ active bool }

func (fake contentFamilyQualityReaderFake) IsDecisionQualityProfileActive(context.Context, string, string) (bool, error) {
	return fake.active, nil
}

func (repository *contentFamilyRepositoryFake) FindContentFamilyCandidates(_ context.Context, query FindContentFamilyCandidatesQuery) ([]ContentFamilyCandidateDTO, error) {
	repository.query = query
	return append([]ContentFamilyCandidateDTO(nil), repository.candidates...), nil
}

func (repository *contentFamilyRepositoryFake) CommitContentFamilyDecision(_ context.Context, command CommitContentFamilyDecisionCommand) (ContentFamilyDecisionDTO, error) {
	repository.command = command
	root := command.RootDocumentVersionID
	if command.Action == "create" {
		root = command.DocumentVersionID
	}
	value := ContentFamilyDecisionDTO{DecisionID: 101, FamilyID: 201, FamilyVersion: 1,
		DocumentVersionID: command.DocumentVersionID, RootDocumentVersionID: root, Action: command.Action,
		Relation: command.Relation, HammingDistance: command.HammingDistance, MinHashSimilarity: command.MinHashSimilarity,
		DecisionProfileVersion: command.DecisionProfileVersion, ReasonCodes: append([]string(nil), command.ReasonCodes...)}
	value.Reused = repository.reused
	if repository.mutate != nil {
		value = repository.mutate(value)
	}
	return value, nil
}

func TestContentFamilyServiceAcceptsAnImmutableReplayAfterCandidateSetChanges(t *testing.T) {
	plaintext := "PostgreSQL summary that was first assigned before another copy arrived"
	fingerprint, err := ingestiondomain.BuildContentFingerprint(plaintext, "content-fingerprint-v1")
	if err != nil {
		t.Fatal(err)
	}
	repository := &contentFamilyRepositoryFake{
		reused: true,
		candidates: []ContentFamilyCandidateDTO{{
			FamilyID: 9, FamilyVersion: 4, RootDocumentVersionID: 30, Fingerprint: contentFingerprintDTO(fingerprint),
		}},
		mutate: func(value ContentFamilyDecisionDTO) ContentFamilyDecisionDTO {
			value.FamilyID = 7
			value.RootDocumentVersionID = 41
			value.Action = "create"
			value.Relation = "unrelated"
			value.HammingDistance = 64
			value.MinHashSimilarity = 0
			value.ReasonCodes = []string{"no_candidate"}
			return value
		},
	}
	service, _ := NewContentFamilyService(repository)
	now := time.Now().UTC()
	result, err := service.Assign(context.Background(), AssignDocumentContentFamilyCommand{
		SourceConnectionID: 1, DocumentVersionID: 41, DerivedArtifactID: 51,
		StoreDerivedRightsDecisionID: 61, RetainRightsDecisionID: 62, DecisionAt: now, RetentionUntil: now.Add(time.Hour),
		CanonicalPlaintext: plaintext, FingerprintProfile: "content-fingerprint-v1", DecisionProfileVersion: "content-family-decision-v1",
	})
	if err != nil {
		t.Fatalf("replay after candidate change failed: %v", err)
	}
	if !result.Decision.Reused || result.Decision.Action != "create" || result.Decision.FamilyID != 7 {
		t.Fatalf("replayed decision = %#v", result.Decision)
	}
}

func TestContentFamilyServicePersistsOnlyFingerprintAndDecisionFacts(t *testing.T) {
	repository := &contentFamilyRepositoryFake{}
	service, err := NewContentFamilyService(repository)
	if err != nil {
		t.Fatal(err)
	}
	plaintext := "华东园区发生爆燃，救援正在进行"
	now := time.Now().UTC()
	result, err := service.Assign(context.Background(), AssignDocumentContentFamilyCommand{
		SourceConnectionID: 1, DocumentVersionID: 41, DerivedArtifactID: 51,
		StoreDerivedRightsDecisionID: 61, RetainRightsDecisionID: 62, DecisionAt: now, RetentionUntil: now.Add(time.Hour),
		CanonicalPlaintext: plaintext, FingerprintProfile: "content-fingerprint-v1",
		DecisionProfileVersion: "content-family-decision-v1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Decision.Action != "create" || result.Decision.RootDocumentVersionID != 41 || repository.query.Limit != 50 {
		t.Fatalf("unexpected family assignment: %#v / %#v", result, repository.query)
	}
	if strings.Contains(repository.command.Fingerprint.NormalizedContentSHA256, "园区") ||
		strings.Contains(repository.command.DecisionProfileVersion, plaintext) || len(repository.command.Fingerprint.MinHash) != 64 {
		t.Fatalf("repository command leaked plaintext or omitted fingerprint: %#v", repository.command)
	}
}

func TestContentFamilyServiceJoinsExactCandidateAndRejectsReceiptDrift(t *testing.T) {
	plaintext := "Acme launches a new product"
	fingerprint, err := ingestiondomain.BuildContentFingerprint(plaintext, "content-fingerprint-v1")
	if err != nil {
		t.Fatal(err)
	}
	repository := &contentFamilyRepositoryFake{candidates: []ContentFamilyCandidateDTO{{
		FamilyID: 9, FamilyVersion: 4, RootDocumentVersionID: 30, Fingerprint: contentFingerprintDTO(fingerprint),
	}}}
	service, _ := NewContentFamilyService(repository)
	now := time.Now().UTC()
	result, err := service.Assign(context.Background(), AssignDocumentContentFamilyCommand{SourceConnectionID: 1, DocumentVersionID: 42,
		DerivedArtifactID: 52, StoreDerivedRightsDecisionID: 61, RetainRightsDecisionID: 62, DecisionAt: now, RetentionUntil: now.Add(time.Hour),
		CanonicalPlaintext: plaintext, FingerprintProfile: "content-fingerprint-v1", DecisionProfileVersion: "content-family-decision-v1"})
	if err != nil || result.Decision.Action != "join" || repository.command.ExpectedFamilyVersion != 4 || repository.command.Relation != "exact_copy" {
		t.Fatalf("exact assignment = %#v / %#v / %v", result, repository.command, err)
	}

	repository.mutate = func(value ContentFamilyDecisionDTO) ContentFamilyDecisionDTO {
		value.Relation = "near_duplicate"
		return value
	}
	if _, err := service.Assign(context.Background(), AssignDocumentContentFamilyCommand{SourceConnectionID: 1, DocumentVersionID: 43,
		DerivedArtifactID: 53, StoreDerivedRightsDecisionID: 61, RetainRightsDecisionID: 62, DecisionAt: now, RetentionUntil: now.Add(time.Hour),
		CanonicalPlaintext: plaintext, FingerprintProfile: "content-fingerprint-v1", DecisionProfileVersion: "content-family-decision-v1"}); err == nil {
		t.Fatal("receipt drift was accepted")
	}
}

func TestContentFamilyServiceDowngradesAutomaticJoinWithoutActiveQualityProfile(t *testing.T) {
	plaintext := "Acme launches a new product"
	fingerprint, err := ingestiondomain.BuildContentFingerprint(plaintext, "content-fingerprint-v1")
	if err != nil {
		t.Fatal(err)
	}
	repository := &contentFamilyRepositoryFake{candidates: []ContentFamilyCandidateDTO{{
		FamilyID: 9, FamilyVersion: 4, RootDocumentVersionID: 30, Fingerprint: contentFingerprintDTO(fingerprint),
	}}}
	service, err := NewContentFamilyServiceWithQualityProfiles(repository, contentFamilyQualityReaderFake{active: false})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	result, err := service.Assign(t.Context(), AssignDocumentContentFamilyCommand{SourceConnectionID: 1, DocumentVersionID: 42,
		DerivedArtifactID: 52, StoreDerivedRightsDecisionID: 61, RetainRightsDecisionID: 62, DecisionAt: now,
		RetentionUntil: now.Add(time.Hour), CanonicalPlaintext: plaintext, FingerprintProfile: "content-fingerprint-v1",
		DecisionProfileVersion: "content-family-conservative-v1"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Decision.Action != "review" || !containsContentFamilyReason(result.Decision.ReasonCodes, "quality_profile_not_active") {
		t.Fatalf("quality-gated family decision = %#v", result.Decision)
	}
}

func containsContentFamilyReason(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
