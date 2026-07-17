package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	ingestiondomain "github.com/StephenQiu30/hotkey-server/internal/modules/ingestion/domain"
	ingestionpostgres "github.com/StephenQiu30/hotkey-server/internal/modules/ingestion/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
)

func TestPlan009RelevanceSnapshotRepository(t *testing.T) {
	runtime, fixture := openRelevanceRuntime(t)
	defer func() { _ = runtime.Close() }()
	repository := ingestionpostgres.NewRelevanceRepository(runtime)

	profileID, profileVersion, profileModelVersion := createEmbeddingProfile(t, runtime)
	first := relevanceSnapshotInput(fixture, strings.Repeat("a", 64))
	first.EmbeddingModelProfileID = &profileID
	first.EmbeddingModelProfileVersion = &profileVersion
	first.EmbeddingModelVersion = &profileModelVersion
	stored, created, err := repository.UpsertSnapshot(context.Background(), first)
	if err != nil || !created {
		t.Fatalf("UpsertSnapshot(first) snapshot/created/error = %#v / %t / %v", stored, created, err)
	}
	retried, created, err := repository.UpsertSnapshot(context.Background(), first)
	if err != nil || created || retried.ID != stored.ID {
		t.Fatalf("UpsertSnapshot(exact retry) snapshot/created/error = %#v / %t / %v", retried, created, err)
	}
	wrongInputRunID := createSuccessfulRelevanceReviewRun(t, runtime, stored.ID, strings.Repeat("f", 64))
	if _, err := repository.ApplySuccessfulReview(context.Background(), ingestiondomain.SuccessfulReviewInput{
		SnapshotID: stored.ID, ExpectedVersion: stored.Version, ReviewAIRunID: wrongInputRunID,
		LLMScore: 72, FinalScore: 72, Decision: ingestiondomain.MatchDecisionReview, ReasonCodes: []string{"ambiguous_context"},
	}); !errors.Is(err, sharedrepository.ErrInvalidInput) {
		t.Fatalf("ApplySuccessfulReview(wrong run input) error = %v, want invalid input", err)
	}
	reviewRunID := createSuccessfulRelevanceReviewRun(t, runtime, stored.ID, stored.InputHash)
	if _, err := repository.ApplySuccessfulReview(context.Background(), ingestiondomain.SuccessfulReviewInput{
		SnapshotID: stored.ID, ExpectedVersion: stored.Version, ReviewAIRunID: reviewRunID,
		LLMScore: 71, FinalScore: 71, Decision: ingestiondomain.MatchDecisionReview, ReasonCodes: []string{"ambiguous_context"},
	}); !errors.Is(err, sharedrepository.ErrInvalidInput) {
		t.Fatalf("ApplySuccessfulReview(mismatched structured result) error = %v, want invalid input", err)
	}
	if _, err := repository.ApplySuccessfulReview(context.Background(), ingestiondomain.SuccessfulReviewInput{
		SnapshotID: stored.ID, ExpectedVersion: stored.Version, ReviewAIRunID: reviewRunID,
		LLMScore: 72, FinalScore: 73, Decision: ingestiondomain.MatchDecisionReview, ReasonCodes: []string{"ambiguous_context"},
	}); !errors.Is(err, sharedrepository.ErrInvalidInput) {
		t.Fatalf("ApplySuccessfulReview(divergent scores) error = %v, want invalid input", err)
	}
	reviewed, err := repository.ApplySuccessfulReview(context.Background(), ingestiondomain.SuccessfulReviewInput{
		SnapshotID: stored.ID, ExpectedVersion: stored.Version, ReviewAIRunID: reviewRunID,
		LLMScore: 72, FinalScore: 72, Decision: ingestiondomain.MatchDecisionReview, ReasonCodes: []string{"ambiguous_context"},
	})
	if err != nil || reviewed.Version != 2 || reviewed.RuleScore != 70 || reviewed.LLMScore == nil || *reviewed.LLMScore != 72 ||
		reviewed.DecisionOrigin != ingestiondomain.DecisionOriginAI || reviewed.ReviewAIRunID == nil || *reviewed.ReviewAIRunID != reviewRunID {
		t.Fatalf("ApplySuccessfulReview() snapshot/error = %#v / %v", reviewed, err)
	}
	if _, err := repository.ApplySuccessfulReview(context.Background(), ingestiondomain.SuccessfulReviewInput{
		SnapshotID: stored.ID, ExpectedVersion: stored.Version, ReviewAIRunID: reviewRunID,
		LLMScore: 72, FinalScore: 72, Decision: ingestiondomain.MatchDecisionReview, ReasonCodes: []string{"ambiguous_context"},
	}); !errors.Is(err, sharedrepository.ErrConflict) {
		t.Fatalf("ApplySuccessfulReview(stale snapshot) error = %v, want conflict", err)
	}
	retriedAfterReview, created, err := repository.UpsertSnapshot(context.Background(), first)
	if err != nil || created || retriedAfterReview.ID != stored.ID || retriedAfterReview.DecisionOrigin != ingestiondomain.DecisionOriginAI {
		t.Fatalf("UpsertSnapshot(exact retry after review) snapshot/created/error = %#v / %t / %v", retriedAfterReview, created, err)
	}

	updated := first
	updated.InputHash = strings.Repeat("b", 64)
	updated.RuleScore, updated.FinalScore = 82, 82
	updated.Explanation = relevanceExplanation(82)
	later, created, err := repository.UpsertSnapshot(context.Background(), updated)
	if err != nil || !created || later.ID == stored.ID {
		t.Fatalf("UpsertSnapshot(changed input) snapshot/created/error = %#v / %t / %v", later, created, err)
	}

	secondContent := createRelevanceContent(t, runtime, fixture.sourceID, "second")
	second := relevanceSnapshotInput(fixture, strings.Repeat("c", 64))
	second.ContentID, second.RuleScore, second.FinalScore, second.Explanation = secondContent, 75, 75, relevanceExplanation(75)
	if _, created, err := repository.UpsertSnapshot(context.Background(), second); err != nil || !created {
		t.Fatalf("UpsertSnapshot(second content) created/error = %t / %v", created, err)
	}

	page, err := repository.ListLatestSnapshots(context.Background(), fixture.monitorID, ingestiondomain.RelevanceSnapshotListQuery{Limit: 1})
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != later.ID || page.Next == nil {
		t.Fatalf("ListLatestSnapshots(first) page/error = %#v / %v", page, err)
	}
	next, err := repository.ListLatestSnapshots(context.Background(), fixture.monitorID, ingestiondomain.RelevanceSnapshotListQuery{Limit: 1, Cursor: page.Next})
	if err != nil || len(next.Items) != 1 || next.Items[0].ContentID != secondContent || next.Next != nil {
		t.Fatalf("ListLatestSnapshots(second) page/error = %#v / %v", next, err)
	}

	contentRepository := ingestionpostgres.NewContentRepository(runtime)
	if _, changed, err := contentRepository.MarkDeleted(context.Background(), fixture.sourceID, fixture.contentExternalID); err != nil || !changed {
		t.Fatalf("MarkDeleted(relevance content) changed/error = %t / %v", changed, err)
	}
	visible, err := repository.ListLatestSnapshots(context.Background(), fixture.monitorID, ingestiondomain.RelevanceSnapshotListQuery{Limit: 10})
	if err != nil || len(visible.Items) != 1 || visible.Items[0].ContentID != secondContent {
		t.Fatalf("ListLatestSnapshots(deleted content) page/error = %#v / %v", visible, err)
	}

	unsafe := second
	unsafe.InputHash = strings.Repeat("d", 64)
	unsafe.Explanation = []byte(`{"raw_response":"forbidden"}`)
	if _, _, err := repository.UpsertSnapshot(context.Background(), unsafe); !errors.Is(err, sharedrepository.ErrInvalidInput) {
		t.Fatalf("UpsertSnapshot(unsafe explanation) error = %v, want invalid input", err)
	}
	wrongVersion := second
	wrongVersion.InputHash = strings.Repeat("e", 64)
	badVersion := profileVersion + 1
	wrongVersion.EmbeddingModelProfileID = &profileID
	wrongVersion.EmbeddingModelProfileVersion = &badVersion
	wrongVersion.EmbeddingModelVersion = &profileModelVersion
	if _, _, err := repository.UpsertSnapshot(context.Background(), wrongVersion); !errors.Is(err, sharedrepository.ErrInvalidInput) {
		t.Fatalf("UpsertSnapshot(stale embedding provenance) error = %v, want invalid input", err)
	}
}

func TestPlan009FeedbackRepositoryUsesOwnVersion(t *testing.T) {
	runtime, fixture := openRelevanceRuntime(t)
	defer func() { _ = runtime.Close() }()
	repository := ingestionpostgres.NewRelevanceRepository(runtime)
	snapshot, _, err := repository.UpsertSnapshot(context.Background(), relevanceSnapshotInput(fixture, strings.Repeat("a", 64)))
	if err != nil {
		t.Fatalf("UpsertSnapshot(): %v", err)
	}

	feedback := ingestiondomain.RelevanceFeedbackInput{
		MonitorID: fixture.monitorID, MonitorConfigVersionID: fixture.configID, ContentID: fixture.contentID,
		MonitorMatchID: &snapshot.ID, ActorUserID: fixture.actorID, FeedbackType: ingestiondomain.FeedbackTypeRelevant,
	}
	created, err := repository.UpsertFeedback(context.Background(), feedback)
	if err != nil || created.Version != 1 || created.FeedbackType != ingestiondomain.FeedbackTypeRelevant {
		t.Fatalf("UpsertFeedback(create) feedback/error = %#v / %v", created, err)
	}
	feedback.ExpectedVersion = &created.Version
	feedback.FeedbackType = ingestiondomain.FeedbackTypeIrrelevant
	updated, err := repository.UpsertFeedback(context.Background(), feedback)
	if err != nil || updated.Version != 2 || updated.FeedbackType != ingestiondomain.FeedbackTypeIrrelevant {
		t.Fatalf("UpsertFeedback(update) feedback/error = %#v / %v", updated, err)
	}
	feedback.ExpectedVersion = nil
	if _, err := repository.UpsertFeedback(context.Background(), feedback); !errors.Is(err, sharedrepository.ErrConflict) {
		t.Fatalf("UpsertFeedback(existing with nil expected version) error = %v, want conflict", err)
	}
	staleVersion := int64(1)
	feedback.ExpectedVersion = &staleVersion
	if _, err := repository.UpsertFeedback(context.Background(), feedback); !errors.Is(err, sharedrepository.ErrConflict) {
		t.Fatalf("UpsertFeedback(stale version) error = %v, want conflict", err)
	}

	wrongConfig := createRelevanceMonitorConfig(t, runtime, "wrong-config")
	mismatch := feedback
	mismatch.ExpectedVersion = nil
	mismatch.MonitorConfigVersionID = wrongConfig.configID
	mismatch.MonitorID = wrongConfig.monitorID
	if _, err := repository.UpsertFeedback(context.Background(), mismatch); !errors.Is(err, sharedrepository.ErrNotFound) {
		t.Fatalf("UpsertFeedback(mismatched match) error = %v, want not found", err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE users SET status = 'disabled' WHERE id = $1`, fixture.actorID); err != nil {
		t.Fatalf("disable feedback actor: %v", err)
	}
	disabled := feedback
	disabled.ActorUserID = fixture.actorID
	disabled.ExpectedVersion = &updated.Version
	if _, err := repository.UpsertFeedback(context.Background(), disabled); !errors.Is(err, sharedrepository.ErrNotFound) {
		t.Fatalf("UpsertFeedback(disabled actor) error = %v, want not found", err)
	}

	suggestionInput := ingestiondomain.RelevanceSuggestionInput{
		MonitorID: fixture.monitorID, MonitorConfigVersionID: fixture.configID,
		SuggestionType: ingestiondomain.SuggestionTypeAddTerm, Value: "OpenAI", SupportCount: 2,
	}
	suggestion, createdSuggestion, err := repository.UpsertPendingSuggestion(context.Background(), suggestionInput)
	if err != nil || !createdSuggestion || suggestion.Status != ingestiondomain.SuggestionStatusPending {
		t.Fatalf("UpsertPendingSuggestion(create) suggestion/created/error = %#v / %t / %v", suggestion, createdSuggestion, err)
	}
	suggestionInput.SupportCount = 3
	updatedSuggestion, createdSuggestion, err := repository.UpsertPendingSuggestion(context.Background(), suggestionInput)
	if err != nil || createdSuggestion || updatedSuggestion.ID != suggestion.ID || updatedSuggestion.Version != 2 || updatedSuggestion.SupportCount != 3 {
		t.Fatalf("UpsertPendingSuggestion(update) suggestion/created/error = %#v / %t / %v", updatedSuggestion, createdSuggestion, err)
	}
	reviewed, err := repository.ReviewSuggestion(context.Background(), fixture.monitorID, suggestion.ID, fixture.reviewerID, updatedSuggestion.Version, ingestiondomain.SuggestionStatusApproved)
	if err != nil || reviewed.Version != 3 || reviewed.Status != ingestiondomain.SuggestionStatusApproved {
		t.Fatalf("ReviewSuggestion() suggestion/error = %#v / %v", reviewed, err)
	}
	var rules int
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM monitor_rules WHERE config_version_id = $1`, fixture.configID).Scan(&rules); err != nil {
		t.Fatalf("count monitor rules after suggestion review: %v", err)
	}
	if rules != 0 {
		t.Fatalf("monitor rules after suggestion review = %d, want 0", rules)
	}
	suggestionInput.SupportCount = 1
	if _, _, err := repository.UpsertPendingSuggestion(context.Background(), suggestionInput); !errors.Is(err, sharedrepository.ErrInvalidInput) {
		t.Fatalf("UpsertPendingSuggestion(insufficient support) error = %v, want invalid input", err)
	}
}

func TestPlan009FalseNegativeFeedbackRequiresNoSnapshot(t *testing.T) {
	runtime, fixture := openRelevanceRuntime(t)
	defer func() { _ = runtime.Close() }()
	repository := ingestionpostgres.NewRelevanceRepository(runtime)

	input := ingestiondomain.RelevanceFeedbackInput{
		MonitorID: fixture.monitorID, MonitorConfigVersionID: fixture.configID, ContentID: fixture.contentID,
		ActorUserID: fixture.actorID, FeedbackType: ingestiondomain.FeedbackTypeFalseNegative,
	}
	created, err := repository.UpsertFalseNegativeFeedback(context.Background(), input)
	if err != nil || created.MonitorMatchID != nil || created.FeedbackType != ingestiondomain.FeedbackTypeFalseNegative {
		t.Fatalf("UpsertFalseNegativeFeedback(unmatched) feedback/error = %#v / %v", created, err)
	}

	for _, feedbackType := range []ingestiondomain.FeedbackType{
		ingestiondomain.FeedbackTypeRelevant,
		ingestiondomain.FeedbackTypeIrrelevant,
		ingestiondomain.FeedbackTypeFalsePositive,
	} {
		invalid := input
		invalid.ContentID = createRelevanceContent(t, runtime, fixture.sourceID, "invalid-"+string(feedbackType))
		invalid.FeedbackType = feedbackType
		if _, err := repository.UpsertFalseNegativeFeedback(context.Background(), invalid); !errors.Is(err, sharedrepository.ErrInvalidInput) {
			t.Fatalf("UpsertFalseNegativeFeedback(%s) error = %v, want invalid input", feedbackType, err)
		}
	}

	matchedContentID := createRelevanceContent(t, runtime, fixture.sourceID, "already-matched")
	matched := relevanceSnapshotInput(fixture, strings.Repeat("f", 64))
	matched.ContentID = matchedContentID
	if _, _, err := repository.UpsertSnapshot(context.Background(), matched); err != nil {
		t.Fatalf("create matched snapshot: %v", err)
	}
	input.ContentID = matchedContentID
	if _, err := repository.UpsertFalseNegativeFeedback(context.Background(), input); !errors.Is(err, sharedrepository.ErrConflict) {
		t.Fatalf("UpsertFalseNegativeFeedback(existing snapshot) error = %v, want conflict", err)
	}
	var count int
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM monitor_match_feedbacks WHERE monitor_id = $1 AND content_id = $2 AND monitor_match_id IS NULL`, fixture.monitorID, matchedContentID).Scan(&count); err != nil {
		t.Fatalf("count false-negative feedback after matched rejection: %v", err)
	}
	if count != 0 {
		t.Fatalf("false-negative feedback after snapshot exists = %d, want 0", count)
	}

	concurrentContentID := createRelevanceContent(t, runtime, fixture.sourceID, "concurrent-snapshot")
	lock, err := runtime.SQL.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatalf("begin concurrent content lock: %v", err)
	}
	defer func() { _ = lock.Rollback() }()
	if _, err := lock.ExecContext(context.Background(), `SELECT id FROM contents WHERE id = $1 FOR UPDATE`, concurrentContentID); err != nil {
		t.Fatalf("lock concurrent content: %v", err)
	}
	snapshotInput := relevanceSnapshotInput(fixture, strings.Repeat("e", 64))
	snapshotInput.ContentID = concurrentContentID
	snapshotDone := make(chan error, 1)
	go func() {
		_, _, err := repository.UpsertSnapshot(context.Background(), snapshotInput)
		snapshotDone <- err
	}()
	waitForRelevanceContentLock(t, runtime, concurrentContentID)
	feedbackDone := make(chan error, 1)
	go func() {
		_, err := repository.UpsertFalseNegativeFeedback(context.Background(), ingestiondomain.RelevanceFeedbackInput{
			MonitorID: fixture.monitorID, MonitorConfigVersionID: fixture.configID, ContentID: concurrentContentID,
			ActorUserID: fixture.actorID, FeedbackType: ingestiondomain.FeedbackTypeFalseNegative,
		})
		feedbackDone <- err
	}()
	if err := lock.Commit(); err != nil {
		t.Fatalf("release concurrent content lock: %v", err)
	}
	if err := <-snapshotDone; err != nil {
		t.Fatalf("concurrent UpsertSnapshot(): %v", err)
	}
	if err := <-feedbackDone; !errors.Is(err, sharedrepository.ErrConflict) {
		t.Fatalf("concurrent UpsertFalseNegativeFeedback() error = %v, want conflict", err)
	}
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM monitor_match_feedbacks WHERE monitor_id = $1 AND content_id = $2 AND monitor_match_id IS NULL`, fixture.monitorID, concurrentContentID).Scan(&count); err != nil {
		t.Fatalf("count false-negative feedback after concurrent snapshot: %v", err)
	}
	if count != 0 {
		t.Fatalf("false-negative feedback after concurrent snapshot = %d, want 0", count)
	}
}

func waitForRelevanceContentLock(t *testing.T, runtime *database.Runtime, contentID int64) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var waiting int
		err := runtime.SQL.QueryRow(`
SELECT count(*)
FROM pg_stat_activity
WHERE datname = current_database()
  AND state = 'active'
  AND wait_event_type = 'Lock'
  AND query LIKE '%SELECT id FROM contents%'`).Scan(&waiting)
		if err != nil {
			t.Fatalf("inspect concurrent relevance lock: %v", err)
		}
		if waiting > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("snapshot creation did not wait for content %d lock", contentID)
}

func TestPlan009FeedbackRefreshAndEvaluationStayMonitorLocal(t *testing.T) {
	runtime, fixture := openRelevanceRuntime(t)
	defer func() { _ = runtime.Close() }()
	repository := ingestionpostgres.NewRelevanceRepository(runtime)

	first, _, err := repository.UpsertSnapshot(context.Background(), relevanceSnapshotInput(fixture, strings.Repeat("1", 64)))
	if err != nil {
		t.Fatalf("create first snapshot: %v", err)
	}
	secondContentID := createRelevanceContent(t, runtime, fixture.sourceID, "refresh-second")
	secondInput := relevanceSnapshotInput(fixture, strings.Repeat("2", 64))
	secondInput.ContentID = secondContentID
	secondInput.RuleScore, secondInput.FinalScore, secondInput.SemanticScore = 80, 80, float64Pointer(80)
	secondInput.Explanation = relevanceExplanation(80)
	second, _, err := repository.UpsertSnapshot(context.Background(), secondInput)
	if err != nil {
		t.Fatalf("create second snapshot: %v", err)
	}
	for _, snapshot := range []ingestiondomain.RelevanceSnapshot{first, second} {
		if _, err := repository.UpsertFeedback(context.Background(), ingestiondomain.RelevanceFeedbackInput{
			MonitorID: fixture.monitorID, MonitorConfigVersionID: fixture.configID, ContentID: snapshot.ContentID,
			MonitorMatchID: &snapshot.ID, ActorUserID: fixture.actorID, FeedbackType: ingestiondomain.FeedbackTypeRelevant,
		}); err != nil {
			t.Fatalf("create feedback for snapshot %d: %v", snapshot.ID, err)
		}
	}

	refreshed, err := repository.RefreshSuggestions(context.Background(), fixture.monitorID)
	if err != nil || refreshed != 1 {
		t.Fatalf("RefreshSuggestions() count/error = %d / %v, want 1/nil", refreshed, err)
	}
	suggestions, err := repository.ListSuggestions(context.Background(), fixture.monitorID, ingestiondomain.RelevanceSuggestionListQuery{Limit: 10})
	if err != nil || len(suggestions.Items) != 1 || suggestions.Items[0].SuggestionType != ingestiondomain.SuggestionTypeAddTerm || suggestions.Items[0].Value != "OpenAI" || suggestions.Items[0].SupportCount != 2 {
		t.Fatalf("ListSuggestions() page/error = %#v / %v", suggestions, err)
	}
	evaluations, err := repository.FeedbackEvaluations(context.Background(), fixture.monitorID)
	if err != nil || len(evaluations) != 1 || evaluations[0].ScoringVersion != "relevance-v1" || evaluations[0].PrecisionAt20 != 100 || evaluations[0].ExclusionFalsePositiveRate != 0 || evaluations[0].EvaluatedCount != 2 {
		t.Fatalf("FeedbackEvaluations() values/error = %#v / %v", evaluations, err)
	}
	var ruleCount int
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM monitor_rules WHERE config_version_id = $1`, fixture.configID).Scan(&ruleCount); err != nil {
		t.Fatalf("count rules after refresh: %v", err)
	}
	if ruleCount != 0 {
		t.Fatalf("RefreshSuggestions() wrote monitor rules = %d", ruleCount)
	}
}

func TestPlan009RelevanceReviewUnavailablePersistence(t *testing.T) {
	runtime, fixture := openRelevanceRuntime(t)
	defer func() { _ = runtime.Close() }()
	repository := ingestionpostgres.NewRelevanceRepository(runtime)

	pending, created, err := repository.UpsertSnapshot(context.Background(), relevanceSnapshotInput(fixture, strings.Repeat("9", 64)))
	if err != nil || !created || pending.ManualLocked {
		t.Fatalf("UpsertSnapshot(pending review) = %#v / %t / %v", pending, created, err)
	}
	unavailable, err := repository.MarkReviewUnavailable(context.Background(), pending.ID, pending.Version, "ai_unavailable")
	if err != nil || unavailable.Version != pending.Version+1 || !unavailable.Degraded || unavailable.Decision != ingestiondomain.MatchDecisionReview ||
		unavailable.DecisionOrigin != ingestiondomain.DecisionOriginRule || !relevanceReasonPresent(unavailable.ReasonCodes, "ai_unavailable") {
		t.Fatalf("MarkReviewUnavailable() = %#v / %v", unavailable, err)
	}
	if _, err := repository.MarkReviewUnavailable(context.Background(), pending.ID, pending.Version, "ai_unavailable"); !errors.Is(err, sharedrepository.ErrConflict) {
		t.Fatalf("MarkReviewUnavailable(stale) error = %v, want conflict", err)
	}

	runID := createSuccessfulRelevanceReviewRun(t, runtime, unavailable.ID, unavailable.InputHash)
	reviewed, err := repository.ApplySuccessfulReview(context.Background(), ingestiondomain.SuccessfulReviewInput{
		SnapshotID: unavailable.ID, ExpectedVersion: unavailable.Version, ReviewAIRunID: runID,
		LLMScore: 72, FinalScore: 72, Decision: ingestiondomain.MatchDecisionReview, ReasonCodes: []string{"ambiguous_context"},
	})
	if err != nil || reviewed.Version != unavailable.Version+1 || reviewed.DecisionOrigin != ingestiondomain.DecisionOriginAI ||
		reviewed.ReviewAIRunID == nil || *reviewed.ReviewAIRunID != runID {
		t.Fatalf("ApplySuccessfulReview(after unavailable) = %#v / %v", reviewed, err)
	}
}

type relevanceFixture struct {
	sourceID, monitorID, configID, contentID, actorID, reviewerID int64
	contentExternalID                                             string
}

func openRelevanceRuntime(t *testing.T) (*database.Runtime, relevanceFixture) {
	t.Helper()
	runtime := openContentRuntime(t)
	sourceID := createContentSource(t, runtime, "relevance")
	contentExternalID := "relevance-content"
	contentID := createRelevanceContent(t, runtime, sourceID, contentExternalID)
	monitor := createRelevanceMonitorConfig(t, runtime, "primary")
	actorID := createRelevanceUser(t, runtime, "actor", "editor")
	reviewerID := createRelevanceUser(t, runtime, "reviewer", "admin")
	return runtime, relevanceFixture{sourceID: sourceID, monitorID: monitor.monitorID, configID: monitor.configID, contentID: contentID, actorID: actorID, reviewerID: reviewerID, contentExternalID: contentExternalID}
}

func createRelevanceContent(t *testing.T, runtime *database.Runtime, sourceID int64, externalID string) int64 {
	t.Helper()
	content, _, err := ingestionpostgres.NewContentRepository(runtime).Upsert(context.Background(), normalizedContent(sourceID, externalID, time.Date(2026, time.July, 17, 9, 0, 0, 0, time.UTC)), activeDecision())
	if err != nil {
		t.Fatalf("create relevance content %q: %v", externalID, err)
	}
	return content.ID
}

type relevanceMonitorConfig struct{ monitorID, configID int64 }

func createRelevanceMonitorConfig(t *testing.T, runtime *database.Runtime, suffix string) relevanceMonitorConfig {
	t.Helper()
	var monitorID, configID int64
	if err := runtime.SQL.QueryRow(`
INSERT INTO monitors (name, status) VALUES ($1, 'draft') RETURNING id`, "relevance-"+suffix+fmt.Sprintf("-%d", time.Now().UnixNano())).Scan(&monitorID); err != nil {
		t.Fatalf("create relevance monitor: %v", err)
	}
	if err := runtime.SQL.QueryRow(`
INSERT INTO monitor_config_versions (monitor_id, revision)
VALUES ($1, 1) RETURNING id`, monitorID).Scan(&configID); err != nil {
		t.Fatalf("create draft relevance monitor config: %v", err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE monitors SET draft_config_version_id = $1 WHERE id = $2`, configID, monitorID); err != nil {
		t.Fatalf("point relevance monitor at draft config: %v", err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE monitor_config_versions SET state = 'published', config_hash = $1, published_at = now() WHERE id = $2`, strings.Repeat("f", 64), configID); err != nil {
		t.Fatalf("publish relevance monitor config: %v", err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE monitors SET status = 'active', draft_config_version_id = NULL, published_config_version_id = $1 WHERE id = $2`, configID, monitorID); err != nil {
		t.Fatalf("point relevance monitor at published config: %v", err)
	}
	return relevanceMonitorConfig{monitorID: monitorID, configID: configID}
}

func createRelevanceUser(t *testing.T, runtime *database.Runtime, suffix, role string) int64 {
	t.Helper()
	var userID int64
	email := fmt.Sprintf("relevance-%s-%d@example.test", suffix, time.Now().UnixNano())
	if err := runtime.SQL.QueryRow(`
INSERT INTO users (email, password_hash, display_name, role)
VALUES ($1, 'hashed-password', $2, $3) RETURNING id`, email, suffix, role).Scan(&userID); err != nil {
		t.Fatalf("create relevance user: %v", err)
	}
	return userID
}

func createEmbeddingProfile(t *testing.T, runtime *database.Runtime) (int64, int64, string) {
	t.Helper()
	var id, version int64
	const modelVersion = "embedding-v1"
	if err := runtime.SQL.QueryRow(`
INSERT INTO ai_model_profiles (
  name, task_type, provider, model_name, credential_ref, model_version,
  embedding_dimensions, timeout_seconds, max_attempts, max_cost, fallback_priority, enabled
) VALUES ($1, 'embedding', 'openai', 'text-embedding-3-large', 'env:OPENAI_API_KEY', $2, 1024, 30, 1, 0.1000, 100, true)
RETURNING id, version`, fmt.Sprintf("relevance-embedding-%d", time.Now().UnixNano()), modelVersion).Scan(&id, &version); err != nil {
		t.Fatalf("create embedding profile: %v", err)
	}
	return id, version, modelVersion
}

func createSuccessfulRelevanceReviewRun(t *testing.T, runtime *database.Runtime, matchID int64, inputHash string) int64 {
	t.Helper()
	const modelVersion = "gpt-5.6sol-2026-07"
	var profileID, profileVersion, runID int64
	if err := runtime.SQL.QueryRow(`
INSERT INTO ai_model_profiles (
  name, task_type, provider, model_name, credential_ref, model_version,
  timeout_seconds, max_attempts, max_cost, fallback_priority, enabled
) VALUES ($1, 'relevance_review', 'openai', 'gpt-5.6sol', 'env:OPENAI_API_KEY', $2, 30, 1, 0.1000, 100, true)
RETURNING id, version`, fmt.Sprintf("relevance-review-%d", time.Now().UnixNano()), modelVersion).Scan(&profileID, &profileVersion); err != nil {
		t.Fatalf("create relevance review profile: %v", err)
	}
	if err := runtime.SQL.QueryRow(`
INSERT INTO ai_runs (
  task_type, target_type, target_id, model_profile_id, prompt_version, schema_version,
  input_hash, structured_result, status, model_profile_version, model_version, parameters_version,
  input_schema_version, evidence_set_hash, reuse_key, attempt, max_attempts, budget_day
) VALUES (
  'relevance_review', 'monitor_match', $1, $2, 'relevance-review-v1', 'v1',
  $3, '{"decision":"review","score":72,"reason_codes":["ambiguous_context"]}'::jsonb,
  'succeeded', $4, $5, 'relevance-v1', 'v1', $6, $7, 1, 1, current_date
) RETURNING id`,
		matchID, profileID, inputHash, profileVersion, modelVersion, strings.Repeat("d", 64), inputHash,
	).Scan(&runID); err != nil {
		t.Fatalf("create succeeded relevance review run: %v", err)
	}
	return runID
}

func relevanceSnapshotInput(fixture relevanceFixture, inputHash string) ingestiondomain.RelevanceSnapshotInput {
	return ingestiondomain.RelevanceSnapshotInput{
		MonitorID: fixture.monitorID, MonitorConfigVersionID: fixture.configID, ContentID: fixture.contentID,
		InputHash: inputHash, ScoringVersion: "relevance-v1", RecallPaths: []string{"lexical", "vector"},
		RuleScore: 70, SemanticScore: float64Pointer(70), FinalScore: 70,
		Decision: ingestiondomain.MatchDecisionReview, DecisionOrigin: ingestiondomain.DecisionOriginRule,
		ReasonCodes: []string{"lexical_candidate"}, Explanation: relevanceExplanation(70),
	}
}

func relevanceExplanation(score float64) []byte {
	return []byte(fmt.Sprintf(`{"matched_terms":["OpenAI"],"matched_entities":[],"excluded_terms":[],"recall_paths":["lexical","vector"],"scores":{"semantic":%.2f,"lexical":80,"entity":60,"title":70,"preference":50},"reason_codes":["lexical_candidate"],"provenance":{"scoring_version":"relevance-v1"}}`, score))
}

func float64Pointer(value float64) *float64 { return &value }

func relevanceReasonPresent(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
