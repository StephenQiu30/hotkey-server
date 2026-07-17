package application

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/StephenQiu30/hotkey-server/internal/modules/knowledge/domain"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
)

type DocumentReader interface {
	GetDocument(id int64) (domain.Document, error)
}
type ProposalStore interface{ SaveProposal(domain.Proposal) error }

type ContextProposalStore interface {
	SaveProposalContext(context.Context, domain.Proposal) error
	UpdateProposalStatus(context.Context, int64, int64, domain.ProposalStatus) (domain.Proposal, error)
	ApplyProposal(context.Context, int64, int64, domain.Document, domain.Revision) (domain.Document, error)
}

type ContextDocumentReader interface {
	GetDocumentContext(context.Context, int64) (domain.Document, error)
}

type Vault interface {
	Read(string, string) ([]byte, string, error)
	WriteAutomatic(string, string, string) (string, error)
}

type ProposalService struct {
	documents DocumentReader
	proposals ProposalStore
}

func NewProposalService(documents DocumentReader, proposals ProposalStore) *ProposalService {
	return &ProposalService{documents: documents, proposals: proposals}
}

func (service *ProposalService) Create(documentID, baseRevision int64, baseHash, frontmatter, body, reason string) (domain.Proposal, error) {
	return service.CreateContext(context.Background(), documentID, baseRevision, baseHash, frontmatter, body, reason)
}

func (service *ProposalService) CreateContext(ctx context.Context, documentID, baseRevision int64, baseHash, frontmatter, body, reason string) (domain.Proposal, error) {
	if service == nil || service.documents == nil || service.proposals == nil || documentID <= 0 || baseRevision < 0 || len(baseHash) != 64 {
		return domain.Proposal{}, fmt.Errorf("invalid proposal service input")
	}
	document, err := service.getDocument(ctx, documentID)
	if err != nil {
		return domain.Proposal{}, err
	}
	if document.RevisionNo != baseRevision || document.ContentHash != baseHash {
		return domain.Proposal{}, fmt.Errorf("knowledge document has changed")
	}
	proposal := domain.Proposal{ID: 1, Version: 1, DocumentID: documentID, BaseRevisionNo: baseRevision, BaseHash: baseHash, ProposedFrontmatter: frontmatter, ProposedBody: body, Reason: reason, Status: domain.ProposalPending}
	if contextStore, ok := service.proposals.(interface {
		SaveProposalContext(context.Context, domain.Proposal) error
	}); ok {
		if err := contextStore.SaveProposalContext(ctx, proposal); err != nil {
			return domain.Proposal{}, err
		}
	} else if err := service.proposals.SaveProposal(proposal); err != nil {
		return domain.Proposal{}, err
	}
	return proposal, nil
}

func (service *ProposalService) Approve(ctx context.Context, proposalID, expectedVersion int64) (domain.Proposal, error) {
	return service.changeStatus(ctx, proposalID, expectedVersion, domain.ProposalApproved)
}

func (service *ProposalService) Reject(ctx context.Context, proposalID, expectedVersion int64) (domain.Proposal, error) {
	return service.changeStatus(ctx, proposalID, expectedVersion, domain.ProposalRejected)
}

func (service *ProposalService) Conflict(ctx context.Context, proposalID, expectedVersion int64) (domain.Proposal, error) {
	return service.changeStatus(ctx, proposalID, expectedVersion, domain.ProposalConflict)
}

func (service *ProposalService) changeStatus(ctx context.Context, proposalID, expectedVersion int64, status domain.ProposalStatus) (domain.Proposal, error) {
	if service == nil || proposalID <= 0 || expectedVersion <= 0 {
		return domain.Proposal{}, sharedrepository.ErrInvalidInput
	}
	store, ok := service.proposals.(interface {
		UpdateProposalStatus(context.Context, int64, int64, domain.ProposalStatus) (domain.Proposal, error)
	})
	if !ok {
		return domain.Proposal{}, sharedrepository.ErrUnavailable
	}
	return store.UpdateProposalStatus(ctx, proposalID, expectedVersion, status)
}

// Apply rechecks both the database revision and the current Vault hash before
// writing. The repository then commits document, proposal and revision rows
// in one transaction, so a process crash cannot leave an applied proposal
// without a durable revision record.
func (service *ProposalService) Apply(ctx context.Context, proposal domain.Proposal, vault Vault) (domain.Document, error) {
	if service == nil || service.documents == nil || vault == nil || proposal.ID <= 0 || proposal.Status != domain.ProposalApproved {
		return domain.Document{}, sharedrepository.ErrInvalidInput
	}
	document, err := service.getDocument(ctx, proposal.DocumentID)
	if err != nil {
		return domain.Document{}, err
	}
	if document.RevisionNo != proposal.BaseRevisionNo || document.ContentHash != proposal.BaseHash {
		return domain.Document{}, sharedrepository.ErrConflict
	}
	kind, key, err := documentPathParts(document)
	if err != nil {
		return domain.Document{}, err
	}
	current, _, err := vault.Read(kind, key)
	if err != nil && !isMissing(err) {
		return domain.Document{}, err
	}
	if len(current) > 0 && domain.HashContent("", string(current)) != proposal.BaseHash {
		return domain.Document{}, sharedrepository.ErrConflict
	}
	if _, err := vault.WriteAutomatic(kind, key, proposal.ProposedBody); err != nil {
		return domain.Document{}, err
	}
	newHash := domain.HashContent(proposal.ProposedFrontmatter, proposal.ProposedBody)
	next := document
	next.Version++
	next.RevisionNo++
	next.ContentHash = newHash
	next.GeneratedHash = newHash
	next.Status = domain.DocumentActive
	revision := domain.Revision{DocumentID: document.ID, RevisionNo: next.RevisionNo, ProposalID: proposal.ID, Source: "proposal", PreviousHash: document.ContentHash, NewHash: newHash, Frontmatter: proposal.ProposedFrontmatter}
	store, ok := service.proposals.(interface {
		ApplyProposal(context.Context, int64, int64, domain.Document, domain.Revision) (domain.Document, error)
	})
	if !ok {
		return domain.Document{}, sharedrepository.ErrUnavailable
	}
	return store.ApplyProposal(ctx, proposal.ID, proposal.Version, next, revision)
}

func (service *ProposalService) getDocument(ctx context.Context, id int64) (domain.Document, error) {
	if reader, ok := service.documents.(ContextDocumentReader); ok {
		return reader.GetDocumentContext(ctx, id)
	}
	return service.documents.GetDocument(id)
}

func documentPathParts(document domain.Document) (string, string, error) {
	clean := filepath.Clean(document.VaultPath)
	if filepath.IsAbs(clean) || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || filepath.Ext(clean) != ".md" {
		return "", "", fmt.Errorf("invalid knowledge document path")
	}
	parts := strings.Split(filepath.ToSlash(clean), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == ".md" {
		return "", "", fmt.Errorf("invalid knowledge document path")
	}
	return parts[0], strings.TrimSuffix(parts[1], ".md"), nil
}

func isMissing(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "no such file") || strings.Contains(err.Error(), "not found"))
}
