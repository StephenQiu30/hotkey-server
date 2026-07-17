package application

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	ingestiondomain "github.com/StephenQiu30/hotkey-server/internal/modules/ingestion/domain"
	intelligenceapplication "github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/application"
)

func TestPlan009ReviewOnlyGrayZoneAndReuse(t *testing.T) {
	repository := newReviewSnapshotFake()
	executor := &reviewExecutorFake{response: intelligenceapplication.RelevanceReviewResult{
		Status: "succeeded", RunID: 41, Decision: "accepted", Score: 80, ReasonCodes: []string{"relevant_evidence"},
	}}
	service, err := NewRelevanceReviewService(RelevanceReviewServiceDependencies{Snapshots: repository, Reviews: executor})
	if err != nil {
		t.Fatalf("NewRelevanceReviewService(): %v", err)
	}

	gray := plan009ReviewRequest(strings.Repeat("a", 64), ingestiondomain.MatchDecisionReview, 70)
	first, err := service.Review(context.Background(), gray)
	if err != nil {
		t.Fatalf("Review(gray): %v", err)
	}
	if first.Status != "succeeded" || executor.callsCount() != 1 || first.Snapshot.DecisionOrigin != ingestiondomain.DecisionOriginAI ||
		first.Snapshot.LLMScore == nil || *first.Snapshot.LLMScore != 80 || first.Snapshot.FinalScore != 80 || first.Snapshot.RuleScore != 70 ||
		first.Snapshot.ReviewAIRunID == nil || *first.Snapshot.ReviewAIRunID != 41 {
		t.Fatalf("Review(gray) = %#v calls=%d, want persisted AI review", first, executor.callsCount())
	}
	second, err := service.Review(context.Background(), gray)
	if err != nil || second.Snapshot.ID != first.Snapshot.ID || executor.callsCount() != 1 {
		t.Fatalf("Review(exact retry) = %#v / %v calls=%d, want no second owner", second, err, executor.callsCount())
	}

	hardReject := plan009ReviewRequest(strings.Repeat("b", 64), ingestiondomain.MatchDecisionRejected, 20)
	if _, err := service.Review(context.Background(), hardReject); err != nil {
		t.Fatalf("Review(hard reject): %v", err)
	}
	if executor.callsCount() != 1 {
		t.Fatalf("Review(hard reject) calls=%d, want 1", executor.callsCount())
	}

	locked := plan009ReviewRequest(strings.Repeat("c", 64), ingestiondomain.MatchDecisionReview, 70)
	repository.seedManualLocked(locked.Snapshot)
	if _, err := service.Review(context.Background(), locked); err != nil {
		t.Fatalf("Review(manual locked): %v", err)
	}
	if executor.callsCount() != 1 {
		t.Fatalf("Review(manual locked) calls=%d, want 1", executor.callsCount())
	}

	mismatchExecutor := &reviewExecutorFake{response: intelligenceapplication.RelevanceReviewResult{
		Status: "succeeded", RunID: 42, Decision: "review", Score: 80, ReasonCodes: []string{"ambiguous_context"},
	}}
	mismatchService, err := NewRelevanceReviewService(RelevanceReviewServiceDependencies{Snapshots: repository, Reviews: mismatchExecutor})
	if err != nil {
		t.Fatalf("NewRelevanceReviewService(mismatch): %v", err)
	}
	mismatch, err := mismatchService.Review(context.Background(), plan009ReviewRequest(strings.Repeat("d", 64), ingestiondomain.MatchDecisionReview, 70))
	if err != nil {
		t.Fatalf("Review(decision/score mismatch): %v", err)
	}
	if mismatch.Status != "degraded" || mismatch.Snapshot.Decision != ingestiondomain.MatchDecisionReview || mismatch.Snapshot.DecisionOrigin != ingestiondomain.DecisionOriginRule ||
		!mismatch.Snapshot.Degraded || !contains(mismatch.Snapshot.ReasonCodes, "ai_unavailable") {
		t.Fatalf("Review(decision/score mismatch) = %#v, want safe rule review", mismatch)
	}
}

func TestPlan009RelevanceReviewDegradesWithoutProvider(t *testing.T) {
	repository := newReviewSnapshotFake()
	executor := &reviewExecutorFake{response: intelligenceapplication.RelevanceReviewResult{Status: "degraded", ReasonCode: "ai_unavailable"}}
	service, err := NewRelevanceReviewService(RelevanceReviewServiceDependencies{Snapshots: repository, Reviews: executor})
	if err != nil {
		t.Fatalf("NewRelevanceReviewService(): %v", err)
	}

	result, err := service.Review(context.Background(), plan009ReviewRequest(strings.Repeat("e", 64), ingestiondomain.MatchDecisionReview, 70))
	if err != nil {
		t.Fatalf("Review(without provider): %v", err)
	}
	if result.Status != "degraded" || result.ReasonCode != "ai_unavailable" || result.Snapshot.Decision != ingestiondomain.MatchDecisionReview ||
		result.Snapshot.DecisionOrigin != ingestiondomain.DecisionOriginRule || result.Snapshot.LLMScore != nil || !result.Snapshot.Degraded ||
		!contains(result.Snapshot.ReasonCodes, "ai_unavailable") || repository.applyCount() != 0 {
		t.Fatalf("Review(without provider) = %#v applies=%d, want persisted degraded rule review", result, repository.applyCount())
	}
}

func plan009ReviewRequest(hash string, decision ingestiondomain.MatchDecision, score float64) RelevanceReviewRequest {
	semantic := 70.0
	return RelevanceReviewRequest{
		Snapshot: ingestiondomain.RelevanceSnapshotInput{
			MonitorID: 1, MonitorConfigVersionID: 2, ContentID: 3, InputHash: hash, ScoringVersion: "relevance-v1",
			RecallPaths: []string{"lexical", "vector"}, ReasonCodes: []string{"lexical_match"}, RuleScore: score, SemanticScore: &semantic,
			FinalScore: score, Decision: decision, DecisionOrigin: ingestiondomain.DecisionOriginRule,
			Explanation: plan009ReviewExplanation(), Degraded: false,
		},
		ContentExcerpt: "A verified OpenAI product announcement.", ContentLanguage: "en", MonitorIntent: "Track OpenAI product releases.",
		Factors:       RelevanceFactors{Semantic: &semantic, Lexical: 80, Entity: 60, Title: 70, Preference: 50},
		EvidenceTerms: []string{"OpenAI"}, HighThreshold: 75,
	}
}

func plan009ReviewExplanation() json.RawMessage {
	return json.RawMessage(`{"matched_terms":["openai"],"matched_entities":[],"excluded_terms":[],"recall_paths":["lexical","vector"],"scores":{"semantic":70,"lexical":80,"entity":60,"title":70,"preference":50},"reason_codes":["lexical_match"],"provenance":{"scoring_version":"relevance-v1"}}`)
}

type reviewExecutorFake struct {
	mu       sync.Mutex
	response intelligenceapplication.RelevanceReviewResult
	calls    int
}

func (fake *reviewExecutorFake) Review(_ context.Context, _ intelligenceapplication.RelevanceReviewRequest) (intelligenceapplication.RelevanceReviewResult, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.calls++
	return fake.response, nil
}

func (fake *reviewExecutorFake) callsCount() int {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	return fake.calls
}

type reviewSnapshotFake struct {
	mu      sync.Mutex
	byHash  map[string]ingestiondomain.RelevanceSnapshot
	applies int
}

func newReviewSnapshotFake() *reviewSnapshotFake {
	return &reviewSnapshotFake{byHash: make(map[string]ingestiondomain.RelevanceSnapshot)}
}

func (fake *reviewSnapshotFake) UpsertSnapshot(_ context.Context, input ingestiondomain.RelevanceSnapshotInput) (ingestiondomain.RelevanceSnapshot, bool, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if existing, ok := fake.byHash[input.InputHash]; ok {
		return cloneReviewSnapshot(existing), false, nil
	}
	snapshot := ingestiondomain.RelevanceSnapshot{RelevanceSnapshotInput: cloneReviewSnapshotInput(input), ID: int64(len(fake.byHash) + 1), Version: 1}
	fake.byHash[input.InputHash] = snapshot
	return cloneReviewSnapshot(snapshot), true, nil
}

func (fake *reviewSnapshotFake) ApplySuccessfulReview(_ context.Context, input ingestiondomain.SuccessfulReviewInput) (ingestiondomain.RelevanceSnapshot, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	for hash, snapshot := range fake.byHash {
		if snapshot.ID != input.SnapshotID || snapshot.Version != input.ExpectedVersion || snapshot.ManualLocked || snapshot.Decision != ingestiondomain.MatchDecisionReview || snapshot.DecisionOrigin != ingestiondomain.DecisionOriginRule {
			continue
		}
		llmScore, runID := input.LLMScore, input.ReviewAIRunID
		snapshot.LLMScore, snapshot.FinalScore, snapshot.Decision, snapshot.DecisionOrigin, snapshot.ReviewAIRunID = &llmScore, input.FinalScore, input.Decision, ingestiondomain.DecisionOriginAI, &runID
		snapshot.ReasonCodes, snapshot.Version = append([]string(nil), input.ReasonCodes...), snapshot.Version+1
		fake.byHash[hash], fake.applies = snapshot, fake.applies+1
		return cloneReviewSnapshot(snapshot), nil
	}
	return ingestiondomain.RelevanceSnapshot{}, fmt.Errorf("snapshot cannot accept review")
}

func (fake *reviewSnapshotFake) MarkReviewUnavailable(_ context.Context, snapshotID, expectedVersion int64, reasonCode string) (ingestiondomain.RelevanceSnapshot, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	for hash, snapshot := range fake.byHash {
		if snapshot.ID != snapshotID || snapshot.Version != expectedVersion || snapshot.ManualLocked || snapshot.Decision != ingestiondomain.MatchDecisionReview || snapshot.DecisionOrigin != ingestiondomain.DecisionOriginRule {
			continue
		}
		snapshot.Degraded, snapshot.ReasonCodes, snapshot.Version = true, uniqueStrings(append(snapshot.ReasonCodes, reasonCode)), snapshot.Version+1
		fake.byHash[hash] = snapshot
		return cloneReviewSnapshot(snapshot), nil
	}
	return ingestiondomain.RelevanceSnapshot{}, fmt.Errorf("snapshot cannot degrade")
}

func (fake *reviewSnapshotFake) seedManualLocked(input ingestiondomain.RelevanceSnapshotInput) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.byHash[input.InputHash] = ingestiondomain.RelevanceSnapshot{RelevanceSnapshotInput: cloneReviewSnapshotInput(input), ID: int64(len(fake.byHash) + 1), Version: 1, ManualLocked: true}
}

func (fake *reviewSnapshotFake) applyCount() int {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	return fake.applies
}

func cloneReviewSnapshot(snapshot ingestiondomain.RelevanceSnapshot) ingestiondomain.RelevanceSnapshot {
	snapshot.RelevanceSnapshotInput = cloneReviewSnapshotInput(snapshot.RelevanceSnapshotInput)
	if snapshot.ReviewAIRunID != nil {
		value := *snapshot.ReviewAIRunID
		snapshot.ReviewAIRunID = &value
	}
	return snapshot
}

func cloneReviewSnapshotInput(input ingestiondomain.RelevanceSnapshotInput) ingestiondomain.RelevanceSnapshotInput {
	input.RecallPaths, input.ReasonCodes = append([]string(nil), input.RecallPaths...), append([]string(nil), input.ReasonCodes...)
	input.Explanation = append(json.RawMessage(nil), input.Explanation...)
	if input.SemanticScore != nil {
		value := *input.SemanticScore
		input.SemanticScore = &value
	}
	if input.LLMScore != nil {
		value := *input.LLMScore
		input.LLMScore = &value
	}
	return input
}
