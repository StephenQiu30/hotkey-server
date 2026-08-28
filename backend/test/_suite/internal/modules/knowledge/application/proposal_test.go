package application

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/domain"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

type proposalDocumentsFake struct{ document domain.Document }

func (fake proposalDocumentsFake) GetDocument(int64) (domain.Document, error) {
	return fake.document, nil
}

type proposalStoreFake struct {
	proposal       domain.Proposal
	updated        domain.Document
	markedConflict bool
	applyErr       error
}

func (fake *proposalStoreFake) SaveProposal(proposal domain.Proposal) error {
	fake.proposal = proposal
	return nil
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
	writes     int
	compareErr error
	readErr    error
}

func (fake *proposalVaultFake) Read(string, string) ([]byte, string, error) {
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

func TestProposalApplyRechecksBaseAndCreatesNewRevision(t *testing.T) {
	old := canonicalVaultDocument(t, 7, 0, domain.DocumentEvent, 9, "Event", "old body")
	baseHash := domain.HashContent("", old)
	documents := proposalDocumentsFake{document: domain.Document{ID: 7, Version: 1, RevisionNo: 0, Type: domain.DocumentEvent, VaultPath: "events/evt-1.md", ContentHash: baseHash, Status: domain.DocumentActive, EventID: ptr(9)}}
	store := &proposalStoreFake{}
	service := NewProposalService(documents, store)
	proposal, err := service.CreateContext(context.Background(), 7, 0, baseHash, `{"title":"Event"}`, "new body", "fixture")
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
