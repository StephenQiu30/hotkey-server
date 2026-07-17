package application

import (
	"fmt"
	"github.com/StephenQiu30/hotkey-server/internal/modules/knowledge/domain"
)

type DocumentReader interface {
	GetDocument(id int64) (domain.Document, error)
}
type ProposalStore interface{ SaveProposal(domain.Proposal) error }

type ProposalService struct {
	documents DocumentReader
	proposals ProposalStore
}

func NewProposalService(documents DocumentReader, proposals ProposalStore) *ProposalService {
	return &ProposalService{documents: documents, proposals: proposals}
}

func (service *ProposalService) Create(documentID, baseRevision int64, baseHash, frontmatter, body, reason string) (domain.Proposal, error) {
	if service == nil || service.documents == nil || service.proposals == nil || documentID <= 0 || baseRevision < 0 || len(baseHash) != 64 {
		return domain.Proposal{}, fmt.Errorf("invalid proposal service input")
	}
	document, err := service.documents.GetDocument(documentID)
	if err != nil {
		return domain.Proposal{}, err
	}
	if document.RevisionNo != baseRevision || document.ContentHash != baseHash {
		return domain.Proposal{}, fmt.Errorf("knowledge document has changed")
	}
	proposal := domain.Proposal{ID: 1, Version: 1, DocumentID: documentID, BaseRevisionNo: baseRevision, BaseHash: baseHash, ProposedFrontmatter: frontmatter, ProposedBody: body, Reason: reason, Status: domain.ProposalPending}
	if err := service.proposals.SaveProposal(proposal); err != nil {
		return domain.Proposal{}, err
	}
	return proposal, nil
}
