//go:build integration

package postgres

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/backend/test/postgresfixture"
)

func TestMicroEventQueryRepositoryListsOnlySafeCurrentContentReferences(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}
	fixture := seedMicroEventAssignmentFixture(t, runtime, "content-search", "accepted")
	microEvents, err := NewMicroEventRepository(runtime)
	if err != nil {
		t.Fatal(err)
	}
	result, err := microEvents.CommitMicroEventMembership(ctx, microEventCommitFixture(
		fixture, "create", 0, 0, strings.Repeat("9", 64), "content-search-micro-event",
	))
	if err != nil {
		t.Fatal(err)
	}
	var contentID int64
	const legacyEventID int64 = 900000
	now := time.Date(2026, 8, 8, 8, 0, 0, 0, time.UTC)
	if err := runtime.SQL.QueryRow(`
INSERT INTO contents (source_connection_id,external_id,content_type,title,canonical_url,published_at,fetched_at,dedupe_key)
VALUES ($1,'work-content-search','article','内容','https://example.test/content',$2,$2,$3) RETURNING id`, fixture.sourceID, now, strings.Repeat("a", 64)).Scan(&contentID); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE documents SET current_document_version_id=$1 WHERE source_connection_id=$2 AND external_work_id='work-content-search'`, fixture.documentVersionID, fixture.sourceID); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.Exec(`
INSERT INTO events (id,event_key,title_zh,summary,lifecycle_status,first_seen_at,last_seen_at)
VALUES ($1,'event-search-reference','错误的旧事件标题','内部摘要不可投影','active',$2,$2)`, legacyEventID, now); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.Exec(`
INSERT INTO event_contents (event_id,content_id,membership_score,evidence_role,origin)
VALUES ($1,$2,88,'primary','rule')`, legacyEventID, contentID); err != nil {
		t.Fatal(err)
	}

	queries, err := NewMicroEventQueryPostgresRepository(runtime)
	if err != nil {
		t.Fatal(err)
	}
	references, err := queries.ListContentSearchReferences(ctx, []int64{contentID})
	if err != nil || len(references) != 1 || references[0].ContentID != contentID || references[0].MicroEventID != result.Event.ID || references[0].MicroEventTitle != "subject:shared · action:released" {
		t.Fatalf("references/error = %#v/%v", references, err)
	}
	if references[0].MicroEventID == legacyEventID || references[0].MicroEventTitle == "错误的旧事件标题" {
		t.Fatalf("legacy event leaked into current content projection: %#v", references[0])
	}
	empty, err := queries.ListContentSearchReferences(ctx, nil)
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty references/error = %#v/%v", empty, err)
	}
}
