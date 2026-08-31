package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/domain"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

type DocumentReader interface {
	GetDocument(context.Context, int64) (domain.Document, error)
}
type ProposalStore interface {
	CreateProposal(context.Context, domain.Proposal) (domain.Proposal, error)
}

type Vault interface {
	Read(string, string) ([]byte, string, error)
	CompareAndSwap(string, string, string, string) (string, error)
}

type ProposalConflictStore interface {
	MarkProposalConflict(context.Context, int64, int64) error
}

type ProposalService struct {
	documents DocumentReader
	proposals ProposalStore
	snapshot  SnapshotStore
	audit     VaultSecurityAuditWriter
}

func (service *ProposalService) WithVaultSecurityAudit(writer VaultSecurityAuditWriter) *ProposalService {
	if service != nil {
		service.audit = writer
	}
	return service
}

func NewProposalService(documents DocumentReader, proposals ProposalStore, snapshots ...SnapshotStore) *ProposalService {
	service := &ProposalService{documents: documents, proposals: proposals}
	if len(snapshots) > 0 {
		service.snapshot = snapshots[0]
	}
	return service
}

func (service *ProposalService) Create(ctx context.Context, documentID, baseRevision int64, baseHash, frontmatter, body, reason string) (domain.Proposal, error) {
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
	proposal := domain.Proposal{Version: 1, DocumentID: documentID, BaseRevisionNo: baseRevision, BaseHash: baseHash, ProposedFrontmatter: frontmatter, ProposedBody: body, Reason: reason, Status: domain.ProposalPending}
	return service.proposals.CreateProposal(ctx, proposal)
}

// ApplyByID rereads the proposal before applying it, which keeps the River
// payload an opaque ID and prevents stale approval data from being trusted.
func (service *ProposalService) ApplyByID(ctx context.Context, proposalID int64, vault Vault) (domain.Document, error) {
	reader, ok := service.proposals.(interface {
		GetProposal(context.Context, int64) (domain.Proposal, error)
	})
	if !ok {
		return domain.Document{}, sharedrepository.ErrUnavailable
	}
	proposal, err := reader.GetProposal(ctx, proposalID)
	if err != nil {
		return domain.Document{}, err
	}
	return service.Apply(ctx, proposal, vault)
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
// publishing through a compare-and-swap fence. The repository subsequently
// commits document, proposal and revision rows together; reconciliation owns
// recovery if the process stops between those two durable boundaries.
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
	sourceID, err := documentSourceID(document)
	if err != nil {
		return domain.Document{}, err
	}
	title, err := proposalVaultTitle(proposal, document.Type, sourceID)
	if err != nil {
		return domain.Document{}, err
	}
	if err := domain.ValidateVaultMarkdown(title + "\n" + proposal.ProposedBody); err != nil {
		return domain.Document{}, service.rejected(ctx, document.ID, err)
	}
	kind, key, err := documentPathParts(document)
	if err != nil {
		return domain.Document{}, service.rejected(ctx, document.ID, err)
	}
	current, _, err := vault.Read(kind, key)
	if err != nil && !isMissing(err) {
		return domain.Document{}, service.rejected(ctx, document.ID, err)
	}
	if len(current) > 0 && !vaultContentMatchesBase(string(current), proposal.BaseHash) {
		return domain.Document{}, service.persistConflict(ctx, proposal)
	}
	renderInput := domain.VaultDocumentRenderInput{
		DocumentID: document.ID, RevisionNo: document.RevisionNo + 1, Type: document.Type,
		SourceID: sourceID, Title: title, Generated: proposal.ProposedBody,
	}
	var replacement string
	if len(current) == 0 {
		if document.Status != domain.DocumentPlanned || document.RevisionNo != 0 || document.ContentHash != domain.HashContent("", "") {
			return domain.Document{}, service.persistConflict(ctx, proposal)
		}
		replacement, err = domain.RenderVaultDocument(renderInput)
	} else {
		replacement, err = domain.UpdateVaultDocument(string(current), renderInput)
	}
	if err != nil {
		if domain.VaultRejectionReason(err) != "" {
			return domain.Document{}, service.rejected(ctx, document.ID, err)
		}
		return domain.Document{}, service.persistConflict(ctx, proposal)
	}
	snapshotKey := ""
	if service.snapshot != nil {
		snapshotKey = fmt.Sprintf("knowledge/v1/%d/%d.md", document.ID, document.RevisionNo+1)
		if err := service.snapshot.Put(ctx, snapshotKey, replacement); err != nil {
			return domain.Document{}, err
		}
	}
	contentHash, err := vault.CompareAndSwap(kind, key, proposal.BaseHash, replacement)
	if errors.Is(err, domain.ErrVaultConflict) {
		return domain.Document{}, service.persistConflict(ctx, proposal)
	}
	if err != nil {
		return domain.Document{}, service.rejected(ctx, document.ID, err)
	}
	if contentHash != domain.HashContent("", replacement) {
		return domain.Document{}, sharedrepository.ErrUnavailable
	}
	generatedHash := domain.HashContent(proposal.ProposedFrontmatter, proposal.ProposedBody)
	next := document
	next.Version++
	next.RevisionNo++
	next.ContentHash = contentHash
	next.GeneratedHash = generatedHash
	next.Status = domain.DocumentActive
	revision := domain.Revision{DocumentID: document.ID, RevisionNo: next.RevisionNo, ProposalID: proposal.ID, Source: "proposal", PreviousHash: document.ContentHash, NewHash: contentHash, Frontmatter: proposal.ProposedFrontmatter}
	revision.SnapshotObjectKey = snapshotKey
	store, ok := service.proposals.(interface {
		ApplyProposal(context.Context, int64, int64, domain.Document, domain.Revision) (domain.Document, error)
	})
	if !ok {
		return domain.Document{}, sharedrepository.ErrUnavailable
	}
	return store.ApplyProposal(ctx, proposal.ID, proposal.Version, next, revision)
}

func (service *ProposalService) rejected(ctx context.Context, documentID int64, err error) error {
	if reason := domain.VaultRejectionReason(err); reason != "" {
		if auditErr := writeVaultSecurityRejection(ctx, service.audit, documentID, err); auditErr != nil {
			return auditErr
		}
		return fmt.Errorf("%w: %w", sharedrepository.ErrInvalidInput, err)
	}
	return err
}

func (service *ProposalService) persistConflict(ctx context.Context, proposal domain.Proposal) error {
	store, ok := service.proposals.(ProposalConflictStore)
	if !ok {
		return sharedrepository.ErrConflict
	}
	if err := store.MarkProposalConflict(ctx, proposal.ID, proposal.Version); err != nil && !errors.Is(err, sharedrepository.ErrConflict) {
		return err
	}
	return sharedrepository.ErrConflict
}

func documentSourceID(document domain.Document) (int64, error) {
	for _, sourceID := range []*int64{document.EventID, document.TopicID, document.ReportID} {
		if sourceID != nil && *sourceID > 0 {
			return *sourceID, nil
		}
	}
	return 0, sharedrepository.ErrInvalidInput
}

func proposalVaultTitle(proposal domain.Proposal, documentType domain.DocumentType, sourceID int64) (string, error) {
	frontmatter := struct {
		Title string `json:"title"`
	}{}
	if proposal.ProposedFrontmatter != "" {
		if err := json.Unmarshal([]byte(proposal.ProposedFrontmatter), &frontmatter); err != nil {
			return "", sharedrepository.ErrInvalidInput
		}
	}
	if strings.TrimSpace(frontmatter.Title) == "" {
		return fmt.Sprintf("%s-%d", documentType, sourceID), nil
	}
	return frontmatter.Title, nil
}

func vaultContentMatchesBase(content, baseHash string) bool {
	if domain.HashContent("", content) == baseHash {
		return true
	}
	start := strings.Index(content, domain.AutomaticRegionBegin)
	end := strings.Index(content, domain.AutomaticRegionEnd)
	if start < 0 || end <= start {
		return false
	}
	body := strings.TrimPrefix(content[start+len(domain.AutomaticRegionBegin):end], "\n")
	body = strings.TrimSuffix(body, "\n")
	return domain.HashContent("", body) == baseHash
}

func (service *ProposalService) getDocument(ctx context.Context, id int64) (domain.Document, error) {
	return service.documents.GetDocument(ctx, id)
}

func documentPathParts(document domain.Document) (string, string, error) {
	raw := document.VaultPath
	clean := filepath.Clean(raw)
	if raw == "" || strings.ContainsRune(raw, '\\') || filepath.ToSlash(clean) != raw || filepath.IsAbs(clean) || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || filepath.Ext(clean) != ".md" {
		return "", "", domain.ErrVaultPathInvalid
	}
	parts := strings.Split(filepath.ToSlash(clean), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == ".md" {
		return "", "", domain.ErrVaultPathInvalid
	}
	kind, key := parts[0], strings.TrimSuffix(parts[1], ".md")
	if err := domain.ValidateVaultLocation(kind, key); err != nil {
		return "", "", err
	}
	return kind, key, nil
}

func isMissing(err error) bool {
	return err != nil && (errors.Is(err, os.ErrNotExist) || strings.Contains(err.Error(), "no such file") || strings.Contains(err.Error(), "not found"))
}
