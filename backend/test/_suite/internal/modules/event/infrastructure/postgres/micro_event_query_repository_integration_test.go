//go:build integration

package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	eventapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/event/application"
	searchdomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/search/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
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
	if detail, err := service.Get(ctx, xResult.Event.ID); err != nil || detail.ID != xResult.Event.ID {
		t.Fatalf("authorized event detail = %#v/%v", detail, err)
	}
	searchQuery := searchdomain.Query{Keyword: "shared", Types: []searchdomain.ResourceType{searchdomain.ResourceEvent}, Limit: 10}
	searchItems, err := readRepository.Search(ctx, searchQuery)
	if err != nil || !containsMicroEventSearchCandidate(searchItems, xResult.Event.ID) {
		t.Fatalf("authorized event search = %#v/%v", searchItems, err)
	}
	notificationFacts := insertMicroEventRevocationNotificationFacts(
		t, runtime, actorID, xFixture.monitorID, xResult.Event.ID, xResult.Event.Version, now,
	)
	beforeRevocation := readMicroEventRevocationNotificationFacts(t, runtime, notificationFacts)

	revokeMicroEventDisplayRights(t, runtime, xFixture)
	if _, err := service.Get(ctx, xResult.Event.ID); !errors.Is(err, sharedrepository.ErrNotFound) {
		t.Fatalf("revoked event detail error = %v, want not found", err)
	}
	visiblePage, err := service.List(ctx, eventapplication.MicroEventListQuery{Sort: "latest", Limit: 10})
	if err != nil {
		t.Fatalf("list after rights revocation: %v", err)
	}
	for _, item := range visiblePage.Items {
		if item.ID == xResult.Event.ID {
			t.Fatalf("revoked event remained in list: %#v", item)
		}
	}
	searchItems, err = readRepository.Search(ctx, searchQuery)
	if err != nil || containsMicroEventSearchCandidate(searchItems, xResult.Event.ID) {
		t.Fatalf("revoked event search = %#v/%v, want event hidden", searchItems, err)
	}
	afterRevocation := readMicroEventRevocationNotificationFacts(t, runtime, notificationFacts)
	if afterRevocation != beforeRevocation {
		t.Fatalf("rights revocation changed durable notification facts\nbefore=%#v\nafter=%#v", beforeRevocation, afterRevocation)
	}
}

type microEventRevocationNotificationFactIDs struct {
	outbox, notification, receipt, deliveryAttempt int64
}

type microEventRevocationNotificationFacts struct {
	outbox, notification, receipt, deliveryAttempt string
}

func insertMicroEventRevocationNotificationFacts(
	t *testing.T,
	runtime *database.Runtime,
	userID, monitorID, eventID, eventVersion int64,
	occurredAt time.Time,
) microEventRevocationNotificationFactIDs {
	t.Helper()
	if _, err := runtime.SQL.Exec(`UPDATE monitors SET created_by=$1,updated_by=$1 WHERE id=$2`, userID, monitorID); err != nil {
		t.Fatal(err)
	}
	var ids microEventRevocationNotificationFactIDs
	if err := runtime.SQL.QueryRow(`INSERT INTO notification_outbox_events(
event_type,resource_type,resource_id,resource_version,monitor_id,occurred_at,title,summary,resource_status,deep_link,dedupe_key)
VALUES ('micro_event.updated','micro_event',$1,$2,$3,$4,'事件更新','撤权前的不可变通知事实','active',$5,$6)
RETURNING id`, eventID, eventVersion, monitorID, occurredAt,
		fmt.Sprintf("/dashboard/events?event=%d", eventID), fmt.Sprintf("rights-revocation:event:%d:v%d", eventID, eventVersion),
	).Scan(&ids.outbox); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`INSERT INTO user_notifications(
outbox_event_id,user_id,monitor_id,event_type,resource_type,resource_id,resource_version,occurred_at,title,summary,resource_status,deep_link)
VALUES ($1,$2,$3,'micro_event.updated','micro_event',$4,$5,$6,'事件更新','撤权前的不可变通知事实','active',$7)
RETURNING id`, ids.outbox, userID, monitorID, eventID, eventVersion, occurredAt,
		fmt.Sprintf("/dashboard/events?event=%d", eventID),
	).Scan(&ids.notification); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`INSERT INTO notification_read_receipts(user_id,read_through_id)
VALUES ($1,$2) RETURNING id`, userID, ids.notification).Scan(&ids.receipt); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`INSERT INTO notification_delivery_attempts(
user_notification_id,channel,delivery_target_key,attempt_no,status,attempted_at)
VALUES ($1,'websocket','browser_ws',1,'succeeded',$2) RETURNING id`, ids.notification, occurredAt,
	).Scan(&ids.deliveryAttempt); err != nil {
		t.Fatal(err)
	}
	return ids
}

func readMicroEventRevocationNotificationFacts(
	t *testing.T,
	runtime *database.Runtime,
	ids microEventRevocationNotificationFactIDs,
) microEventRevocationNotificationFacts {
	t.Helper()
	var facts microEventRevocationNotificationFacts
	for _, item := range []struct {
		query string
		id    int64
		value *string
	}{
		{`SELECT row_to_json(fact)::text FROM notification_outbox_events AS fact WHERE id=$1`, ids.outbox, &facts.outbox},
		{`SELECT row_to_json(fact)::text FROM user_notifications AS fact WHERE id=$1`, ids.notification, &facts.notification},
		{`SELECT row_to_json(fact)::text FROM notification_read_receipts AS fact WHERE id=$1`, ids.receipt, &facts.receipt},
		{`SELECT row_to_json(fact)::text FROM notification_delivery_attempts AS fact WHERE id=$1`, ids.deliveryAttempt, &facts.deliveryAttempt},
	} {
		if err := runtime.SQL.QueryRow(item.query, item.id).Scan(item.value); err != nil {
			t.Fatal(err)
		}
	}
	return facts
}

func containsMicroEventSearchCandidate(items []searchdomain.Candidate, eventID int64) bool {
	for _, item := range items {
		if item.ID == eventID {
			return true
		}
	}
	return false
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
