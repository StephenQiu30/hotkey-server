//go:build integration

package postgres

import (
	"context"
	"strings"
	"testing"
	"time"

	searchdomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/search/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/backend/test/postgresfixture"
)

func TestKnowledgeRepositorySearchesCurrentAppliedPostgresProjection(t *testing.T) {
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
	if err := runtime.SQL.QueryRowContext(ctx, `INSERT INTO events
  (event_key,title_zh,summary,lifecycle_status,first_seen_at,last_seen_at)
VALUES ('knowledge-search-' || md5(random()::text),'Knowledge search','','active',$1,$1)
RETURNING id`, now).Scan(&eventID); err != nil {
		t.Fatal(err)
	}
	const documentID = int64(99101)
	const proposalID = int64(99102)
	frontmatter := `{"title":"芯片 Release 知识","entities":["Acme-42"]}`
	body := "PostgreSQL full text search authorized knowledge body"
	if _, err := runtime.SQL.ExecContext(ctx, `INSERT INTO knowledge_documents
  (id,version,document_type,event_id,vault_path,revision_no,content_hash,generated_hash,status,last_written_at)
VALUES ($1,2,'event',$2,'events/search-knowledge.md',1,$3,$4,'active',$5)`,
		documentID, eventID, strings.Repeat("a", 64), strings.Repeat("b", 64), now); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.ExecContext(ctx, `INSERT INTO knowledge_change_proposals
  (id,version,document_id,change_type,base_revision_no,base_hash,proposed_frontmatter,proposed_body,diff_summary,reason,status,applied_at,created_at,updated_at)
VALUES ($1,3,$2,'create',0,$3,$4::jsonb,$5,'fixture','fixture','applied',$6,$6,$6)`,
		proposalID, documentID, strings.Repeat("c", 64), frontmatter, body, now); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.ExecContext(ctx, `INSERT INTO knowledge_revisions
  (document_id,revision_no,source,proposal_id,previous_hash,new_hash,frontmatter_snapshot,created_at)
VALUES ($1,1,'proposal',$2,NULL,$3,$4::jsonb,$5)`,
		documentID, proposalID, strings.Repeat("a", 64), frontmatter, now); err != nil {
		t.Fatal(err)
	}
	repository := NewRepository(runtime)
	queries := []searchdomain.Query{
		{Keyword: "芯片"},
		{Keyword: "releas"},
		{Keyword: "authorized"},
		{Keyword: "release", Entity: "acme-42"},
		{Keyword: "PostgreSQL", Status: "active", From: knowledgeSearchTime(now.Add(-time.Minute)), To: knowledgeSearchTime(now.Add(time.Minute))},
	}
	for _, query := range queries {
		query.Types = []searchdomain.ResourceType{searchdomain.ResourceKnowledge}
		query.Limit = 10
		items, err := repository.Search(ctx, query.Normalized())
		if err != nil || len(items) != 1 || items[0].Type != searchdomain.ResourceKnowledge || items[0].ID != documentID || items[0].Title != "芯片 Release 知识" || items[0].OccurredAt != now || items[0].Score < 0 {
			t.Fatalf("Search(%#v) = %#v/%v", query, items, err)
		}
	}
	filtered, err := repository.Search(ctx, searchdomain.Query{Keyword: "release", MonitorID: knowledgeSearchID(1), Types: []searchdomain.ResourceType{searchdomain.ResourceKnowledge}, Limit: 10}.Normalized())
	if err != nil || len(filtered) != 0 {
		t.Fatalf("Search(cross-owner filter) = %#v/%v", filtered, err)
	}
	visibilityQuery := searchdomain.Query{Keyword: "release", Types: []searchdomain.ResourceType{searchdomain.ResourceKnowledge}, Limit: 10}.Normalized()
	visibleItems, err := repository.Search(ctx, visibilityQuery)
	if err != nil || len(visibleItems) != 1 {
		t.Fatalf("Search(visibility) = %#v/%v", visibleItems, err)
	}
	if visible, err := repository.CanDisplay(ctx, visibilityQuery, visibleItems[0]); err != nil || !visible {
		t.Fatalf("CanDisplay(active) = %v/%v", visible, err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE knowledge_documents SET status='archived',version=version+1,updated_at=now() WHERE id=$1`, documentID); err != nil {
		t.Fatal(err)
	}
	if visible, err := repository.CanDisplay(ctx, visibilityQuery, visibleItems[0]); err != nil || visible {
		t.Fatalf("CanDisplay(archived) = %v/%v", visible, err)
	}
}

func knowledgeSearchTime(value time.Time) *time.Time { return &value }
func knowledgeSearchID(value int64) *int64           { return &value }
