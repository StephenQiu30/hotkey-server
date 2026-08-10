package application

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestPublishedDocumentMatchServicePersistsConservativeReviewFactsWithoutReranker(t *testing.T) {
	recall := &publishedMatchRecallFake{result: HybridRecallResult{
		MonitorID: 7, Purpose: "published", ConfigVersionID: 11, MonitorVersionID: 11,
		CompiledProfileID: 13, MatchingAlgorithmVersion: HybridRecallMatchingAlgorithmVersion,
		Degraded: true, DegradationReasons: []string{"semantic_model_unavailable"},
		Candidates: []HybridRecallCandidateDTO{{
			DocumentVersionID: 17, RRFScore: 0.02,
			Signals: []RecallSignalDTO{{Channel: "lexical", Rank: 1, RawScore: 0.75, AlgorithmVersion: LexicalRecallAlgorithmVersion}},
		}},
	}}
	profiles := &documentMatchProfileFake{profile: RelevanceDecisionProfileDTO{
		ID: 19, Version: 1, EvaluationRunID: 29, MatchingAlgorithmVersion: HybridRecallMatchingAlgorithmVersion,
		RerankerVersion: "cross-encoder-v1", CalibrationVersion: "uncalibrated-v1",
		Status: "uncalibrated", RejectThreshold: 0.40, AcceptThreshold: 0.80,
	}}
	repository := &documentMatchRepositoryFake{}
	service, err := NewPublishedDocumentMatchService(recall, profiles, repository, nil, fixedDocumentMatchClock{now: time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("NewPublishedDocumentMatchService(): %v", err)
	}
	result, err := service.EvaluatePublished(context.Background(), EvaluatePublishedDocumentMatchesCommand{
		MonitorID: 7, MonitorVersionID: 11, CompiledProfileID: 13, RelevanceProfileID: 19,
	})
	if err != nil {
		t.Fatalf("EvaluatePublished(): %v", err)
	}
	if len(result.Decisions) != 1 || result.Decisions[0].Decision != "review" || !result.Decisions[0].Degraded || result.Decisions[0].RelevanceProbability != nil {
		t.Fatalf("decision = %#v, want degraded review without probability", result.Decisions)
	}
	if len(repository.commands) != 1 || repository.commands[0].DocumentVersionID != 17 || repository.commands[0].InputHash == "" {
		t.Fatalf("persist commands = %#v", repository.commands)
	}
	if !containsDocumentMatchReason(repository.commands[0].ReasonCodes, "relevance_reranker_unavailable") ||
		!containsDocumentMatchReason(repository.commands[0].ReasonCodes, "semantic_model_unavailable") {
		t.Fatalf("reason codes = %#v", repository.commands[0].ReasonCodes)
	}
}

func TestPublishedDocumentMatchServiceAppliesExactActivePlattProfile(t *testing.T) {
	recall := &publishedMatchRecallFake{result: HybridRecallResult{
		MonitorID: 7, Purpose: "published", ConfigVersionID: 11, MonitorVersionID: 11,
		CompiledProfileID: 13, MatchingAlgorithmVersion: HybridRecallMatchingAlgorithmVersion,
		Candidates: []HybridRecallCandidateDTO{
			{DocumentVersionID: 17, RRFScore: .04, Signals: []RecallSignalDTO{
				{Channel: "lexical", Rank: 1, RawScore: .9, AlgorithmVersion: LexicalRecallAlgorithmVersion},
				{Channel: "semantic", Rank: 1, RawScore: .9, AlgorithmVersion: SemanticRecallAlgorithmVersion},
				{Channel: "structured", Rank: 1, RawScore: 3, AlgorithmVersion: StructuredRecallAlgorithmVersion},
			}},
			{DocumentVersionID: 18, RRFScore: .01, Signals: []RecallSignalDTO{
				{Channel: "lexical", Rank: 100, RawScore: .01, AlgorithmVersion: LexicalRecallAlgorithmVersion},
			}},
		},
	}}
	profiles := &documentMatchProfileFake{profile: RelevanceDecisionProfileDTO{
		ID: 19, Version: 1, EvaluationRunID: 29, MatchingAlgorithmVersion: HybridRecallMatchingAlgorithmVersion,
		RerankerVersion: CanonicalDocumentMatchRerankerVersion, CalibrationVersion: CanonicalDocumentMatchCalibrationVersion,
		Status: "active", RejectThreshold: .4, AcceptThreshold: .8, CalibrationSlope: 1.2, CalibrationIntercept: 0,
	}}
	repository := &documentMatchRepositoryFake{}
	service, err := NewPublishedDocumentMatchService(recall, profiles, repository, NewRankSignalDocumentMatchReranker(), fixedDocumentMatchClock{now: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.EvaluatePublished(context.Background(), EvaluatePublishedDocumentMatchesCommand{
		MonitorID: 7, MonitorVersionID: 11, CompiledProfileID: 13, RelevanceProfileID: 19,
	})
	if err != nil {
		t.Fatalf("EvaluatePublished(): %v", err)
	}
	if len(result.Decisions) != 2 || result.Decisions[0].Decision != "accepted" || result.Decisions[1].Decision != "rejected" ||
		result.Decisions[0].RelevanceProbability == nil || result.Decisions[1].RelevanceProbability == nil ||
		*result.Decisions[0].RelevanceProbability < .8 || *result.Decisions[1].RelevanceProbability >= .4 {
		t.Fatalf("calibrated decisions = %#v", result.Decisions)
	}
}

func TestPublishedDocumentMatchServiceFailsClosedForActiveProfileWithoutReranker(t *testing.T) {
	recall := &publishedMatchRecallFake{result: HybridRecallResult{
		MonitorID: 7, Purpose: "published", ConfigVersionID: 11, MonitorVersionID: 11,
		CompiledProfileID: 13, MatchingAlgorithmVersion: HybridRecallMatchingAlgorithmVersion,
		Candidates: []HybridRecallCandidateDTO{{DocumentVersionID: 17, Signals: []RecallSignalDTO{{Channel: "lexical", Rank: 1, RawScore: .5, AlgorithmVersion: LexicalRecallAlgorithmVersion}}}},
	}}
	profiles := &documentMatchProfileFake{profile: RelevanceDecisionProfileDTO{
		ID: 19, Version: 1, EvaluationRunID: 29, MatchingAlgorithmVersion: HybridRecallMatchingAlgorithmVersion,
		RerankerVersion: "cross-encoder-v1", CalibrationVersion: "temperature-2026-08",
		Status: "active", RejectThreshold: .4, AcceptThreshold: .8, CalibrationSlope: 1,
	}}
	repository := &documentMatchRepositoryFake{}
	service, err := NewPublishedDocumentMatchService(recall, profiles, repository, nil, fixedDocumentMatchClock{now: time.Now()})
	if err != nil {
		t.Fatalf("NewPublishedDocumentMatchService(): %v", err)
	}
	_, err = service.EvaluatePublished(context.Background(), EvaluatePublishedDocumentMatchesCommand{MonitorID: 7, MonitorVersionID: 11, CompiledProfileID: 13, RelevanceProfileID: 19})
	if !errors.Is(err, ErrDocumentMatchRerankerUnavailable) {
		t.Fatalf("EvaluatePublished() error = %v, want reranker unavailable", err)
	}
	if len(repository.commands) != 0 {
		t.Fatalf("active profile persisted without reranker: %#v", repository.commands)
	}
}

func TestPublishedDocumentMatchServiceRejectsPartialOrForeignActiveRerankerOutput(t *testing.T) {
	recall := &publishedMatchRecallFake{result: HybridRecallResult{
		MonitorID: 7, Purpose: "published", ConfigVersionID: 11, MonitorVersionID: 11,
		CompiledProfileID: 13, MatchingAlgorithmVersion: HybridRecallMatchingAlgorithmVersion,
		Candidates: []HybridRecallCandidateDTO{
			{DocumentVersionID: 17, Signals: []RecallSignalDTO{{Channel: "lexical", Rank: 1, RawScore: .5, AlgorithmVersion: LexicalRecallAlgorithmVersion}}},
			{DocumentVersionID: 18, Signals: []RecallSignalDTO{{Channel: "structured", Rank: 1, RawScore: 1, AlgorithmVersion: StructuredRecallAlgorithmVersion}}},
		},
	}}
	profiles := &documentMatchProfileFake{profile: RelevanceDecisionProfileDTO{
		ID: 19, Version: 1, EvaluationRunID: 29, MatchingAlgorithmVersion: HybridRecallMatchingAlgorithmVersion,
		RerankerVersion: "cross-encoder-v1", CalibrationVersion: "temperature-2026-08",
		Status: "active", RejectThreshold: .4, AcceptThreshold: .8, CalibrationSlope: 1,
	}}
	for _, fixture := range []struct {
		name   string
		values []RerankedDocumentMatchDTO
	}{
		{name: "partial", values: []RerankedDocumentMatchDTO{{DocumentVersionID: 17, RelevanceProbability: .9}}},
		{name: "foreign", values: []RerankedDocumentMatchDTO{
			{DocumentVersionID: 17, RelevanceProbability: .9},
			{DocumentVersionID: 99, RelevanceProbability: .2},
		}},
		{name: "invalid reason", values: []RerankedDocumentMatchDTO{
			{DocumentVersionID: 17, RelevanceProbability: .9, ReasonCodes: []string{"not valid"}},
			{DocumentVersionID: 18, RelevanceProbability: .2},
		}},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			repository := &documentMatchRepositoryFake{}
			service, err := NewPublishedDocumentMatchService(recall, profiles, repository,
				&documentMatchRerankerFake{values: fixture.values}, fixedDocumentMatchClock{now: time.Now()})
			if err != nil {
				t.Fatalf("NewPublishedDocumentMatchService(): %v", err)
			}
			_, err = service.EvaluatePublished(context.Background(), EvaluatePublishedDocumentMatchesCommand{
				MonitorID: 7, MonitorVersionID: 11, CompiledProfileID: 13, RelevanceProfileID: 19,
			})
			if !errors.Is(err, ErrInvalidDocumentMatchContract) {
				t.Fatalf("EvaluatePublished() error = %v, want invalid contract", err)
			}
			if len(repository.commands) != 0 {
				t.Fatalf("invalid reranker output persisted: %#v", repository.commands)
			}
		})
	}
}

func TestDocumentMatchOverrideAppendsNewFactAndReplaysIdempotently(t *testing.T) {
	repository := &documentMatchRepositoryFake{overrideResult: DocumentMatchOverrideDTO{
		ID: 31, MatchDecisionID: 23, Sequence: 1, MonitorID: 7, MonitorVersionID: 11, DocumentVersionID: 17,
		Decision: "accepted", PreviousEffectiveDecision: "review", ReasonCode: "manual_relevant",
		Note: "正文与监控目标一致", ActorUserID: 5, CreatedAt: time.Date(2026, 8, 10, 10, 0, 0, 0, time.UTC),
	}}
	authorizer := &documentMatchAuthorizerFake{}
	service, err := NewDocumentMatchReviewService(repository, authorizer, fixedDocumentMatchClock{now: time.Date(2026, 8, 10, 10, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("NewDocumentMatchReviewService(): %v", err)
	}
	result, err := service.Override(context.Background(), OverrideDocumentMatchCommand{
		ActorUserID: 5, MonitorID: 7, MatchDecisionID: 23, Decision: "accepted",
		ReasonCode: "manual_relevant", Note: "正文与监控目标一致", IdempotencyKey: "match-review-23-accepted",
	})
	if err != nil {
		t.Fatalf("Override(): %v", err)
	}
	if result.Override.ID != 31 || authorizer.calls != 1 || repository.overrideCommand.CommandFingerprint == "" {
		t.Fatalf("override result = %#v authorizer=%d command=%#v", result, authorizer.calls, repository.overrideCommand)
	}
}

func TestDocumentMatchOverrideRejectsControlCharactersInIdempotencyKey(t *testing.T) {
	repository := &documentMatchRepositoryFake{}
	authorizer := &documentMatchAuthorizerFake{}
	service, err := NewDocumentMatchReviewService(repository, authorizer, fixedDocumentMatchClock{now: time.Now()})
	if err != nil {
		t.Fatalf("NewDocumentMatchReviewService(): %v", err)
	}
	_, err = service.Override(context.Background(), OverrideDocumentMatchCommand{
		ActorUserID: 5, MonitorID: 7, MatchDecisionID: 23, Decision: "accepted",
		ReasonCode: "manual_relevant", IdempotencyKey: "match\treview",
	})
	if !errors.Is(err, ErrInvalidDocumentMatchContract) || authorizer.calls != 0 {
		t.Fatalf("Override() error=%v authorizer calls=%d, want pre-authorization rejection", err, authorizer.calls)
	}
}

func TestPublishedDocumentMatchServiceRejectsRepositoryReceiptDrift(t *testing.T) {
	recall := &publishedMatchRecallFake{result: HybridRecallResult{
		MonitorID: 7, Purpose: "published", ConfigVersionID: 11, MonitorVersionID: 11,
		CompiledProfileID: 13, MatchingAlgorithmVersion: HybridRecallMatchingAlgorithmVersion,
		Candidates: []HybridRecallCandidateDTO{{
			DocumentVersionID: 17, RRFScore: .02,
			Signals: []RecallSignalDTO{{Channel: "lexical", Rank: 1, RawScore: .5, AlgorithmVersion: LexicalRecallAlgorithmVersion}},
		}},
	}}
	profiles := &documentMatchProfileFake{profile: RelevanceDecisionProfileDTO{
		ID: 19, Version: 1, MatchingAlgorithmVersion: HybridRecallMatchingAlgorithmVersion,
		RerankerVersion: "cross-encoder-v1", CalibrationVersion: "uncalibrated-v1",
		Status: "uncalibrated", RejectThreshold: .4, AcceptThreshold: .8,
	}}
	repository := &documentMatchRepositoryFake{decisionMutator: func(value *DocumentMatchDecisionDTO) { value.Decision = "accepted" }}
	service, err := NewPublishedDocumentMatchService(recall, profiles, repository, nil, fixedDocumentMatchClock{now: time.Now()})
	if err != nil {
		t.Fatalf("NewPublishedDocumentMatchService(): %v", err)
	}
	_, err = service.EvaluatePublished(context.Background(), EvaluatePublishedDocumentMatchesCommand{
		MonitorID: 7, MonitorVersionID: 11, CompiledProfileID: 13, RelevanceProfileID: 19,
	})
	if !errors.Is(err, ErrInvalidDocumentMatchContract) {
		t.Fatalf("EvaluatePublished() error = %v, want invalid receipt", err)
	}
}

type publishedMatchRecallFake struct {
	result HybridRecallResult
	err    error
}

func (fake *publishedMatchRecallFake) Recall(context.Context, HybridRecallQuery) (HybridRecallResult, error) {
	return fake.result, fake.err
}

type documentMatchProfileFake struct {
	profile RelevanceDecisionProfileDTO
	err     error
}

type documentMatchRerankerFake struct {
	values []RerankedDocumentMatchDTO
	err    error
}

func (fake *documentMatchRerankerFake) RerankDocumentMatches(context.Context, RerankDocumentMatchesQuery) ([]RerankedDocumentMatchDTO, error) {
	return append([]RerankedDocumentMatchDTO(nil), fake.values...), fake.err
}

func (fake *documentMatchProfileFake) ReadRelevanceDecisionProfile(context.Context, ReadRelevanceDecisionProfileQuery) (RelevanceDecisionProfileDTO, error) {
	return fake.profile, fake.err
}

type documentMatchRepositoryFake struct {
	commands        []PersistAutomaticDocumentMatchCommand
	overrideCommand AppendDocumentMatchOverrideCommand
	overrideResult  DocumentMatchOverrideDTO
	decisionMutator func(*DocumentMatchDecisionDTO)
}

func (fake *documentMatchRepositoryFake) PersistAutomaticDocumentMatches(_ context.Context, commands []PersistAutomaticDocumentMatchCommand) ([]DocumentMatchDecisionDTO, error) {
	fake.commands = append(fake.commands, commands...)
	result := make([]DocumentMatchDecisionDTO, len(commands))
	for index, command := range commands {
		result[index] = documentMatchDecisionFromCommand(command, int64(index+1))
		if fake.decisionMutator != nil {
			fake.decisionMutator(&result[index])
		}
	}
	return result, nil
}

func (fake *documentMatchRepositoryFake) AppendDocumentMatchOverride(_ context.Context, command AppendDocumentMatchOverrideCommand) (DocumentMatchOverrideDTO, bool, error) {
	fake.overrideCommand = command
	return fake.overrideResult, false, nil
}

type documentMatchAuthorizerFake struct{ calls int }

func (fake *documentMatchAuthorizerFake) AuthorizeDocumentMatchReview(context.Context, AuthorizeDocumentMatchReviewQuery) error {
	fake.calls++
	return nil
}

type fixedDocumentMatchClock struct{ now time.Time }

func (clock fixedDocumentMatchClock) Now() time.Time { return clock.now }
