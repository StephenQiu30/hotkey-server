//go:build integration

package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
	"github.com/StephenQiu30/hotkey-server/backend/test/postgresfixture"
)

func TestKnowledgeRepositoryPersistsDocumentAndProposal(t *testing.T) {
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
	if err := runtime.SQL.QueryRowContext(ctx, `INSERT INTO events (event_key, title_zh, summary, lifecycle_status, first_seen_at, last_seen_at) VALUES ('knowledge-event-' || md5(random()::text), 'Knowledge event', '', 'active', $1, $1) RETURNING id`, now).Scan(&eventID); err != nil {
		t.Fatal(err)
	}
	repository := NewRepository(runtime)
	document := domain.Document{ID: 9501, Version: 1, RevisionNo: 0, Type: domain.DocumentEvent, VaultPath: "events/knowledge-event.md", Status: domain.DocumentPlanned, EventID: &eventID}
	if err := repository.SaveDocument(ctx, document); err != nil {
		t.Fatal(err)
	}
	got, err := repository.GetDocument(document.ID)
	if err != nil || got.EventID == nil || *got.EventID != eventID {
		t.Fatalf("GetDocument() = %#v/%v", got, err)
	}
	proposal := domain.Proposal{ID: 9601, Version: 1, DocumentID: document.ID, BaseRevisionNo: 0, BaseHash: strings.Repeat("a", 64), ProposedFrontmatter: `{}`, ProposedBody: "updated", Reason: "fixture", Status: domain.ProposalPending}
	if err := repository.SaveProposal(proposal); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := runtime.SQL.QueryRowContext(ctx, `SELECT count(*) FROM knowledge_change_proposals WHERE id = $1`, proposal.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("proposal count = %d, want 1", count)
	}
	approved, err := repository.UpdateProposalStatus(ctx, proposal.ID, proposal.Version, domain.ProposalApproved)
	if err != nil || approved.Version != 2 {
		t.Fatalf("UpdateProposalStatus() = %#v/%v", approved, err)
	}
	if err := repository.MarkProposalConflict(ctx, proposal.ID, approved.Version); err != nil {
		t.Fatal(err)
	}
	var status string
	var version int64
	if err := runtime.SQL.QueryRowContext(ctx, `SELECT status, version FROM knowledge_change_proposals WHERE id = $1`, proposal.ID).Scan(&status, &version); err != nil {
		t.Fatal(err)
	}
	if status != "conflict" || version != 3 {
		t.Fatalf("persisted conflict = %s/v%d", status, version)
	}
	if err := repository.MarkProposalConflict(ctx, proposal.ID, approved.Version); !errors.Is(err, sharedrepository.ErrConflict) {
		t.Fatalf("stale MarkProposalConflict() error = %v", err)
	}
	next := got
	next.Version++
	next.RevisionNo++
	next.ContentHash = strings.Repeat("b", 64)
	next.GeneratedHash = strings.Repeat("c", 64)
	next.Status = domain.DocumentActive
	revision := domain.Revision{
		DocumentID: got.ID, RevisionNo: next.RevisionNo, ProposalID: proposal.ID,
		Source: "proposal", PreviousHash: got.ContentHash, NewHash: next.ContentHash, Frontmatter: `{}`,
	}
	if _, err := repository.ApplyProposal(ctx, proposal.ID, approved.Version, next, revision); !errors.Is(err, sharedrepository.ErrConflict) {
		t.Fatalf("ApplyProposal() after conflict error = %v", err)
	}
	unchanged, err := repository.GetDocument(got.ID)
	if err != nil || unchanged.Version != got.Version || unchanged.RevisionNo != got.RevisionNo || unchanged.ContentHash != got.ContentHash {
		t.Fatalf("document after rejected apply = %#v/%v, want %#v", unchanged, err, got)
	}
	if err := runtime.SQL.QueryRowContext(ctx, `SELECT count(*) FROM knowledge_revisions WHERE proposal_id = $1`, proposal.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("revision count after rejected apply = %d", count)
	}
}
