package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/StephenQiu30/hotkey-server/internal/modules/knowledge/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
)

type Repository struct{ runtime *database.Runtime }

func NewRepository(runtime *database.Runtime) *Repository { return &Repository{runtime: runtime} }

func (repository *Repository) GetDocument(id int64) (domain.Document, error) {
	return repository.GetDocumentContext(context.Background(), id)
}

func (repository *Repository) GetDocumentContext(ctx context.Context, id int64) (domain.Document, error) {
	if repository == nil || repository.runtime == nil {
		return domain.Document{}, sharedrepository.ErrUnavailable
	}
	if id <= 0 {
		return domain.Document{}, fmt.Errorf("%w: document id", sharedrepository.ErrInvalidInput)
	}
	var document domain.Document
	err := knowledgeQueryerFor(ctx, repository.runtime).QueryRowContext(ctx, `
SELECT id, version, revision_no, document_type, vault_path, coalesce(content_hash, ''), coalesce(generated_hash, ''), status, event_id, topic_id, report_id
FROM knowledge_documents WHERE id = $1`, id).Scan(&document.ID, &document.Version, &document.RevisionNo, &document.Type, &document.VaultPath, &document.ContentHash, &document.GeneratedHash, &document.Status, &document.EventID, &document.TopicID, &document.ReportID)
	if err != nil {
		if err == sql.ErrNoRows {
			return domain.Document{}, sharedrepository.ErrNotFound
		}
		return domain.Document{}, sharedrepository.MapError(err)
	}
	return document, nil
}

func (repository *Repository) ListDocuments(ctx context.Context) ([]domain.Document, error) {
	if repository == nil || repository.runtime == nil {
		return nil, sharedrepository.ErrUnavailable
	}
	rows, err := knowledgeQueryerFor(ctx, repository.runtime).QueryContext(ctx, `
SELECT id, version, revision_no, document_type, vault_path, coalesce(content_hash, ''), coalesce(generated_hash, ''), status, event_id, topic_id, report_id
FROM knowledge_documents WHERE status <> 'archived' ORDER BY id`)
	if err != nil {
		return nil, sharedrepository.MapError(err)
	}
	defer rows.Close()
	documents := make([]domain.Document, 0)
	for rows.Next() {
		var document domain.Document
		if err := rows.Scan(&document.ID, &document.Version, &document.RevisionNo, &document.Type, &document.VaultPath, &document.ContentHash, &document.GeneratedHash, &document.Status, &document.EventID, &document.TopicID, &document.ReportID); err != nil {
			return nil, sharedrepository.MapError(err)
		}
		documents = append(documents, document)
	}
	if err := rows.Err(); err != nil {
		return nil, sharedrepository.MapError(err)
	}
	return documents, nil
}

func (repository *Repository) GetProposal(ctx context.Context, id int64) (domain.Proposal, error) {
	if repository == nil || repository.runtime == nil || id <= 0 {
		return domain.Proposal{}, sharedrepository.ErrInvalidInput
	}
	var proposal domain.Proposal
	var frontmatter []byte
	err := knowledgeQueryerFor(ctx, repository.runtime).QueryRowContext(ctx, `
SELECT id, version, document_id, base_revision_no, coalesce(base_hash, ''), proposed_frontmatter, proposed_body, diff_summary, reason, status
FROM knowledge_change_proposals WHERE id = $1`, id).Scan(&proposal.ID, &proposal.Version, &proposal.DocumentID, &proposal.BaseRevisionNo, &proposal.BaseHash, &frontmatter, &proposal.ProposedBody, &proposal.DiffSummary, &proposal.Reason, &proposal.Status)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Proposal{}, sharedrepository.ErrNotFound
		}
		return domain.Proposal{}, sharedrepository.MapError(err)
	}
	proposal.ProposedFrontmatter = string(frontmatter)
	return proposal, nil
}

func (repository *Repository) SaveDocument(ctx context.Context, document domain.Document) error {
	if repository == nil || repository.runtime == nil {
		return sharedrepository.ErrUnavailable
	}
	if err := document.Validate(); err != nil {
		return fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}
	if countReferences(document) != 1 {
		return fmt.Errorf("%w: document requires exactly one source reference", sharedrepository.ErrInvalidInput)
	}
	_, err := repository.runtime.SQL.ExecContext(ctx, `
INSERT INTO knowledge_documents (id, version, document_type, event_id, topic_id, report_id, vault_path, revision_no, content_hash, generated_hash, status)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NULLIF($9, ''), NULLIF($10, ''), $11)
ON CONFLICT (id) DO UPDATE SET version = EXCLUDED.version, vault_path = EXCLUDED.vault_path,
revision_no = EXCLUDED.revision_no, content_hash = EXCLUDED.content_hash, generated_hash = EXCLUDED.generated_hash,
status = EXCLUDED.status, updated_at = now()`, document.ID, document.Version, document.Type, document.EventID, document.TopicID,
		document.ReportID, document.VaultPath, document.RevisionNo, document.ContentHash, document.GeneratedHash, document.Status)
	return sharedrepository.MapError(err)
}

func (repository *Repository) SaveProposal(proposal domain.Proposal) error {
	return repository.saveProposal(context.Background(), proposal)
}

func (repository *Repository) SaveProposalContext(ctx context.Context, proposal domain.Proposal) error {
	return repository.saveProposal(ctx, proposal)
}

func (repository *Repository) UpdateProposalStatus(ctx context.Context, proposalID, expectedVersion int64, status domain.ProposalStatus) (domain.Proposal, error) {
	if repository == nil || repository.runtime == nil || proposalID <= 0 || expectedVersion <= 0 {
		return domain.Proposal{}, sharedrepository.ErrInvalidInput
	}
	if status != domain.ProposalApproved && status != domain.ProposalRejected && status != domain.ProposalConflict {
		return domain.Proposal{}, sharedrepository.ErrInvalidInput
	}
	var proposal domain.Proposal
	var frontmatter []byte
	err := knowledgeQueryerFor(ctx, repository.runtime).QueryRowContext(ctx, `
UPDATE knowledge_change_proposals
SET status = $1, version = version + 1, reviewed_at = now(), updated_at = now()
WHERE id = $2 AND version = $3 AND status = 'pending'
RETURNING id, version, document_id, base_revision_no, coalesce(base_hash, ''), proposed_frontmatter, proposed_body, diff_summary, reason, status`, status, proposalID, expectedVersion).Scan(
		&proposal.ID, &proposal.Version, &proposal.DocumentID, &proposal.BaseRevisionNo, &proposal.BaseHash, &frontmatter, &proposal.ProposedBody, &proposal.DiffSummary, &proposal.Reason, &proposal.Status)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Proposal{}, sharedrepository.ErrConflict
		}
		return domain.Proposal{}, sharedrepository.MapError(err)
	}
	proposal.ProposedFrontmatter = string(frontmatter)
	return proposal, nil
}

func (repository *Repository) ApplyProposal(ctx context.Context, proposalID, expectedVersion int64, document domain.Document, revision domain.Revision) (domain.Document, error) {
	if repository == nil || repository.runtime == nil {
		return domain.Document{}, sharedrepository.ErrUnavailable
	}
	if err := document.Validate(); err != nil {
		return domain.Document{}, fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}
	if err := revision.Validate(); err != nil {
		return domain.Document{}, fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}
	var applied domain.Document
	err := repository.runtime.WithinTransaction(ctx, func(transactionCtx context.Context, transaction database.Transaction) error {
		var currentVersion, currentRevision int64
		var currentHash string
		if err := transaction.SQL.QueryRowContext(transactionCtx, `SELECT version, revision_no, coalesce(content_hash, '') FROM knowledge_documents WHERE id = $1 FOR UPDATE`, document.ID).Scan(&currentVersion, &currentRevision, &currentHash); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return sharedrepository.ErrNotFound
			}
			return sharedrepository.MapError(err)
		}
		if currentVersion != document.Version-1 || currentRevision != document.RevisionNo-1 || currentHash != revision.PreviousHash {
			return sharedrepository.ErrConflict
		}
		if _, err := transaction.SQL.ExecContext(transactionCtx, `UPDATE knowledge_documents SET version = $1, revision_no = $2, content_hash = $3, generated_hash = $4, status = $5, last_written_at = now(), updated_at = now() WHERE id = $6`, document.Version, document.RevisionNo, document.ContentHash, document.GeneratedHash, document.Status, document.ID); err != nil {
			return sharedrepository.MapError(err)
		}
		if _, err := transaction.SQL.ExecContext(transactionCtx, `UPDATE knowledge_change_proposals SET status = 'applied', version = $1, applied_at = now(), updated_at = now() WHERE id = $2 AND version = $3 AND status = 'approved'`, expectedVersion+1, proposalID, expectedVersion); err != nil {
			return sharedrepository.MapError(err)
		}
		if _, err := transaction.SQL.ExecContext(transactionCtx, `INSERT INTO knowledge_revisions (document_id, revision_no, source, proposal_id, previous_hash, new_hash, snapshot_object_key, frontmatter_snapshot) VALUES ($1,$2,$3,NULLIF($4,0),NULLIF($5,''),$6,NULLIF($7,''),$8::jsonb)`, revision.DocumentID, revision.RevisionNo, revision.Source, proposalID, revision.PreviousHash, revision.NewHash, revision.SnapshotObjectKey, nullableJSON(revision.Frontmatter)); err != nil {
			return sharedrepository.MapError(err)
		}
		applied = document
		return nil
	})
	return applied, err
}

func nullableJSON(value string) string {
	if value == "" {
		return "{}"
	}
	return value
}

func (repository *Repository) saveProposal(ctx context.Context, proposal domain.Proposal) error {
	if repository == nil || repository.runtime == nil {
		return sharedrepository.ErrUnavailable
	}
	if err := proposal.Validate(); err != nil {
		return fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}
	frontmatter := proposal.ProposedFrontmatter
	if frontmatter == "" {
		frontmatter = "{}"
	}
	var raw json.RawMessage
	if err := json.Unmarshal([]byte(frontmatter), &raw); err != nil {
		return fmt.Errorf("%w: invalid proposal frontmatter: %v", sharedrepository.ErrInvalidInput, err)
	}
	_, err := repository.runtime.SQL.ExecContext(ctx, `
INSERT INTO knowledge_change_proposals (id, version, document_id, change_type, base_revision_no, base_hash, proposed_frontmatter, proposed_body, diff_summary, reason, status)
VALUES ($1, $2, $3, 'update', $4, NULLIF($5, ''), $6, $7, $8, $9, $10)
ON CONFLICT (id) DO UPDATE SET version = EXCLUDED.version, proposed_frontmatter = EXCLUDED.proposed_frontmatter,
proposed_body = EXCLUDED.proposed_body, diff_summary = EXCLUDED.diff_summary, reason = EXCLUDED.reason, status = EXCLUDED.status,
updated_at = now()`, proposal.ID, proposal.Version, proposal.DocumentID, proposal.BaseRevisionNo, proposal.BaseHash, raw,
		proposal.ProposedBody, proposal.DiffSummary, proposal.Reason, proposal.Status)
	return sharedrepository.MapError(err)
}

func countReferences(document domain.Document) int {
	count := 0
	if document.EventID != nil {
		count++
	}
	if document.TopicID != nil {
		count++
	}
	if document.ReportID != nil {
		count++
	}
	return count
}

type knowledgeQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func knowledgeQueryerFor(ctx context.Context, runtime *database.Runtime) knowledgeQueryer {
	if transaction, ok := database.TransactionFromContext(ctx); ok {
		return transaction.SQL
	}
	return runtime.SQL
}
