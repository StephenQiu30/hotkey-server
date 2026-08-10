package application_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
	"time"

	eventapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/event/application"
	intelligenceapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/intelligence/application"
	intelligencedomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/intelligence/domain"
)

func TestAutomaticClaimEvidenceServiceUsesAuthorizedPlaintextAndPublishesCitedFacts(t *testing.T) {
	now := time.Date(2026, time.August, 10, 8, 0, 0, 0, time.UTC)
	body := "Acme 正式发布 Project Nova。"
	digest := sha256HexFixture(body)
	targets := &automaticEvidenceTargetFake{target: automaticEvidenceTargetFixture(now, digest, int64(len(body)))}
	projections := &automaticEvidenceProjectionFake{result: eventapplication.AutomaticClaimEvidenceProjectionDTO{
		Plaintext: body, MIMEType: "text/plain; charset=utf-8", SHA256: digest, SizeBytes: int64(len(body)),
	}}
	models := &automaticEvidenceModelFake{result: intelligenceapplication.StructuredExecutionResult{
		Status: "succeeded", Run: intelligencedomain.Run{ID: 23},
		Result: json.RawMessage(`{"claims":[{"subject":"Acme","predicate":"发布","object":"Project Nova","relation":"asserts","exact_quote":"Acme 正式发布 Project Nova。","relation_score":0.91,"qualifiers":[]}]}`),
	}}
	selectors := &automaticEvidenceSelectorFake{result: eventapplication.LocatedAutomaticQuoteSelectorDTO{
		ID: 29, Version: 1, DocumentVersionID: 11, ExactQuote: body, PlaintextSHA256: digest,
	}}
	facts := &automaticEvidenceFactRepositoryFake{target: eventapplication.ClaimEvidenceTargetDTO{
		MicroEventID: 7, MicroEventVersion: 3, DocumentVersionID: 11, TextQuoteSelectorID: 29,
		ContentFamilyID: 31, LineageRootID: 11, QuoteSHA256: sha256HexFixture(body), PlaintextSHA256: digest,
		SelectorVersion: "text-quote-selector-v1", SelectorRetentionUntil: now.Add(time.Hour), CurrentlyCitable: true,
		CapturedAt: now.Add(-time.Minute),
	}}
	evidence, err := eventapplication.NewClaimEvidenceService(facts)
	if err != nil {
		t.Fatal(err)
	}
	summaryRepository := &automaticEvidenceSummaryRepositoryFake{}
	summaries, err := eventapplication.NewEvidenceSummaryService(summaryRepository)
	if err != nil {
		t.Fatal(err)
	}
	service, err := eventapplication.NewAutomaticClaimEvidenceService(eventapplication.AutomaticClaimEvidenceDependencies{
		Targets: targets, Projections: projections, Models: models, Selectors: selectors,
		Evidence: evidence, Summaries: summaries, Clock: fixedAutomaticEvidenceClock{now},
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := service.Extract(context.Background(), eventapplication.AutomaticClaimEvidenceCommand{
		MicroEventID: 7, ExpectedEventVersion: 3, DocumentVersionID: 11,
	})
	if err != nil {
		t.Fatalf("Extract(): %v", err)
	}
	if result.Status != "succeeded" || result.ModelRunID != 23 || len(result.Evidence) != 1 ||
		result.EvidenceState == nil || result.Summary == nil || targets.calls != 2 || selectors.calls != 1 {
		t.Fatalf("result/calls = %#v / %d / %d", result, targets.calls, selectors.calls)
	}
	if models.input.InputSchemaVersion != "v2" || models.input.SchemaVersion != "v2" ||
		models.input.PromptVersion != eventapplication.AtomicClaimEvidencePromptVersion ||
		strings.Contains(string(models.input.Input), "object_key") || !strings.Contains(string(models.input.Input), body) {
		t.Fatalf("model input = %#v / %s", models.input, models.input.Input)
	}
	if facts.committed.Origin != "automatic" || facts.committed.ModelRunID == nil || *facts.committed.ModelRunID != 23 ||
		facts.committed.ModelRelationScore == nil || *facts.committed.ModelRelationScore != .91 {
		t.Fatalf("evidence mutation = %#v", facts.committed)
	}
	if len(summaryRepository.command.Sentences) != 1 || summaryRepository.command.Sentences[0].EditorialNote ||
		len(summaryRepository.command.Sentences[0].ClaimEvidenceVersionIDs) != 1 || summaryRepository.command.Sentences[0].ClaimEvidenceVersionIDs[0] != 41 {
		t.Fatalf("summary command = %#v", summaryRepository.command)
	}
}

func TestAutomaticClaimEvidenceServiceDegradesBeforeReadingBodyWhenExternalModelIsNotAuthorized(t *testing.T) {
	now := time.Date(2026, time.August, 10, 8, 0, 0, 0, time.UTC)
	target := automaticEvidenceTargetFixture(now, strings.Repeat("a", 64), 32)
	target.ExternalModelAllowed = false
	targets := &automaticEvidenceTargetFake{target: target}
	projections := &automaticEvidenceProjectionFake{}
	models := &automaticEvidenceModelFake{}
	facts := &automaticEvidenceFactRepositoryFake{}
	evidence, err := eventapplication.NewClaimEvidenceService(facts)
	if err != nil {
		t.Fatal(err)
	}
	summaries, err := eventapplication.NewEvidenceSummaryService(&automaticEvidenceSummaryRepositoryFake{})
	if err != nil {
		t.Fatal(err)
	}
	service, err := eventapplication.NewAutomaticClaimEvidenceService(eventapplication.AutomaticClaimEvidenceDependencies{
		Targets: targets, Projections: projections, Models: models, Selectors: &automaticEvidenceSelectorFake{},
		Evidence: evidence, Summaries: summaries, Clock: fixedAutomaticEvidenceClock{now},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Extract(context.Background(), eventapplication.AutomaticClaimEvidenceCommand{
		MicroEventID: 7, ExpectedEventVersion: 3, DocumentVersionID: 11,
	})
	if err != nil || result.Status != "degraded" || result.ReasonCode != "external_model_not_authorized" ||
		targets.calls != 1 || projections.calls != 0 || models.calls != 0 {
		t.Fatalf("result/error/calls = %#v / %v / %d/%d/%d", result, err, targets.calls, projections.calls, models.calls)
	}
}

func TestAutomaticClaimEvidenceServiceDegradesBeforeReadingBodyWithoutActiveQualityProfile(t *testing.T) {
	now := time.Date(2026, time.August, 10, 8, 0, 0, 0, time.UTC)
	targets := &automaticEvidenceTargetFake{target: automaticEvidenceTargetFixture(now, strings.Repeat("a", 64), 32)}
	projections := &automaticEvidenceProjectionFake{}
	models := &automaticEvidenceModelFake{}
	evidence, _ := eventapplication.NewClaimEvidenceService(&automaticEvidenceFactRepositoryFake{})
	summaries, _ := eventapplication.NewEvidenceSummaryService(&automaticEvidenceSummaryRepositoryFake{})
	service, err := eventapplication.NewAutomaticClaimEvidenceService(eventapplication.AutomaticClaimEvidenceDependencies{
		Targets: targets, Projections: projections, Models: models, Selectors: &automaticEvidenceSelectorFake{}, Evidence: evidence,
		Summaries: summaries, Clock: fixedAutomaticEvidenceClock{now}, QualityProfiles: automaticEvidenceQualityReaderFake{active: false},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Extract(t.Context(), eventapplication.AutomaticClaimEvidenceCommand{MicroEventID: 7, ExpectedEventVersion: 3, DocumentVersionID: 11})
	if err != nil || result.Status != "degraded" || result.ReasonCode != "quality_profile_not_active" || projections.calls != 0 || models.calls != 0 {
		t.Fatalf("quality-gated evidence result=%#v error=%v body/model=%d/%d", result, err, projections.calls, models.calls)
	}
}

type automaticEvidenceQualityReaderFake struct{ active bool }

func (fake automaticEvidenceQualityReaderFake) IsDecisionQualityProfileActive(context.Context, string, string) (bool, error) {
	return fake.active, nil
}

type fixedAutomaticEvidenceClock struct{ at time.Time }

func (clock fixedAutomaticEvidenceClock) Now() time.Time { return clock.at }

type automaticEvidenceTargetFake struct {
	target eventapplication.AutomaticClaimEvidenceTargetDTO
	calls  int
}

func (fake *automaticEvidenceTargetFake) ReadAutomaticClaimEvidenceTarget(_ context.Context, query eventapplication.AutomaticClaimEvidenceTargetQuery) (eventapplication.AutomaticClaimEvidenceTargetDTO, error) {
	fake.calls++
	value := fake.target
	value.DecisionAt = query.DecisionAt
	return value, nil
}

type automaticEvidenceProjectionFake struct {
	result eventapplication.AutomaticClaimEvidenceProjectionDTO
	calls  int
}

func (fake *automaticEvidenceProjectionFake) ReadAutomaticClaimEvidenceProjection(context.Context, eventapplication.AutomaticClaimEvidenceProjectionQuery) (eventapplication.AutomaticClaimEvidenceProjectionDTO, error) {
	fake.calls++
	return fake.result, nil
}

type automaticEvidenceModelFake struct {
	input  intelligenceapplication.StructuredExecutionInput
	result intelligenceapplication.StructuredExecutionResult
	calls  int
}

func (fake *automaticEvidenceModelFake) ExecuteStructured(_ context.Context, input intelligenceapplication.StructuredExecutionInput) (intelligenceapplication.StructuredExecutionResult, error) {
	fake.calls++
	fake.input = input
	return fake.result, nil
}

type automaticEvidenceSelectorFake struct {
	result eventapplication.LocatedAutomaticQuoteSelectorDTO
	calls  int
}

func (fake *automaticEvidenceSelectorFake) LocateAutomaticQuoteSelector(_ context.Context, _ eventapplication.LocateAutomaticQuoteSelectorCommand) (eventapplication.LocatedAutomaticQuoteSelectorDTO, error) {
	fake.calls++
	return fake.result, nil
}

type automaticEvidenceFactRepositoryFake struct {
	target    eventapplication.ClaimEvidenceTargetDTO
	committed eventapplication.CommitClaimEvidenceCommand
}

func (fake *automaticEvidenceFactRepositoryFake) ReadClaimEvidenceTarget(_ context.Context, _ eventapplication.ClaimEvidenceTargetQuery) (eventapplication.ClaimEvidenceTargetDTO, error) {
	return fake.target, nil
}
func (fake *automaticEvidenceFactRepositoryFake) CommitClaimEvidence(_ context.Context, command eventapplication.CommitClaimEvidenceCommand) (eventapplication.RecordClaimEvidenceResult, error) {
	fake.committed = command
	return eventapplication.RecordClaimEvidenceResult{Created: true,
		Claim: eventapplication.ClaimDTO{ID: 37, Version: 1, MicroEventID: command.Target.MicroEventID,
			MicroEventVersion: command.Target.MicroEventVersion, ClaimHash: command.ClaimHash,
			Subject: command.Subject, Predicate: command.Predicate, Object: command.Object, Qualifiers: command.Qualifiers},
		Evidence: eventapplication.ClaimEvidenceVersionDTO{ID: 41, Version: 1, ClaimID: 37,
			DocumentVersionID: command.Target.DocumentVersionID, TextQuoteSelectorID: command.Target.TextQuoteSelectorID,
			ContentFamilyID: command.Target.ContentFamilyID, LineageRootID: command.Target.LineageRootID,
			Relation: command.Relation, ExtractionSchemaVersion: command.ExtractionSchemaVersion, Origin: command.Origin,
			ModelRunID: command.ModelRunID, ModelRelationScore: command.ModelRelationScore,
			QuoteSHA256: command.Target.QuoteSHA256, PlaintextSHA256: command.Target.PlaintextSHA256,
			SelectorVersion: command.Target.SelectorVersion, RetentionUntil: command.Target.SelectorRetentionUntil,
			CapturedAt: command.Target.CapturedAt, CreatedAt: command.DecisionAt}}, nil
}
func (fake *automaticEvidenceFactRepositoryFake) ReadEvidenceStateTarget(_ context.Context, query eventapplication.EvidenceStateTargetQuery) (eventapplication.EvidenceStateTargetDTO, error) {
	return eventapplication.EvidenceStateTargetDTO{MicroEventID: query.MicroEventID, EventVersion: query.EventVersion,
		ProfileID: 43, AlgorithmVersion: query.AlgorithmVersion,
		Items: []eventapplication.EvidenceStateItemDTO{{ClaimEvidenceVersionID: 41, LineageRootID: 11, Relation: "asserts", Citable: true}}}, nil
}
func (fake *automaticEvidenceFactRepositoryFake) CommitEvidenceStateSnapshot(_ context.Context, command eventapplication.CommitEvidenceStateSnapshotCommand) (eventapplication.EvidenceStateSnapshotDTO, error) {
	return eventapplication.EvidenceStateSnapshotDTO{ID: 47, Version: 1, MicroEventID: command.MicroEventID,
		EventVersion: command.EventVersion, ProfileID: command.ProfileID, AlgorithmVersion: command.AlgorithmVersion,
		EvidenceSetHash: command.EvidenceSetHash, State: command.State, IndependentOriginCount: command.IndependentOriginCount,
		ReasonCodes: command.ReasonCodes, ClaimEvidenceVersionIDs: command.ClaimEvidenceVersionIDs, CalculatedAt: command.CalculatedAt, Created: true}, nil
}
func (*automaticEvidenceFactRepositoryFake) ReadClaimEvidenceCorrectionTarget(context.Context, eventapplication.ClaimEvidenceCorrectionTargetQuery) (eventapplication.ClaimEvidenceCorrectionTargetDTO, error) {
	panic("unexpected correction")
}
func (*automaticEvidenceFactRepositoryFake) CommitClaimEvidenceCorrection(context.Context, eventapplication.CommitClaimEvidenceCorrectionCommand) (eventapplication.CorrectClaimEvidenceResult, error) {
	panic("unexpected correction")
}

type automaticEvidenceSummaryRepositoryFake struct {
	command eventapplication.CommitEvidenceSummaryCommand
}

func (fake *automaticEvidenceSummaryRepositoryFake) CommitEvidenceSummary(_ context.Context, command eventapplication.CommitEvidenceSummaryCommand) (eventapplication.EvidenceSummaryDTO, error) {
	fake.command = command
	sentences := make([]eventapplication.EvidenceSummarySentenceDTO, len(command.Sentences))
	for index, sentence := range command.Sentences {
		sentences[index] = eventapplication.EvidenceSummarySentenceDTO{ID: int64(51 + index), Version: 1, SummaryID: 49,
			Ordinal: index, Text: sentence.Text, EditorialNote: sentence.EditorialNote,
			ClaimEvidenceVersionIDs: sentence.ClaimEvidenceVersionIDs, DecisionOrigin: sentence.DecisionOrigin,
			ModelRunID: sentence.ModelRunID, ActorUserID: sentence.ActorUserID, CreatedAt: command.CreatedAt}
	}
	return eventapplication.EvidenceSummaryDTO{ID: 49, Version: 1, MicroEventID: command.MicroEventID,
		EventVersion: command.EventVersion, SummaryProfileVersion: command.SummaryProfileVersion,
		Sentences: sentences, CreatedAt: command.CreatedAt, Created: true}, nil
}

func automaticEvidenceTargetFixture(now time.Time, digest string, size int64) eventapplication.AutomaticClaimEvidenceTargetDTO {
	return eventapplication.AutomaticClaimEvidenceTargetDTO{MicroEventID: 7, EventVersion: 3, EventKey: "evt-7",
		ExternalModelAllowed: true, Artifact: eventapplication.AutomaticClaimEvidenceArtifactDTO{DocumentID: 9,
			DocumentVersionID: 11, ArtifactType: "plaintext", TransformerProfileSHA256: strings.Repeat("a", 64),
			MIMEType: "text/plain; charset=utf-8", PlaintextSHA256: digest, SizeBytes: size, RetentionUntil: now.Add(time.Hour)}}
}

func sha256HexFixture(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
