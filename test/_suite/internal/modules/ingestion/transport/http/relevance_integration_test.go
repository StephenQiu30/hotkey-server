package http

import (
	"context"
	"fmt"
	stdhttp "net/http"
	"strings"
	"testing"
	"time"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/internal/modules/ingestion/application"
	ingestiondomain "github.com/StephenQiu30/hotkey-server/internal/modules/ingestion/domain"
	ingestionpostgres "github.com/StephenQiu30/hotkey-server/internal/modules/ingestion/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	httptransport "github.com/StephenQiu30/hotkey-server/internal/platform/http"
	"github.com/gin-gonic/gin"
)

func TestPlan009RelevanceRoutesPostgresIntegration(t *testing.T) {
	gin.SetMode(gin.TestMode)
	runtime := openContentHTTPRuntime(t)
	defer func() { _ = runtime.Close() }()

	contents := ingestionpostgres.NewContentRepository(runtime)
	sourceID := createContentHTTPSource(t, runtime)
	contentInput := contentHTTPInput(sourceID, "relevance-http", time.Date(2026, time.July, 17, 9, 0, 0, 0, time.UTC))
	content, _, err := contents.Upsert(context.Background(), contentInput, ingestiondomain.DedupeDecision{Status: ingestiondomain.ContentStatusActive})
	if err != nil {
		t.Fatalf("create active content: %v", err)
	}
	monitorID, configID := createRelevanceHTTPMonitor(t, runtime)
	actorID := createRelevanceHTTPUser(t, runtime)

	snapshots := ingestionpostgres.NewRelevanceRepository(runtime)
	snapshot, _, err := snapshots.UpsertSnapshot(context.Background(), ingestiondomain.RelevanceSnapshotInput{
		MonitorID: monitorID, MonitorConfigVersionID: configID, ContentID: content.ID,
		InputHash: strings.Repeat("d", 64), ScoringVersion: "relevance-v1", RecallPaths: []string{"lexical"},
		ReasonCodes: []string{"lexical_candidate"}, RuleScore: 70, FinalScore: 70,
		Decision: ingestiondomain.MatchDecisionReview, DecisionOrigin: ingestiondomain.DecisionOriginRule,
		Explanation: []byte(`{"matched_terms":["OpenAI"],"matched_entities":[],"excluded_terms":[],"recall_paths":["lexical"],"scores":{"semantic":0,"lexical":80,"entity":60,"title":70,"preference":50},"reason_codes":["lexical_candidate"],"provenance":{"scoring_version":"relevance-v1"}}`),
	})
	if err != nil {
		t.Fatalf("create relevance snapshot: %v", err)
	}
	service, err := ingestionapplication.NewRelevanceAPIService(ingestionapplication.RelevanceAPIServiceDependencies{
		Snapshots: snapshots, Contents: contents, Candidates: ingestionpostgres.NewRelevanceCandidateReader(runtime),
	})
	if err != nil {
		t.Fatalf("NewRelevanceAPIService(): %v", err)
	}
	router := gin.New()
	RegisterRelevanceRoutes(router, service, relevanceIntegrationAuthenticator{userID: actorID, role: httptransport.RoleAdmin})

	list := performRelevanceRequest(router, stdhttp.MethodGet, fmt.Sprintf("/api/v1/monitors/%d/matches?limit=1", monitorID), "", "admin")
	if list.Code != stdhttp.StatusOK {
		t.Fatalf("list matches status = %d: %s", list.Code, list.Body.String())
	}
	if strings.Contains(list.Body.String(), contentInput.Excerpt) || strings.Contains(list.Body.String(), contentInput.Body) || strings.Contains(list.Body.String(), "provenance") || strings.Contains(list.Body.String(), snapshot.InputHash) {
		t.Fatalf("list matches leaked private relevance fact: %s", list.Body.String())
	}

	detail := performRelevanceRequest(router, stdhttp.MethodGet, fmt.Sprintf("/api/v1/monitors/%d/matches/%d", monitorID, snapshot.ID), "", "admin")
	if detail.Code != stdhttp.StatusOK {
		t.Fatalf("get match status = %d: %s", detail.Code, detail.Body.String())
	}
	if strings.Contains(detail.Body.String(), contentInput.Excerpt) || strings.Contains(detail.Body.String(), contentInput.Body) || strings.Contains(detail.Body.String(), "provenance") {
		t.Fatalf("match detail leaked private relevance fact: %s", detail.Body.String())
	}
	foreign := performRelevanceRequest(router, stdhttp.MethodGet, fmt.Sprintf("/api/v1/monitors/%d/matches/%d", monitorID+100000, snapshot.ID), "", "admin")
	if foreign.Code != stdhttp.StatusNotFound {
		t.Fatalf("foreign monitor match status = %d, want 404: %s", foreign.Code, foreign.Body.String())
	}

	var matchesBefore, runsBefore int
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM monitor_matches WHERE monitor_id = $1`, monitorID).Scan(&matchesBefore); err != nil {
		t.Fatalf("count matches before preview: %v", err)
	}
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM ai_runs`).Scan(&runsBefore); err != nil {
		t.Fatalf("count AI runs before preview: %v", err)
	}
	preview := performRelevanceRequest(router, stdhttp.MethodPost, fmt.Sprintf("/api/v1/monitors/%d/relevance-preview", monitorID), "", "admin")
	if preview.Code != stdhttp.StatusOK {
		t.Fatalf("preview status = %d: %s", preview.Code, preview.Body.String())
	}
	var matchesAfter, runsAfter int
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM monitor_matches WHERE monitor_id = $1`, monitorID).Scan(&matchesAfter); err != nil {
		t.Fatalf("count matches after preview: %v", err)
	}
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM ai_runs`).Scan(&runsAfter); err != nil {
		t.Fatalf("count AI runs after preview: %v", err)
	}
	if matchesAfter != matchesBefore || runsAfter != runsBefore {
		t.Fatalf("preview writes = matches %d->%d, ai runs %d->%d", matchesBefore, matchesAfter, runsBefore, runsAfter)
	}

	unmatchedInput := contentHTTPInput(sourceID, "relevance-http-unmatched", time.Date(2026, time.July, 17, 10, 0, 0, 0, time.UTC))
	unmatched, _, err := contents.Upsert(context.Background(), unmatchedInput, ingestiondomain.DedupeDecision{Status: ingestiondomain.ContentStatusActive})
	if err != nil {
		t.Fatalf("create unmatched active content: %v", err)
	}
	for _, feedbackType := range []string{"relevant", "irrelevant", "false_positive", "false_negative"} {
		invalidType := performRelevanceRequest(router, stdhttp.MethodPut, fmt.Sprintf("/api/v1/monitors/%d/contents/%d/feedback", monitorID, unmatched.ID), `{"feedback_type":"`+feedbackType+`","expected_feedback_version":null}`, "admin")
		if invalidType.Code != stdhttp.StatusBadRequest {
			t.Fatalf("content feedback type %q status = %d, want 400: %s", feedbackType, invalidType.Code, invalidType.Body.String())
		}
	}
	falseNegative := performRelevanceRequest(router, stdhttp.MethodPut, fmt.Sprintf("/api/v1/monitors/%d/contents/%d/feedback", monitorID, unmatched.ID), `{"expected_feedback_version":null}`, "admin")
	if falseNegative.Code != stdhttp.StatusOK {
		t.Fatalf("unmatched false-negative status = %d: %s", falseNegative.Code, falseNegative.Body.String())
	}
	matchedFalseNegative := performRelevanceRequest(router, stdhttp.MethodPut, fmt.Sprintf("/api/v1/monitors/%d/contents/%d/feedback", monitorID, content.ID), `{"expected_feedback_version":null}`, "admin")
	if matchedFalseNegative.Code != stdhttp.StatusConflict {
		t.Fatalf("matched false-negative status = %d, want 409: %s", matchedFalseNegative.Code, matchedFalseNegative.Body.String())
	}
	var matchedFalseNegativeCount int
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM monitor_match_feedbacks WHERE monitor_id = $1 AND content_id = $2 AND monitor_match_id IS NULL`, monitorID, content.ID).Scan(&matchedFalseNegativeCount); err != nil {
		t.Fatalf("count matched false-negative feedback: %v", err)
	}
	if matchedFalseNegativeCount != 0 {
		t.Fatalf("matched false-negative feedback rows = %d, want 0", matchedFalseNegativeCount)
	}

	feedback := performRelevanceRequest(router, stdhttp.MethodPut, fmt.Sprintf("/api/v1/monitors/%d/matches/%d/feedback", monitorID, snapshot.ID), `{"feedback_type":"relevant","expected_feedback_version":null}`, "admin")
	if feedback.Code != stdhttp.StatusOK {
		t.Fatalf("match feedback status = %d: %s", feedback.Code, feedback.Body.String())
	}
	missingExpectedVersion := performRelevanceRequest(router, stdhttp.MethodPut, fmt.Sprintf("/api/v1/monitors/%d/matches/%d/feedback", monitorID, snapshot.ID), `{"feedback_type":"irrelevant"}`, "admin")
	if missingExpectedVersion.Code != stdhttp.StatusBadRequest {
		t.Fatalf("missing expected version status = %d, want 400: %s", missingExpectedVersion.Code, missingExpectedVersion.Body.String())
	}
	conflict := performRelevanceRequest(router, stdhttp.MethodPut, fmt.Sprintf("/api/v1/monitors/%d/matches/%d/feedback", monitorID, snapshot.ID), `{"feedback_type":"irrelevant","expected_feedback_version":null}`, "admin")
	if conflict.Code != stdhttp.StatusConflict {
		t.Fatalf("duplicate initial feedback status = %d, want 409: %s", conflict.Code, conflict.Body.String())
	}
	evaluation := performRelevanceRequest(router, stdhttp.MethodGet, fmt.Sprintf("/api/v1/monitors/%d/feedback/evaluation", monitorID), "", "admin")
	if evaluation.Code != stdhttp.StatusOK {
		t.Fatalf("evaluation status = %d: %s", evaluation.Code, evaluation.Body.String())
	}
	refresh := performRelevanceRequest(router, stdhttp.MethodPost, fmt.Sprintf("/api/v1/monitors/%d/feedback/suggestions/refresh", monitorID), "", "admin")
	if refresh.Code != stdhttp.StatusOK {
		t.Fatalf("refresh suggestions status = %d: %s", refresh.Code, refresh.Body.String())
	}

	suggestion, _, err := snapshots.UpsertPendingSuggestion(context.Background(), ingestiondomain.RelevanceSuggestionInput{
		MonitorID: monitorID, MonitorConfigVersionID: configID, SuggestionType: ingestiondomain.SuggestionTypeAddTerm, Value: "OpenAI", SupportCount: 2,
	})
	if err != nil {
		t.Fatalf("create suggestion: %v", err)
	}
	suggestions := performRelevanceRequest(router, stdhttp.MethodGet, fmt.Sprintf("/api/v1/monitors/%d/feedback/suggestions?limit=5", monitorID), "", "admin")
	if suggestions.Code != stdhttp.StatusOK {
		t.Fatalf("list suggestions status = %d: %s", suggestions.Code, suggestions.Body.String())
	}
	review := performRelevanceRequest(router, stdhttp.MethodPost, fmt.Sprintf("/api/v1/monitors/%d/feedback/suggestions/%d/review", monitorID, suggestion.ID), `{"expected_version":1,"status":"approved"}`, "admin")
	if review.Code != stdhttp.StatusOK {
		t.Fatalf("review suggestion status = %d: %s", review.Code, review.Body.String())
	}
	var ruleCount int
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM monitor_rules WHERE config_version_id = $1`, configID).Scan(&ruleCount); err != nil {
		t.Fatalf("count monitor rules: %v", err)
	}
	if ruleCount != 0 {
		t.Fatalf("suggestion workflow wrote monitor rules = %d", ruleCount)
	}
}

type relevanceIntegrationAuthenticator struct {
	userID int64
	role   httptransport.Role
}

func (authenticator relevanceIntegrationAuthenticator) Authenticate(context.Context, string) (httptransport.Subject, error) {
	return httptransport.Subject{UserID: authenticator.userID, SessionID: 1, Role: authenticator.role}, nil
}

func createRelevanceHTTPMonitor(t *testing.T, runtime *database.Runtime) (int64, int64) {
	t.Helper()
	var monitorID, configID int64
	if err := runtime.SQL.QueryRow(`INSERT INTO monitors (name, status) VALUES ($1, 'draft') RETURNING id`, fmt.Sprintf("relevance-http-%d", time.Now().UnixNano())).Scan(&monitorID); err != nil {
		t.Fatalf("create monitor: %v", err)
	}
	if err := runtime.SQL.QueryRow(`INSERT INTO monitor_config_versions (monitor_id, revision) VALUES ($1, 1) RETURNING id`, monitorID).Scan(&configID); err != nil {
		t.Fatalf("create monitor config: %v", err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE monitor_config_versions SET state = 'published', config_hash = $1, published_at = now() WHERE id = $2`, strings.Repeat("e", 64), configID); err != nil {
		t.Fatalf("publish config: %v", err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE monitors SET status = 'active', published_config_version_id = $1 WHERE id = $2`, configID, monitorID); err != nil {
		t.Fatalf("activate monitor: %v", err)
	}
	return monitorID, configID
}

func createRelevanceHTTPUser(t *testing.T, runtime *database.Runtime) int64 {
	t.Helper()
	var userID int64
	if err := runtime.SQL.QueryRow(`
INSERT INTO users (email, password_hash, display_name, role)
VALUES ($1, 'hash', 'relevance http admin', 'admin') RETURNING id`, fmt.Sprintf("relevance-http-%d@example.test", time.Now().UnixNano())).Scan(&userID); err != nil {
		t.Fatalf("create relevance user: %v", err)
	}
	return userID
}
