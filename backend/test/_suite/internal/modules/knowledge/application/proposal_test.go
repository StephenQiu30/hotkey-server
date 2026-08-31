package application

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/domain"
	operationsdomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/domain"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

type proposalDocumentsFake struct{ document domain.Document }

func (fake proposalDocumentsFake) GetDocument(context.Context, int64) (domain.Document, error) {
	return fake.document, nil
}

type proposalStoreFake struct {
	proposal       domain.Proposal
	updated        domain.Document
	markedConflict bool
	applyErr       error
}

func (fake *proposalStoreFake) CreateProposal(_ context.Context, proposal domain.Proposal) (domain.Proposal, error) {
	proposal.ID = 1
	fake.proposal = proposal
	return proposal, nil
}
func (fake *proposalStoreFake) UpdateProposalStatus(_ context.Context, id, version int64, status domain.ProposalStatus) (domain.Proposal, error) {
	fake.proposal.ID, fake.proposal.Version, fake.proposal.Status = id, version+1, status
	return fake.proposal, nil
}
func (fake *proposalStoreFake) ApplyProposal(_ context.Context, _ int64, _ int64, document domain.Document, _ domain.Revision) (domain.Document, error) {
	fake.updated = document
	return document, fake.applyErr
}
func (fake *proposalStoreFake) MarkProposalConflict(_ context.Context, id, version int64) error {
	fake.markedConflict = true
	fake.proposal.ID, fake.proposal.Version, fake.proposal.Status = id, version+1, domain.ProposalConflict
	return nil
}

type proposalVaultFake struct {
	content    string
	reads      int
	writes     int
	compareErr error
	readErr    error
}

func (fake *proposalVaultFake) Read(string, string) ([]byte, string, error) {
	fake.reads++
	return []byte(fake.content), "events/evt-1.md", fake.readErr
}
func (fake *proposalVaultFake) CompareAndSwap(_, _ string, expectedHash, replacement string) (string, error) {
	if fake.compareErr != nil {
		return "", fake.compareErr
	}
	if domain.HashContent("", fake.content) != expectedHash {
		return "", domain.ErrVaultConflict
	}
	fake.writes++
	fake.content = replacement
	return domain.HashContent("", replacement), nil
}

type proposalSnapshotFake struct{ err error }

func (fake proposalSnapshotFake) Put(context.Context, string, string) error { return fake.err }

type proposalSnapshotRecorder struct{ writes int }

func (fake *proposalSnapshotRecorder) Put(context.Context, string, string) error {
	fake.writes++
	return nil
}

type vaultSecurityAuditFake struct{ entries []operationsdomain.AuditEntry }

func (fake *vaultSecurityAuditFake) WriteIndependent(_ context.Context, entry operationsdomain.AuditEntry) error {
	if err := entry.Validate(); err != nil {
		return err
	}
	fake.entries = append(fake.entries, entry)
	return nil
}

func TestProposalApplyRechecksBaseAndCreatesNewRevision(t *testing.T) {
	old := canonicalVaultDocument(t, 7, 0, domain.DocumentEvent, 9, "Event", "old body")
	baseHash := domain.HashContent("", old)
	documents := proposalDocumentsFake{document: domain.Document{ID: 7, Version: 1, RevisionNo: 0, Type: domain.DocumentEvent, VaultPath: "events/evt-1.md", ContentHash: baseHash, Status: domain.DocumentActive, EventID: ptr(9)}}
	store := &proposalStoreFake{}
	service := NewProposalService(documents, store)
	proposal, err := service.Create(context.Background(), 7, 0, baseHash, `{"title":"Event"}`, "new body", "fixture")
	if err != nil {
		t.Fatal(err)
	}
	proposal, err = service.Approve(context.Background(), proposal.ID, proposal.Version)
	if err != nil || proposal.Status != domain.ProposalApproved {
		t.Fatalf("approve = %#v/%v", proposal, err)
	}
	vault := &proposalVaultFake{content: old}
	updated, err := service.Apply(context.Background(), proposal, vault)
	if err != nil || updated.RevisionNo != 1 || updated.Version != 2 || store.updated.ContentHash != domain.HashContent("", vault.content) || store.updated.GeneratedHash != domain.HashContent(`{"title":"Event"}`, "new body") {
		t.Fatalf("apply = %#v/%v, stored=%#v", updated, err, store.updated)
	}
}

func TestProposalApplyDoesNotWriteVaultWhenSnapshotFails(t *testing.T) {
	old := canonicalVaultDocument(t, 7, 0, domain.DocumentEvent, 9, "Event", "old body")
	baseHash := domain.HashContent("", old)
	document := domain.Document{ID: 7, Version: 1, RevisionNo: 0, Type: domain.DocumentEvent, VaultPath: "events/evt-1.md", ContentHash: baseHash, Status: domain.DocumentActive, EventID: ptr(9)}
	store := &proposalStoreFake{}
	service := NewProposalService(proposalDocumentsFake{document: document}, store, proposalSnapshotFake{err: errors.New("snapshot unavailable")})
	proposal := domain.Proposal{ID: 5, Version: 2, DocumentID: document.ID, BaseRevisionNo: 0, BaseHash: baseHash, ProposedFrontmatter: `{"title":"Event"}`, ProposedBody: "new body", Status: domain.ProposalApproved}
	vault := &proposalVaultFake{content: old}
	if _, err := service.Apply(context.Background(), proposal, vault); err == nil {
		t.Fatal("Apply() error = nil")
	}
	if vault.writes != 0 || vault.content != old || store.updated.ID != 0 {
		t.Fatalf("snapshot failure mutated state: vault=%#v document=%#v", vault, store.updated)
	}
}

func TestProposalApplyPersistsConcurrentVaultConflictWithoutOverwriting(t *testing.T) {
	old := canonicalVaultDocument(t, 7, 0, domain.DocumentEvent, 9, "Event", "old body")
	baseHash := domain.HashContent("", old)
	document := domain.Document{ID: 7, Version: 1, RevisionNo: 0, Type: domain.DocumentEvent, VaultPath: "events/evt-1.md", ContentHash: baseHash, Status: domain.DocumentActive, EventID: ptr(9)}
	store := &proposalStoreFake{}
	service := NewProposalService(proposalDocumentsFake{document: document}, store)
	proposal := domain.Proposal{ID: 5, Version: 2, DocumentID: document.ID, BaseRevisionNo: 0, BaseHash: baseHash, ProposedFrontmatter: `{"title":"Event"}`, ProposedBody: "new body", Status: domain.ProposalApproved}
	vault := &proposalVaultFake{content: old, compareErr: domain.ErrVaultConflict}

	if _, err := service.Apply(context.Background(), proposal, vault); !errors.Is(err, sharedrepository.ErrConflict) {
		t.Fatalf("Apply() error = %v, want conflict", err)
	}
	if !store.markedConflict || vault.writes != 0 || vault.content != old || store.updated.ID != 0 {
		t.Fatalf("conflict side effects: store=%#v vault=%#v", store, vault)
	}
}

func TestProposalApplyStopsOnRenamedOrDuplicateStableDocument(t *testing.T) {
	tests := []struct {
		name    string
		content string
		readErr error
	}{
		{name: "renamed file", readErr: os.ErrNotExist},
		{name: "duplicate stable identity", content: canonicalVaultDocument(t, 8, 0, domain.DocumentEvent, 10, "Other", "other body")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			baseHash := domain.HashContent("", test.content)
			document := domain.Document{ID: 7, Version: 1, RevisionNo: 0, Type: domain.DocumentEvent, VaultPath: "events/evt-1.md", ContentHash: baseHash, Status: domain.DocumentActive, EventID: ptr(9)}
			store := &proposalStoreFake{}
			service := NewProposalService(proposalDocumentsFake{document: document}, store)
			proposal := domain.Proposal{ID: 5, Version: 2, DocumentID: document.ID, BaseRevisionNo: 0, BaseHash: baseHash, ProposedFrontmatter: `{"title":"Event"}`, ProposedBody: "new body", Status: domain.ProposalApproved}
			vault := &proposalVaultFake{content: test.content, readErr: test.readErr}

			if _, err := service.Apply(context.Background(), proposal, vault); !errors.Is(err, sharedrepository.ErrConflict) {
				t.Fatalf("Apply() error = %v, want conflict", err)
			}
			if !store.markedConflict || vault.writes != 0 || store.updated.ID != 0 {
				t.Fatalf("conflict side effects: store=%#v vault=%#v", store, vault)
			}
		})
	}
}

func TestProposalApplyRejectsAndAuditsUnsafeMarkdownBeforeVaultIO(t *testing.T) {
	old := canonicalVaultDocument(t, 7, 0, domain.DocumentEvent, 9, "Event", "old body")
	baseHash := domain.HashContent("", old)
	document := domain.Document{ID: 7, Version: 1, RevisionNo: 0, Type: domain.DocumentEvent, VaultPath: "events/evt-1.md", ContentHash: baseHash, Status: domain.DocumentActive, EventID: ptr(9)}
	store := &proposalStoreFake{}
	snapshot := &proposalSnapshotRecorder{}
	audit := &vaultSecurityAuditFake{}
	service := NewProposalService(proposalDocumentsFake{document: document}, store, snapshot).WithVaultSecurityAudit(audit)
	proposal := domain.Proposal{
		ID: 5, Version: 2, DocumentID: document.ID, BaseRevisionNo: 0, BaseHash: baseHash,
		ProposedFrontmatter: `{"title":"Event"}`, ProposedBody: `<script>alert("sentinel")</script>`, Status: domain.ProposalApproved,
	}
	vault := &proposalVaultFake{content: old}

	_, err := service.Apply(context.Background(), proposal, vault)
	if !errors.Is(err, domain.ErrVaultContentUnsafe) || !errors.Is(err, sharedrepository.ErrInvalidInput) {
		t.Fatalf("Apply() error = %v", err)
	}
	if vault.reads != 0 || vault.writes != 0 || snapshot.writes != 0 || store.updated.ID != 0 {
		t.Fatalf("unsafe publish side effects: vault=%#v snapshot=%#v store=%#v", vault, snapshot, store)
	}
	if len(audit.entries) != 1 {
		t.Fatalf("audit entries = %#v", audit.entries)
	}
	entry := audit.entries[0]
	if entry.Action != operationsdomain.ActionKnowledgeProjectionRejected || entry.ResourceType != "knowledge_document" || entry.ResourceID != document.ID || entry.Result != operationsdomain.AuditResultDenied || entry.After["reason_code"] != domain.VaultReasonContentUnsafe || strings.Contains(fmt.Sprint(entry), "sentinel") || strings.Contains(fmt.Sprint(entry), document.VaultPath) {
		t.Fatalf("security audit = %#v", entry)
	}
}

func TestProposalApplyRejectsAndAuditsUnsafePathWithoutHostDisclosure(t *testing.T) {
	old := canonicalVaultDocument(t, 7, 0, domain.DocumentEvent, 9, "Event", "old body")
	baseHash := domain.HashContent("", old)
	sensitivePath := "/Users/private/secret-vault/events/7.md"
	document := domain.Document{ID: 7, Version: 1, RevisionNo: 0, Type: domain.DocumentEvent, VaultPath: sensitivePath, ContentHash: baseHash, Status: domain.DocumentActive, EventID: ptr(9)}
	audit := &vaultSecurityAuditFake{}
	service := NewProposalService(proposalDocumentsFake{document: document}, &proposalStoreFake{}).WithVaultSecurityAudit(audit)
	proposal := domain.Proposal{ID: 5, Version: 2, DocumentID: document.ID, BaseRevisionNo: 0, BaseHash: baseHash, ProposedFrontmatter: `{"title":"Event"}`, ProposedBody: "safe body", Status: domain.ProposalApproved}
	vault := &proposalVaultFake{content: old}

	_, err := service.Apply(context.Background(), proposal, vault)
	if !errors.Is(err, domain.ErrVaultPathInvalid) || !errors.Is(err, sharedrepository.ErrInvalidInput) || strings.Contains(err.Error(), sensitivePath) {
		t.Fatalf("Apply() error = %v", err)
	}
	if vault.reads != 0 || vault.writes != 0 || len(audit.entries) != 1 || audit.entries[0].After["reason_code"] != domain.VaultReasonPathInvalid || strings.Contains(fmt.Sprint(audit.entries[0]), sensitivePath) {
		t.Fatalf("path rejection side effects/audit = vault:%#v audit:%#v", vault, audit.entries)
	}
}

func TestProposalApplyAuditsSymlinkRejection(t *testing.T) {
	old := canonicalVaultDocument(t, 7, 0, domain.DocumentEvent, 9, "Event", "old body")
	baseHash := domain.HashContent("", old)
	document := domain.Document{ID: 7, Version: 1, RevisionNo: 0, Type: domain.DocumentEvent, VaultPath: "events/7.md", ContentHash: baseHash, Status: domain.DocumentActive, EventID: ptr(9)}
	audit := &vaultSecurityAuditFake{}
	service := NewProposalService(proposalDocumentsFake{document: document}, &proposalStoreFake{}).WithVaultSecurityAudit(audit)
	proposal := domain.Proposal{ID: 5, Version: 2, DocumentID: document.ID, BaseRevisionNo: 0, BaseHash: baseHash, ProposedFrontmatter: `{"title":"Event"}`, ProposedBody: "safe body", Status: domain.ProposalApproved}
	vault := &proposalVaultFake{readErr: domain.ErrVaultPathSymlink}

	_, err := service.Apply(context.Background(), proposal, vault)
	if !errors.Is(err, domain.ErrVaultPathSymlink) || len(audit.entries) != 1 || audit.entries[0].After["reason_code"] != domain.VaultReasonPathSymlink || vault.writes != 0 {
		t.Fatalf("symlink rejection = %v audit:%#v vault:%#v", err, audit.entries, vault)
	}
}

func TestDocumentPathPartsRejectsNonCanonicalAndEncodedTraversal(t *testing.T) {
	for _, path := range []string{
		"events/../reports/1.md", "events/./1.md", "events//1.md", `events\\1.md`,
		"events/%2e%2e.md", "events/%252e%252e.md", "/absolute/1.md", `C:\\absolute\\1.md`,
	} {
		document := domain.Document{VaultPath: path}
		if _, _, err := documentPathParts(document); !errors.Is(err, domain.ErrVaultPathInvalid) || strings.Contains(err.Error(), path) {
			t.Errorf("document path %q error = %v", path, err)
		}
	}
}

func canonicalVaultDocument(t *testing.T, documentID, revision int64, documentType domain.DocumentType, sourceID int64, title, generated string) string {
	t.Helper()
	content, err := domain.RenderVaultDocument(domain.VaultDocumentRenderInput{
		DocumentID: documentID, RevisionNo: revision, Type: documentType, SourceID: sourceID,
		Title: title, Generated: generated,
	})
	if err != nil {
		t.Fatal(err)
	}
	return content
}

func ptr(value int64) *int64 { return &value }
