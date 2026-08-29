//go:build integration

package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	eventapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/event/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/backend/test/postgresfixture"
)

func TestMicroEventQueryRepositoryAppliesMultiDimensionalFiltersAndStableRelevanceCursor(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	var actorID, evidenceProfileID int64
	if err := runtime.SQL.QueryRow(`INSERT INTO users (email,password_hash,display_name,role)
VALUES ('micro-event-query@example.test','fixture','Micro Event Query','admin') RETURNING id`).Scan(&actorID); err != nil {
		t.Fatal(err)
	}
	profileTransaction, err := runtime.SQL.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer profileTransaction.Rollback()
	if _, err := profileTransaction.Exec(`SET LOCAL session_replication_role='replica'`); err != nil {
		t.Fatal(err)
	}
	if _, err := profileTransaction.Exec(`INSERT INTO relevance_decision_profiles (
id,profile_name,matching_algorithm_version,reranker_version,calibration_version,status,
reject_threshold,accept_threshold,calibration_slope,calibration_intercept,evaluation_sample_count,
created_by_user_id,activated_by_user_id,activated_at)
VALUES (800002,'micro-event-query-fixture','rrf-k60-v1','reranker-v1','calibration-v1','active',
.3,.7,1,0,2,$1,$1,$2)`, actorID, now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := profileTransaction.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`INSERT INTO evidence_state_profiles
(algorithm_version,status,activated_by_user_id,activated_at)
VALUES ('evidence-state-query-fixture-v1','active',$1,$2) RETURNING id`, actorID, now.Add(-time.Hour)).Scan(&evidenceProfileID); err != nil {
		t.Fatal(err)
	}

	xFixture := seedMicroEventAssignmentFixture(t, runtime, "query-x", "accepted")
	xFixture.occurredAt = now.Add(-2 * time.Hour)
	rssFixture := seedMicroEventAssignmentFixture(t, runtime, "query-rss", "accepted")
	rssFixture.occurredAt = now.Add(-time.Hour)
	setMicroEventQueryFixtureDimensions(t, runtime, xFixture, "x", .95)
	setMicroEventQueryFixtureDimensions(t, runtime, rssFixture, "rss", .80)

	writeRepository, err := NewMicroEventRepository(runtime)
	if err != nil {
		t.Fatal(err)
	}
	xResult, err := writeRepository.CommitMicroEventMembership(ctx, microEventCommitFixture(
		xFixture, "create", 0, 0, strings.Repeat("a", 64), "micro-event-query-x"))
	if err != nil {
		t.Fatalf("create x event: %v", err)
	}
	rssResult, err := writeRepository.CommitMicroEventMembership(ctx, microEventCommitFixture(
		rssFixture, "create", 0, 0, strings.Repeat("b", 64), "micro-event-query-rss-longer"))
	if err != nil {
		t.Fatalf("create rss event: %v", err)
	}
	insertMicroEventQueryEvidenceState(t, runtime, xResult.Event.ID, xResult.Event.Version, evidenceProfileID,
		"multiple_origins", 2, now.Add(-30*time.Minute), "5")
	insertMicroEventQueryEvidenceState(t, runtime, rssResult.Event.ID, rssResult.Event.Version, evidenceProfileID,
		"single_origin", 1, now.Add(-20*time.Minute), "6")

	readRepository, err := NewMicroEventQueryPostgresRepository(runtime)
	if err != nil {
		t.Fatal(err)
	}
	readRepository.now = func() time.Time { return now.Add(time.Hour) }
	service, err := eventapplication.NewMicroEventQueryService(readRepository)
	if err != nil {
		t.Fatal(err)
	}

	firstPage, err := service.List(ctx, eventapplication.MicroEventListQuery{Sort: "relevance", Limit: 1})
	if err != nil {
		t.Fatalf("list first relevance page: %v", err)
	}
	if len(firstPage.Items) != 1 || firstPage.Items[0].ID != xResult.Event.ID ||
		firstPage.Items[0].RelevanceScore == nil || *firstPage.Items[0].RelevanceScore != .95 || firstPage.NextCursor == "" {
		t.Fatalf("first relevance page = %#v", firstPage)
	}
	concurrentFixture := seedMicroEventAssignmentFixture(t, runtime, "query-concurrent", "accepted")
	concurrentFixture.occurredAt = now.Add(-3 * time.Hour)
	setMicroEventQueryFixtureDimensions(t, runtime, concurrentFixture, "rss", .70)
	concurrentResult, err := writeRepository.CommitMicroEventMembership(ctx, microEventCommitFixture(
		concurrentFixture, "create", 0, 0, strings.Repeat("c", 64), "micro-event-query-new"))
	if err != nil {
		t.Fatalf("create concurrent event: %v", err)
	}
	secondPage, err := service.List(ctx, eventapplication.MicroEventListQuery{
		Sort: "relevance", Limit: 1, Cursor: firstPage.NextCursor,
	})
	if err != nil {
		t.Fatalf("list second relevance page: %v", err)
	}
	if len(secondPage.Items) != 1 || secondPage.Items[0].ID != rssResult.Event.ID ||
		secondPage.Items[0].RelevanceScore == nil || *secondPage.Items[0].RelevanceScore != .80 || secondPage.NextCursor != "" {
		t.Fatalf("second relevance page = %#v", secondPage)
	}
	freshPage, err := service.List(ctx, eventapplication.MicroEventListQuery{Sort: "relevance", Limit: 10})
	if err != nil {
		t.Fatalf("list fresh relevance page: %v", err)
	}
	if len(freshPage.Items) != 3 || freshPage.Items[2].ID != concurrentResult.Event.ID {
		t.Fatalf("fresh relevance page omitted concurrent event = %#v", freshPage)
	}
	if _, err := service.List(ctx, eventapplication.MicroEventListQuery{
		Sort: "relevance", Limit: 1, Cursor: firstPage.NextCursor, SourceTypes: []string{"x"},
	}); !errors.Is(err, eventapplication.ErrInvalidMicroEventQuery) {
		t.Fatalf("changed-filter cursor error = %v", err)
	}

	startedFrom, startedTo := now.Add(-3*time.Hour), now.Add(-90*time.Minute)
	filtered, err := service.List(ctx, eventapplication.MicroEventListQuery{
		Sort: "relevance", MonitorID: xFixture.monitorID, SourceTypes: []string{"x"},
		EvidenceStates: []string{"multiple_origins"}, StartedFrom: &startedFrom, StartedTo: &startedTo,
	})
	if err != nil {
		t.Fatalf("list combined filters: %v", err)
	}
	if len(filtered.Items) != 1 || filtered.Items[0].ID != xResult.Event.ID ||
		filtered.Items[0].LatestEvidenceState == nil || filtered.Items[0].LatestEvidenceState.State != "multiple_origins" {
		t.Fatalf("combined filtered page = %#v", filtered)
	}

	rssOnly, err := service.List(ctx, eventapplication.MicroEventListQuery{
		Sort: "latest", SourceTypes: []string{"rss"}, EvidenceStates: []string{"single_origin"},
	})
	if err != nil {
		t.Fatalf("list rss filters: %v", err)
	}
	if len(rssOnly.Items) != 1 || rssOnly.Items[0].ID != rssResult.Event.ID {
		t.Fatalf("rss filtered page = %#v", rssOnly)
	}
}

func setMicroEventQueryFixtureDimensions(t *testing.T, runtime *database.Runtime, fixture microEventAssignmentFixture, sourceType string, relevance float64) {
	t.Helper()
	transaction, err := runtime.SQL.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer transaction.Rollback()
	if _, err := transaction.Exec(`SET LOCAL session_replication_role='replica'`); err != nil {
		t.Fatal(err)
	}
	if _, err := transaction.Exec(`UPDATE source_connections SET source_type=$1 WHERE id=$2`, sourceType, fixture.sourceID); err != nil {
		t.Fatal(err)
	}
	if _, err := transaction.Exec(`UPDATE document_match_decisions SET relevance_probability=$1 WHERE id=$2`, relevance, fixture.matchDecisionID); err != nil {
		t.Fatal(err)
	}
	if err := transaction.Commit(); err != nil {
		t.Fatal(err)
	}
}

func insertMicroEventQueryEvidenceState(t *testing.T, runtime *database.Runtime, eventID, eventVersion, profileID int64,
	state string, originCount int, calculatedAt time.Time, hashCharacter string) {
	t.Helper()
	if _, err := runtime.SQL.Exec(`INSERT INTO evidence_state_snapshots (
micro_event_id,micro_event_version,evidence_state_profile_id,algorithm_version,evidence_set_hash,
evidence_state,independent_origin_count,reason_codes,calculated_at)
VALUES ($1,$2,$3,'evidence-state-query-fixture-v1',$4,$5,$6,'["fixture"]',$7)`,
		eventID, eventVersion, profileID, strings.Repeat(hashCharacter, 64), state, originCount, calculatedAt); err != nil {
		t.Fatal(err)
	}
}
