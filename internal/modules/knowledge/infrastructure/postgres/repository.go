package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/StephenQiu30/hotkey-server/internal/modules/knowledge/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
)

type Repository struct{ runtime *database.Runtime }

func NewRepository(runtime *database.Runtime) *Repository { return &Repository{runtime: runtime} }

func (repository *Repository) GetDocument(id int64) (domain.Document, error) {
	if repository == nil || repository.runtime == nil {
		return domain.Document{}, sharedrepository.ErrUnavailable
	}
	if id <= 0 {
		return domain.Document{}, fmt.Errorf("%w: document id", sharedrepository.ErrInvalidInput)
	}
	var document domain.Document
	err := repository.runtime.SQL.QueryRowContext(context.Background(), `
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
