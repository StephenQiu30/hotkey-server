//go:build integration

package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/backend/internal/shared/pagination"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
	"github.com/StephenQiu30/hotkey-server/backend/test/postgresfixture"
)

func TestKnowledgeListCursorsAreSignedBoundExpiringAndSnapshotStable(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}
	codec, err := pagination.NewCodec(strings.Repeat("knowledge-list-secret-", 2), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	repository := NewRepositoryWithCursorCodec(runtime, codec)

	var eventID int64
	now := time.Now().UTC().Truncate(time.Microsecond)
	if err := runtime.SQL.QueryRowContext(ctx, `
INSERT INTO events (event_key, title_zh, summary, lifecycle_status, first_seen_at, last_seen_at)
VALUES ('knowledge-list-' || md5(random()::text), 'Knowledge list', '', 'active', $1, $1)
RETURNING id`, now).Scan(&eventID); err != nil {
		t.Fatal(err)
	}
	insertDocument := func(id int64, status domain.DocumentStatus) {
		t.Helper()
		if _, err := runtime.SQL.ExecContext(ctx, `
INSERT INTO knowledge_documents (id, version, document_type, event_id, vault_path, revision_no, status)
VALUES ($1, 1, 'event', $2, $3, 0, $4)`, id, eventID, fmt.Sprintf("events/list-%d.md", id), status); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []int64{9101, 9102, 9103} {
		insertDocument(id, domain.DocumentActive)
	}
	insertDocument(9199, domain.DocumentArchived)

	documentsFirst, err := repository.ListDocumentPage(ctx, domain.DocumentListQuery{Limit: 2})
	if err != nil || len(documentsFirst.Items) != 2 || documentsFirst.Items[0].ID != 9101 || documentsFirst.Items[1].ID != 9102 || documentsFirst.NextCursor == "" {
		t.Fatalf("first document page = %#v/%v", documentsFirst, err)
	}
	insertDocument(9104, domain.DocumentActive)
	documentsSecond, err := repository.ListDocumentPage(ctx, domain.DocumentListQuery{Limit: 2, Cursor: documentsFirst.NextCursor})
	if err != nil || len(documentsSecond.Items) != 1 || documentsSecond.Items[0].ID != 9103 || documentsSecond.NextCursor != "" {
		t.Fatalf("second document page = %#v/%v", documentsSecond, err)
	}
	for name, query := range map[string]domain.DocumentListQuery{
		"tampered":  {Limit: 2, Cursor: tamperKnowledgeCursor(documentsFirst.NextCursor)},
		"oversized": {Limit: 201},
	} {
		if _, err := repository.ListDocumentPage(ctx, query); !errors.Is(err, sharedrepository.ErrInvalidInput) {
			t.Errorf("%s document cursor error = %v", name, err)
		}
	}

	insertProposal := func(id int64, status domain.ProposalStatus) {
		t.Helper()
		if _, err := runtime.SQL.ExecContext(ctx, `
INSERT INTO knowledge_change_proposals
  (id, version, document_id, change_type, base_revision_no, base_hash, proposed_frontmatter, proposed_body, status, created_at, updated_at)
VALUES ($1, 1, 9101, 'update', 0, $2, '{}'::jsonb, '', $3, $4, $4)`, id, strings.Repeat("a", 64), status, now); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []int64{9201, 9202, 9203} {
		insertProposal(id, domain.ProposalPending)
	}
	insertProposal(9299, domain.ProposalApproved)

	proposalsFirst, err := repository.ListProposalPage(ctx, domain.ProposalListQuery{Limit: 2, Status: domain.ProposalPending})
	if err != nil || len(proposalsFirst.Items) != 2 || proposalsFirst.Items[0].ID != 9203 || proposalsFirst.Items[1].ID != 9202 || proposalsFirst.NextCursor == "" {
		t.Fatalf("first proposal page = %#v/%v", proposalsFirst, err)
	}
	insertProposal(9204, domain.ProposalPending)
	proposalsSecond, err := repository.ListProposalPage(ctx, domain.ProposalListQuery{Limit: 2, Status: domain.ProposalPending, Cursor: proposalsFirst.NextCursor})
	if err != nil || len(proposalsSecond.Items) != 1 || proposalsSecond.Items[0].ID != 9201 || proposalsSecond.NextCursor != "" {
		t.Fatalf("second proposal page = %#v/%v", proposalsSecond, err)
	}
	for name, query := range map[string]domain.ProposalListQuery{
		"tampered":       {Limit: 2, Status: domain.ProposalPending, Cursor: tamperKnowledgeCursor(proposalsFirst.NextCursor)},
		"cross-filter":   {Limit: 2, Status: domain.ProposalApproved, Cursor: proposalsFirst.NextCursor},
		"cross-purpose":  {Limit: 2, Status: domain.ProposalPending, Cursor: documentsFirst.NextCursor},
		"invalid-status": {Limit: 2, Status: domain.ProposalStatus("unknown")},
		"oversized":      {Limit: 201, Status: domain.ProposalPending},
	} {
		if _, err := repository.ListProposalPage(ctx, query); !errors.Is(err, sharedrepository.ErrInvalidInput) {
			t.Errorf("%s proposal cursor error = %v", name, err)
		}
	}

	shortCodec, err := pagination.NewCodec(strings.Repeat("short-knowledge-list-secret-", 2), time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	shortRepository := NewRepositoryWithCursorCodec(runtime, shortCodec)
	expiring, err := shortRepository.ListDocumentPage(ctx, domain.DocumentListQuery{Limit: 1})
	if err != nil || expiring.NextCursor == "" {
		t.Fatalf("expiring document page = %#v/%v", expiring, err)
	}
	time.Sleep(5 * time.Millisecond)
	if _, err := shortRepository.ListDocumentPage(ctx, domain.DocumentListQuery{Limit: 1, Cursor: expiring.NextCursor}); !errors.Is(err, sharedrepository.ErrInvalidInput) {
		t.Fatalf("expired document cursor error = %v", err)
	}
}

func tamperKnowledgeCursor(value string) string {
	replacement := byte('x')
	if value[len(value)-1] == replacement {
		replacement = 'y'
	}
	return value[:len(value)-1] + string(replacement)
}

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

func TestKnowledgeRepositoryLoadsVaultRebuildFact(t *testing.T) {
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
	if err := runtime.SQL.QueryRowContext(ctx, `INSERT INTO events (event_key, title_zh, summary, lifecycle_status, first_seen_at, last_seen_at) VALUES ('knowledge-rebuild-' || md5(random()::text), 'Knowledge rebuild', '', 'active', $1, $1) RETURNING id`, now).Scan(&eventID); err != nil {
		t.Fatal(err)
	}
	repository := NewRepository(runtime)
	emptyHash := domain.HashContent("", "")
	document := domain.Document{
		ID: 9701, Version: 1, RevisionNo: 0, Type: domain.DocumentEvent, EventID: &eventID,
		VaultPath: "events/9701.md", ContentHash: emptyHash, GeneratedHash: emptyHash, Status: domain.DocumentPlanned,
	}
	if err := repository.SaveDocument(ctx, document); err != nil {
		t.Fatal(err)
	}
	frontmatter := `{"title":"Knowledge rebuild"}`
	body := "approved generated body"
	proposal := domain.Proposal{
		ID: 9801, Version: 1, DocumentID: document.ID, BaseRevisionNo: 0, BaseHash: emptyHash,
		ProposedFrontmatter: frontmatter, ProposedBody: body, Reason: "fixture", Status: domain.ProposalPending,
	}
	if err := repository.SaveProposal(proposal); err != nil {
		t.Fatal(err)
	}
	approved, err := repository.UpdateProposalStatus(ctx, proposal.ID, proposal.Version, domain.ProposalApproved)
	if err != nil {
		t.Fatal(err)
	}
	content, err := domain.RenderVaultDocument(domain.VaultDocumentRenderInput{
		DocumentID: document.ID, RevisionNo: 1, Type: document.Type, SourceID: eventID,
		Title: "Knowledge rebuild", Generated: body,
	})
	if err != nil {
		t.Fatal(err)
	}
	next := document
	next.Version = 2
	next.RevisionNo = 1
	next.ContentHash = domain.HashContent("", content)
	next.GeneratedHash = domain.HashContent(approved.ProposedFrontmatter, body)
	next.Status = domain.DocumentActive
	revision := domain.Revision{
		DocumentID: document.ID, RevisionNo: 1, ProposalID: proposal.ID, Source: "proposal",
		PreviousHash: emptyHash, NewHash: next.ContentHash, SnapshotObjectKey: "knowledge/v1/9701/1.md", Frontmatter: approved.ProposedFrontmatter,
	}
	if _, err := repository.ApplyProposal(ctx, proposal.ID, approved.Version, next, revision); err != nil {
		t.Fatal(err)
	}

	fact, err := repository.LoadVaultRebuildFact(ctx, document.ID)
	if err != nil {
		t.Fatal(err)
	}
	if fact.Document.ContentHash != next.ContentHash || fact.SnapshotObjectKey != revision.SnapshotObjectKey || fact.RenderInput.DocumentID != document.ID || fact.RenderInput.RevisionNo != 1 || fact.RenderInput.SourceID != eventID || fact.RenderInput.Title != "Knowledge rebuild" || fact.RenderInput.Generated != body {
		t.Fatalf("Vault rebuild fact = %#v", fact)
	}
}
