//go:build integration

package postgres

import (
	"context"
	"encoding/json"
	"strings"
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
	var eventID int64
	if err := runtime.SQL.QueryRow(`
INSERT INTO micro_events (
  event_key,status,primary_subject_key,primary_action_key,location_keys,identifier_keys,
  event_started_at,clustering_profile_version
) VALUES ($1,'review_pending',$2,$3,$4,$5,$6,'lexical-v1') RETURNING id`,
		strings.Repeat("a", 64), `芯片 <img src=x onerror=sentinel>`, "Release update",
		[]string{"上海", "CN"}, []string{"Acme-42"}, now).Scan(&eventID); err != nil {
		t.Fatal(err)
	}
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
