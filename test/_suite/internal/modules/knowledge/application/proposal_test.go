package application

import (
	"context"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/modules/knowledge/domain"
)

type proposalDocumentsFake struct{ document domain.Document }

func (fake proposalDocumentsFake) GetDocument(int64) (domain.Document, error) {
	return fake.document, nil
}

type proposalStoreFake struct {
	proposal domain.Proposal
	updated  domain.Document
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
	return document, nil
}

type proposalVaultFake struct{ content string }

func (fake *proposalVaultFake) Read(string, string) ([]byte, string, error) {
	return []byte(fake.content), "events/evt-1.md", nil
}
func (fake *proposalVaultFake) WriteAutomatic(_, _ string, generated string) (string, error) {
	fake.content = generated
	return "events/evt-1.md", nil
}

func TestProposalApplyRechecksBaseAndCreatesNewRevision(t *testing.T) {
	old := "old body"
	baseHash := domain.HashContent("", old)
	documents := proposalDocumentsFake{document: domain.Document{ID: 7, Version: 1, RevisionNo: 0, Type: domain.DocumentEvent, VaultPath: "events/evt-1.md", ContentHash: baseHash, Status: domain.DocumentActive, EventID: ptr(9)}}
	store := &proposalStoreFake{}
	service := NewProposalService(documents, store)
	proposal, err := service.CreateContext(context.Background(), 7, 0, baseHash, `{}`, "new body", "fixture")
	if err != nil {
		t.Fatal(err)
	}
	proposal, err = service.Approve(context.Background(), proposal.ID, proposal.Version)
	if err != nil || proposal.Status != domain.ProposalApproved {
		t.Fatalf("approve = %#v/%v", proposal, err)
	}
	vault := &proposalVaultFake{content: old}
	updated, err := service.Apply(context.Background(), proposal, vault)
	if err != nil || updated.RevisionNo != 1 || updated.Version != 2 || store.updated.ContentHash == baseHash {
		t.Fatalf("apply = %#v/%v, stored=%#v", updated, err, store.updated)
	}
}

func ptr(value int64) *int64 { return &value }
