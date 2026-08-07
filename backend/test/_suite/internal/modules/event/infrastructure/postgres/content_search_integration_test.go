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

func TestEventRepositoryListsSafeCurrentContentSearchReferences(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}
	var sourceID, contentID, eventID int64
	if err := runtime.SQL.QueryRow(`INSERT INTO source_connections (source_type,name,endpoint) VALUES ('rss','event-search','https://example.test/feed') RETURNING id`).Scan(&sourceID); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 8, 8, 0, 0, 0, time.UTC)
	if err := runtime.SQL.QueryRow(`
INSERT INTO contents (source_connection_id,external_id,content_type,title,canonical_url,published_at,fetched_at,dedupe_key)
VALUES ($1,'event-search-content','article','内容','https://example.test/content',$2,$2,$3) RETURNING id`, sourceID, now, strings.Repeat("a", 64)).Scan(&contentID); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`
INSERT INTO events (event_key,title_zh,summary,lifecycle_status,first_seen_at,last_seen_at)
VALUES ('event-search-reference','安全事件标题','内部摘要不可投影','active',$1,$1) RETURNING id`, now).Scan(&eventID); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.Exec(`
INSERT INTO event_contents (event_id,content_id,membership_score,evidence_role,origin)
VALUES ($1,$2,88,'primary','rule')`, eventID, contentID); err != nil {
		t.Fatal(err)
	}

	references, err := NewRepository(runtime).ListContentSearchReferences(ctx, []int64{contentID})
	if err != nil || len(references) != 1 || references[0].ContentID != contentID || references[0].EventID != eventID || references[0].EventTitle != "安全事件标题" {
		t.Fatalf("references/error = %#v/%v", references, err)
	}
	empty, err := NewRepository(runtime).ListContentSearchReferences(ctx, nil)
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty references/error = %#v/%v", empty, err)
	}
}
