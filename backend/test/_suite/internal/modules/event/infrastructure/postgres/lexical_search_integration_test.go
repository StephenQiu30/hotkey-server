//go:build integration

package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	searchdomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/search/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/backend/test/postgresfixture"
)

func TestMicroEventQueryRepositorySearchesPostgresFTSAndTrigramFields(t *testing.T) {
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
	eventID := seedAuthorizedMicroEventSearchProjection(t, runtime, "lexical-fields", now, now)
	updateMicroEventSearchProjection(t, runtime, eventID, `芯片 <img src=x onerror=sentinel>`, "Release update",
		[]string{"上海", "CN"}, []string{"Acme-42"}, "review_pending", now, now)
	repository, err := NewMicroEventQueryPostgresRepository(runtime)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := repository.ExplainSearch(ctx, searchdomain.Query{
		Keyword: "release", Types: []searchdomain.ResourceType{searchdomain.ResourceEvent}, Limit: 10,
	})
	if err != nil || !json.Valid(plan) {
		t.Fatalf("ExplainSearch() = %s/%v", plan, err)
	}
	queries := []searchdomain.Query{
		{Keyword: "芯片"},
		{Keyword: "releas"},
		{Keyword: "release", Entity: "acme-42"},
		{Keyword: "上海", Status: "review_pending", From: eventSearchTime(now.Add(-time.Minute)), To: eventSearchTime(now.Add(time.Minute))},
	}
	for _, query := range queries {
		query.Types = []searchdomain.ResourceType{searchdomain.ResourceEvent}
		query.Limit = 10
		items, err := repository.Search(ctx, query.Normalized())
		if err != nil || len(items) != 1 || items[0].Type != searchdomain.ResourceEvent || items[0].ID != eventID || items[0].Status != "review_pending" || items[0].OccurredAt != now || items[0].Score < 0 {
			t.Fatalf("Search(%#v) = %#v/%v", query, items, err)
		}
	}
	wrongStatus, err := repository.Search(ctx, searchdomain.Query{Keyword: "release", Status: "closed", Types: []searchdomain.ResourceType{searchdomain.ResourceEvent}, Limit: 10}.Normalized())
	if err != nil || len(wrongStatus) != 0 {
		t.Fatalf("Search(wrong status) = %#v/%v", wrongStatus, err)
	}
	visibilityQuery := searchdomain.Query{Keyword: "release", Status: "review_pending", Types: []searchdomain.ResourceType{searchdomain.ResourceEvent}, Limit: 10}.Normalized()
	visibleItems, err := repository.Search(ctx, visibilityQuery)
	if err != nil || len(visibleItems) != 1 {
		t.Fatalf("Search(visibility) = %#v/%v", visibleItems, err)
	}
	if visible, err := repository.CanDisplay(ctx, visibilityQuery, visibleItems[0]); err != nil || !visible {
		t.Fatalf("CanDisplay(review pending) = %v/%v", visible, err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE micro_events SET status='closed',version=version+1,event_ended_at=now(),updated_at=now() WHERE id=$1`, eventID); err != nil {
		t.Fatal(err)
	}
	if visible, err := repository.CanDisplay(ctx, visibilityQuery, visibleItems[0]); err != nil || visible {
		t.Fatalf("CanDisplay(changed status) = %v/%v", visible, err)
	}
}

func eventSearchTime(value time.Time) *time.Time { return &value }

func TestMicroEventLexicalSearchUsesSnapshotKeysetOrdering(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}
	base := time.Now().UTC().Truncate(time.Microsecond)
	eventIDs := make([]int64, 0, 3)
	for index, createdAt := range []time.Time{base.Add(-2 * time.Minute), base.Add(-time.Minute), base.Add(time.Second)} {
		eventID := seedAuthorizedMicroEventSearchProjection(t, runtime, "snapshot-"+string(rune('a'+index)), base.Add(-time.Hour), createdAt)
		updateMicroEventSearchProjection(t, runtime, eventID, "Release", "snapshot ordering", nil, nil, "active", base.Add(-time.Hour), createdAt)
		eventIDs = append(eventIDs, eventID)
	}
	repository, err := NewMicroEventQueryPostgresRepository(runtime)
	if err != nil {
		t.Fatal(err)
	}
	query := searchdomain.Query{
		Keyword: "release", Types: []searchdomain.ResourceType{searchdomain.ResourceEvent}, Sort: searchdomain.SortLatest,
		Limit: 1, CandidateLimit: 2, SnapshotAt: base,
	}.Normalized()
	first, err := repository.Search(ctx, query)
	if err != nil || len(first) != 2 || first[0].ID != eventIDs[1] || first[1].ID != eventIDs[0] {
		t.Fatalf("snapshot first candidates = %#v / %v", first, err)
	}
	query.After = &searchdomain.Position{
		Type: first[0].Type, ID: first[0].ID, Score: first[0].Score, OccurredAt: first[0].OccurredAt,
	}
	second, err := repository.Search(ctx, query)
	if err != nil || len(second) != 1 || second[0].ID != eventIDs[0] || second[0].ID == eventIDs[2] {
		t.Fatalf("snapshot second candidates = %#v / %v; concurrent=%d", second, err, eventIDs[2])
	}
}

func seedAuthorizedMicroEventSearchProjection(t *testing.T, runtime *database.Runtime, suffix string, occurredAt, createdAt time.Time) int64 {
	t.Helper()
	fixture := seedMicroEventAssignmentFixture(t, runtime, "event-search-"+suffix, "accepted")
	fixture.occurredAt = occurredAt
	repository, err := NewMicroEventRepository(runtime)
	if err != nil {
		t.Fatal(err)
	}
	command := microEventCommitFixture(fixture, "create", 0, 0,
		fmt.Sprintf("%064x", uint64(occurredAt.UnixNano())^uint64(len(suffix))), "event-search-"+suffix)
	command.EventKey = fmt.Sprintf("%064x", uint64(createdAt.UnixNano())^uint64(len(suffix)*31))
	result, err := repository.CommitMicroEventMembership(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	return result.Event.ID
}

func updateMicroEventSearchProjection(t *testing.T, runtime *database.Runtime, eventID int64, subject, action string,
	locations, identifiers []string, status string, startedAt, createdAt time.Time,
) {
	t.Helper()
	if locations == nil {
		locations = []string{}
	}
	if identifiers == nil {
		identifiers = []string{}
	}
	transaction, err := runtime.SQL.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer transaction.Rollback()
	if _, err := transaction.Exec(`SET LOCAL session_replication_role='replica'`); err != nil {
		t.Fatal(err)
	}
	if _, err := transaction.Exec(`UPDATE micro_events
SET status=$1,primary_subject_key=$2,primary_action_key=$3,location_keys=$4,identifier_keys=$5,
    event_started_at=$6,created_at=$7,updated_at=$7
WHERE id=$8`, status, subject, action, locations, identifiers, startedAt, createdAt, eventID); err != nil {
		t.Fatal(err)
	}
	if err := transaction.Commit(); err != nil {
		t.Fatal(err)
	}
}
