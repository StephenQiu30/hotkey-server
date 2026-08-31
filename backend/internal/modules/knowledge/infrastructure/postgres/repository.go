package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	databaserepository "github.com/StephenQiu30/hotkey-server/backend/internal/platform/database/repository"
	"github.com/StephenQiu30/hotkey-server/backend/internal/shared/pagination"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

type Repository struct {
	runtime     *database.Runtime
	cursorCodec *pagination.Codec
}

func NewRepository(runtime *database.Runtime) *Repository {
	seed := "knowledge:unavailable"
	if runtime != nil && runtime.Pool != nil {
		seed = "knowledge:" + runtime.Pool.Config().ConnString()
	}
	return NewRepositoryWithCursorCodec(runtime, pagination.NewTestCodec(seed))
}

func NewRepositoryWithCursorCodec(runtime *database.Runtime, codec *pagination.Codec) *Repository {
	return &Repository{runtime: runtime, cursorCodec: codec}
}

func (repository *Repository) GetDocument(ctx context.Context, id int64) (domain.Document, error) {
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
		return domain.Document{}, databaserepository.MapError(err)
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
		return nil, databaserepository.MapError(err)
	}
	defer func() { _ = rows.Close() }()
	documents := make([]domain.Document, 0)
	for rows.Next() {
		var document domain.Document
		if err := rows.Scan(&document.ID, &document.Version, &document.RevisionNo, &document.Type, &document.VaultPath, &document.ContentHash, &document.GeneratedHash, &document.Status, &document.EventID, &document.TopicID, &document.ReportID); err != nil {
			return nil, databaserepository.MapError(err)
		}
		documents = append(documents, document)
	}
	if err := rows.Err(); err != nil {
		return nil, databaserepository.MapError(err)
	}
	return documents, nil
}

const (
	knowledgeListDefaultLimit = 50
	knowledgeListMaximumLimit = 200
	knowledgeCursorVersion    = 1
)

type knowledgeDocumentCursor struct {
	Version    int   `json:"v"`
	SnapshotID int64 `json:"snapshot_id"`
	AfterID    int64 `json:"after_id"`
}

func (repository *Repository) ListDocumentPage(ctx context.Context, query domain.DocumentListQuery) (domain.DocumentPage, error) {
	if repository == nil || repository.runtime == nil || repository.cursorCodec == nil {
		return domain.DocumentPage{}, sharedrepository.ErrUnavailable
	}
	limit, cursor, err := repository.documentPageParameters(ctx, query)
	if err != nil {
		return domain.DocumentPage{}, err
	}
	if cursor.SnapshotID == 0 {
		return domain.DocumentPage{Items: []domain.Document{}}, nil
	}
	rows, err := knowledgeQueryerFor(ctx, repository.runtime).QueryContext(ctx, `
SELECT id, version, revision_no, document_type, vault_path, coalesce(content_hash, ''), coalesce(generated_hash, ''), status, event_id, topic_id, report_id
FROM knowledge_documents
WHERE status <> 'archived' AND id > $1 AND id <= $2
ORDER BY id ASC
LIMIT $3`, cursor.AfterID, cursor.SnapshotID, limit+1)
	if err != nil {
		return domain.DocumentPage{}, databaserepository.MapError(err)
	}
	defer func() { _ = rows.Close() }()
	items := make([]domain.Document, 0, limit+1)
	for rows.Next() {
		var item domain.Document
		if err := rows.Scan(&item.ID, &item.Version, &item.RevisionNo, &item.Type, &item.VaultPath, &item.ContentHash, &item.GeneratedHash, &item.Status, &item.EventID, &item.TopicID, &item.ReportID); err != nil {
			return domain.DocumentPage{}, databaserepository.MapError(err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return domain.DocumentPage{}, databaserepository.MapError(err)
	}
	if len(items) <= limit {
		return domain.DocumentPage{Items: items}, nil
	}
	items = items[:limit]
	cursor.AfterID = items[len(items)-1].ID
	nextCursor, err := repository.cursorCodec.Seal("knowledge_document_list", cursor)
	if err != nil {
		return domain.DocumentPage{}, fmt.Errorf("%w: encode document cursor", sharedrepository.ErrInvalidInput)
	}
	return domain.DocumentPage{Items: items, NextCursor: nextCursor}, nil
}

func (repository *Repository) documentPageParameters(ctx context.Context, query domain.DocumentListQuery) (int, knowledgeDocumentCursor, error) {
	limit := query.Limit
	if limit == 0 {
		limit = knowledgeListDefaultLimit
	}
	if limit < 1 || limit > knowledgeListMaximumLimit {
		return 0, knowledgeDocumentCursor{}, fmt.Errorf("%w: invalid document page size", sharedrepository.ErrInvalidInput)
	}
	cursor := knowledgeDocumentCursor{Version: knowledgeCursorVersion}
	if query.Cursor != "" {
		if err := repository.cursorCodec.Open(query.Cursor, "knowledge_document_list", &cursor); err != nil ||
			cursor.Version != knowledgeCursorVersion || cursor.SnapshotID <= 0 || cursor.AfterID <= 0 || cursor.AfterID >= cursor.SnapshotID {
			return 0, knowledgeDocumentCursor{}, fmt.Errorf("%w: invalid document cursor", sharedrepository.ErrInvalidInput)
		}
		return limit, cursor, nil
	}
	if err := knowledgeQueryerFor(ctx, repository.runtime).QueryRowContext(ctx, `
SELECT COALESCE(MAX(id), 0) FROM knowledge_documents WHERE status <> 'archived'`).Scan(&cursor.SnapshotID); err != nil {
		return 0, knowledgeDocumentCursor{}, databaserepository.MapError(err)
	}
	return limit, cursor, nil
}

// LoadVaultRebuildFact closes the PostgreSQL lineage needed to reproduce the
// current automatic region. A document without its exact immutable Revision
// and applied Proposal is not guessed from hashes or from the Vault.
func (repository *Repository) LoadVaultRebuildFact(ctx context.Context, documentID int64) (application.VaultRebuildFact, error) {
	if repository == nil || repository.runtime == nil {
		return application.VaultRebuildFact{}, sharedrepository.ErrUnavailable
	}
	if documentID <= 0 {
		return application.VaultRebuildFact{}, sharedrepository.ErrInvalidInput
	}
	var fact application.VaultRebuildFact
	var revisionSource, revisionHash, snapshotKey, proposalStatus, proposedBody string
	var revisionFrontmatter, proposedFrontmatter []byte
	var proposalID sql.NullInt64
	err := knowledgeQueryerFor(ctx, repository.runtime).QueryRowContext(ctx, `
SELECT d.id, d.version, d.revision_no, d.document_type, d.vault_path,
       coalesce(d.content_hash, ''), coalesce(d.generated_hash, ''), d.status,
       d.event_id, d.topic_id, d.report_id,
       r.source, coalesce(r.new_hash, ''), coalesce(r.snapshot_object_key, ''), r.frontmatter_snapshot,
       p.id, coalesce(p.status, ''), coalesce(p.proposed_frontmatter, '{}'::jsonb), coalesce(p.proposed_body, '')
FROM knowledge_documents d
JOIN knowledge_revisions r ON r.document_id = d.id AND r.revision_no = d.revision_no
LEFT JOIN knowledge_change_proposals p ON p.id = r.proposal_id
WHERE d.id = $1`, documentID).Scan(
		&fact.Document.ID, &fact.Document.Version, &fact.Document.RevisionNo, &fact.Document.Type, &fact.Document.VaultPath,
		&fact.Document.ContentHash, &fact.Document.GeneratedHash, &fact.Document.Status,
		&fact.Document.EventID, &fact.Document.TopicID, &fact.Document.ReportID,
		&revisionSource, &revisionHash, &snapshotKey, &revisionFrontmatter,
		&proposalID, &proposalStatus, &proposedFrontmatter, &proposedBody)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return application.VaultRebuildFact{}, sharedrepository.ErrUnavailable
		}
		return application.VaultRebuildFact{}, databaserepository.MapError(err)
	}
	if err := fact.Document.Validate(); err != nil || revisionSource != "proposal" || !proposalID.Valid || proposalStatus != string(domain.ProposalApplied) ||
		len(fact.Document.ContentHash) != 64 || revisionHash != fact.Document.ContentHash ||
		fact.Document.GeneratedHash != domain.HashContent(string(proposedFrontmatter), proposedBody) || string(revisionFrontmatter) != string(proposedFrontmatter) {
		return application.VaultRebuildFact{}, domain.ErrVaultConflict
	}
	sourceID, err := knowledgeDocumentSourceID(fact.Document)
	if err != nil {
		return application.VaultRebuildFact{}, err
	}
	frontmatter := struct {
		Title string `json:"title"`
	}{}
	if err := json.Unmarshal(proposedFrontmatter, &frontmatter); err != nil {
		return application.VaultRebuildFact{}, domain.ErrVaultConflict
	}
	title := strings.TrimSpace(frontmatter.Title)
	if title == "" {
		title = fmt.Sprintf("%s-%d", fact.Document.Type, sourceID)
	}
	fact.RenderInput = domain.VaultDocumentRenderInput{
		DocumentID: fact.Document.ID, RevisionNo: fact.Document.RevisionNo, Type: fact.Document.Type,
		SourceID: sourceID, Title: title, Generated: proposedBody,
	}
	fact.SnapshotObjectKey = snapshotKey
	return fact, nil
}

func (repository *Repository) ListProposals(ctx context.Context, status domain.ProposalStatus) ([]domain.Proposal, error) {
	if repository == nil || repository.runtime == nil {
		return nil, sharedrepository.ErrUnavailable
	}
	rows, err := knowledgeQueryerFor(ctx, repository.runtime).QueryContext(ctx, `
SELECT id, version, document_id, base_revision_no, coalesce(base_hash, ''), proposed_frontmatter, proposed_body, diff_summary, reason, status
FROM knowledge_change_proposals
WHERE ($1 = '' OR status = $1)
ORDER BY created_at DESC, id DESC
LIMIT 100`, status)
	if err != nil {
		return nil, databaserepository.MapError(err)
	}
	defer func() { _ = rows.Close() }()
	items := make([]domain.Proposal, 0)
	for rows.Next() {
		var item domain.Proposal
		var frontmatter []byte
		if err := rows.Scan(&item.ID, &item.Version, &item.DocumentID, &item.BaseRevisionNo, &item.BaseHash, &frontmatter, &item.ProposedBody, &item.DiffSummary, &item.Reason, &item.Status); err != nil {
			return nil, databaserepository.MapError(err)
		}
		item.ProposedFrontmatter = string(frontmatter)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, databaserepository.MapError(err)
	}
	return items, nil
}

type knowledgeProposalCursor struct {
	Version        int                   `json:"v"`
	Status         domain.ProposalStatus `json:"status"`
	SnapshotID     int64                 `json:"snapshot_id"`
	AfterCreatedAt time.Time             `json:"after_created_at"`
	AfterID        int64                 `json:"after_id"`
}

type knowledgeProposalRow struct {
	proposal  domain.Proposal
	createdAt time.Time
}

func (repository *Repository) ListProposalPage(ctx context.Context, query domain.ProposalListQuery) (domain.ProposalPage, error) {
	if repository == nil || repository.runtime == nil || repository.cursorCodec == nil {
		return domain.ProposalPage{}, sharedrepository.ErrUnavailable
	}
	limit, cursor, err := repository.proposalPageParameters(ctx, query)
	if err != nil {
		return domain.ProposalPage{}, err
	}
	if cursor.SnapshotID == 0 {
		return domain.ProposalPage{Items: []domain.Proposal{}}, nil
	}
	var afterCreatedAt *time.Time
	if !cursor.AfterCreatedAt.IsZero() {
		afterCreatedAt = &cursor.AfterCreatedAt
	}
	rows, err := knowledgeQueryerFor(ctx, repository.runtime).QueryContext(ctx, `
SELECT id, version, document_id, base_revision_no, coalesce(base_hash, ''), proposed_frontmatter, proposed_body, diff_summary, reason, status, created_at
FROM knowledge_change_proposals
WHERE ($1::text = '' OR status = $1::text)
  AND id <= $2
  AND ($3::timestamptz IS NULL OR (created_at, id) < ($3::timestamptz, $4))
ORDER BY created_at DESC, id DESC
LIMIT $5`, query.Status, cursor.SnapshotID, afterCreatedAt, cursor.AfterID, limit+1)
	if err != nil {
		return domain.ProposalPage{}, databaserepository.MapError(err)
	}
	defer func() { _ = rows.Close() }()
	entries := make([]knowledgeProposalRow, 0, limit+1)
	for rows.Next() {
		var entry knowledgeProposalRow
		var frontmatter []byte
		if err := rows.Scan(&entry.proposal.ID, &entry.proposal.Version, &entry.proposal.DocumentID, &entry.proposal.BaseRevisionNo, &entry.proposal.BaseHash, &frontmatter, &entry.proposal.ProposedBody, &entry.proposal.DiffSummary, &entry.proposal.Reason, &entry.proposal.Status, &entry.createdAt); err != nil {
			return domain.ProposalPage{}, databaserepository.MapError(err)
		}
		entry.proposal.ProposedFrontmatter = string(frontmatter)
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return domain.ProposalPage{}, databaserepository.MapError(err)
	}
	items := make([]domain.Proposal, 0, min(len(entries), limit))
	for index, entry := range entries {
		if index == limit {
			break
		}
		items = append(items, entry.proposal)
	}
	if len(entries) <= limit {
		return domain.ProposalPage{Items: items}, nil
	}
	last := entries[limit-1]
	cursor.AfterCreatedAt = last.createdAt
	cursor.AfterID = last.proposal.ID
	nextCursor, err := repository.cursorCodec.Seal("knowledge_proposal_list", cursor)
	if err != nil {
		return domain.ProposalPage{}, fmt.Errorf("%w: encode proposal cursor", sharedrepository.ErrInvalidInput)
	}
	return domain.ProposalPage{Items: items, NextCursor: nextCursor}, nil
}

func (repository *Repository) proposalPageParameters(ctx context.Context, query domain.ProposalListQuery) (int, knowledgeProposalCursor, error) {
	limit := query.Limit
	if limit == 0 {
		limit = knowledgeListDefaultLimit
	}
	if limit < 1 || limit > knowledgeListMaximumLimit || !query.Status.ValidListFilter() {
		return 0, knowledgeProposalCursor{}, fmt.Errorf("%w: invalid proposal list query", sharedrepository.ErrInvalidInput)
	}
	cursor := knowledgeProposalCursor{Version: knowledgeCursorVersion, Status: query.Status}
	if query.Cursor != "" {
		if err := repository.cursorCodec.Open(query.Cursor, "knowledge_proposal_list", &cursor); err != nil ||
			cursor.Version != knowledgeCursorVersion || cursor.Status != query.Status || cursor.SnapshotID <= 0 ||
			cursor.AfterCreatedAt.IsZero() || cursor.AfterID <= 0 || cursor.AfterID > cursor.SnapshotID {
			return 0, knowledgeProposalCursor{}, fmt.Errorf("%w: invalid proposal cursor", sharedrepository.ErrInvalidInput)
		}
		return limit, cursor, nil
	}
	if err := knowledgeQueryerFor(ctx, repository.runtime).QueryRowContext(ctx, `
SELECT COALESCE(MAX(id), 0)
FROM knowledge_change_proposals
WHERE ($1::text = '' OR status = $1::text)`, query.Status).Scan(&cursor.SnapshotID); err != nil {
		return 0, knowledgeProposalCursor{}, databaserepository.MapError(err)
	}
	return limit, cursor, nil
}

// EnsureReportDocument creates the single knowledge projection assigned to a
// report. It is transaction-aware so publication can roll it back atomically.
func (repository *Repository) EnsureReportDocument(ctx context.Context, reportID int64, vaultPath string, actorID *int64) (domain.Document, error) {
	if repository == nil || repository.runtime == nil || reportID <= 0 || vaultPath == "" {
		return domain.Document{}, sharedrepository.ErrInvalidInput
	}
	emptyHash := domain.HashContent("", "")
	var document domain.Document
	err := knowledgeQueryerFor(ctx, repository.runtime).QueryRowContext(ctx, `
INSERT INTO knowledge_documents (version, document_type, report_id, vault_path, revision_no, content_hash, generated_hash, status, created_by, updated_by)
VALUES (1, 'report', $1, $2, 0, $3, $3, 'planned', $4, $4)
ON CONFLICT (report_id) WHERE report_id IS NOT NULL DO UPDATE SET updated_at = knowledge_documents.updated_at
RETURNING id, version, revision_no, document_type, vault_path, coalesce(content_hash, ''), coalesce(generated_hash, ''), status, event_id, topic_id, report_id`, reportID, vaultPath, emptyHash, actorID).Scan(
		&document.ID, &document.Version, &document.RevisionNo, &document.Type, &document.VaultPath, &document.ContentHash, &document.GeneratedHash, &document.Status, &document.EventID, &document.TopicID, &document.ReportID)
	if err != nil {
		return domain.Document{}, databaserepository.MapError(err)
	}
	return document, nil
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
		return domain.Proposal{}, databaserepository.MapError(err)
	}
	proposal.ProposedFrontmatter = string(frontmatter)
	return proposal, nil
}

func (repository *Repository) SaveDocument(ctx context.Context, document domain.Document) error {
	if repository == nil || repository.runtime == nil {
		return sharedrepository.ErrUnavailable
	}
	if err := document.Validate(); err != nil {
		return fmt.Errorf("%w: %w", sharedrepository.ErrInvalidInput, err)
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
	return databaserepository.MapError(err)
}

func (repository *Repository) SaveProposal(ctx context.Context, proposal domain.Proposal) error {
	return repository.saveProposal(ctx, proposal)
}

func (repository *Repository) CreateProposal(ctx context.Context, proposal domain.Proposal) (domain.Proposal, error) {
	if repository == nil || repository.runtime == nil {
		return domain.Proposal{}, sharedrepository.ErrUnavailable
	}
	if proposal.ID != 0 {
		return domain.Proposal{}, fmt.Errorf("proposal id must be zero for creation")
	}
	if err := proposal.ValidateCreate(); err != nil {
		return domain.Proposal{}, fmt.Errorf("%w: %w", sharedrepository.ErrInvalidInput, err)
	}
	frontmatter := proposal.ProposedFrontmatter
	if frontmatter == "" {
		frontmatter = "{}"
	}
	var raw json.RawMessage
	if err := json.Unmarshal([]byte(frontmatter), &raw); err != nil {
		return domain.Proposal{}, fmt.Errorf("%w: invalid proposal frontmatter: %w", sharedrepository.ErrInvalidInput, err)
	}
	var created domain.Proposal
	var storedFrontmatter []byte
	err := knowledgeQueryerFor(ctx, repository.runtime).QueryRowContext(ctx, `
INSERT INTO knowledge_change_proposals (version, document_id, change_type, base_revision_no, base_hash, proposed_frontmatter, proposed_body, diff_summary, reason, status)
VALUES ($1, $2, 'update', $3, NULLIF($4, ''), $5, $6, $7, $8, $9)
RETURNING id, version, document_id, base_revision_no, coalesce(base_hash, ''), proposed_frontmatter, proposed_body, diff_summary, reason, status`, proposal.Version, proposal.DocumentID, proposal.BaseRevisionNo, proposal.BaseHash, raw, proposal.ProposedBody, proposal.DiffSummary, proposal.Reason, proposal.Status).Scan(
		&created.ID, &created.Version, &created.DocumentID, &created.BaseRevisionNo, &created.BaseHash, &storedFrontmatter, &created.ProposedBody, &created.DiffSummary, &created.Reason, &created.Status)
	if err != nil {
		return domain.Proposal{}, databaserepository.MapError(err)
	}
	created.ProposedFrontmatter = string(storedFrontmatter)
	return created, nil
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
		return domain.Proposal{}, databaserepository.MapError(err)
	}
	proposal.ProposedFrontmatter = string(frontmatter)
	return proposal, nil
}

func (repository *Repository) MarkProposalConflict(ctx context.Context, proposalID, expectedVersion int64) error {
	if repository == nil || repository.runtime == nil || proposalID <= 0 || expectedVersion <= 0 {
		return sharedrepository.ErrInvalidInput
	}
	result, err := knowledgeQueryerFor(ctx, repository.runtime).ExecContext(ctx, `
UPDATE knowledge_change_proposals
SET status = 'conflict', version = version + 1, reviewed_at = now(), updated_at = now()
WHERE id = $1 AND version = $2 AND status IN ('pending','approved')`, proposalID, expectedVersion)
	if err != nil {
		return databaserepository.MapError(err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return databaserepository.MapError(err)
	}
	if changed != 1 {
		return sharedrepository.ErrConflict
	}
	return nil
}

func (repository *Repository) ApplyProposal(ctx context.Context, proposalID, expectedVersion int64, document domain.Document, revision domain.Revision) (domain.Document, error) {
	if repository == nil || repository.runtime == nil {
		return domain.Document{}, sharedrepository.ErrUnavailable
	}
	if err := document.Validate(); err != nil {
		return domain.Document{}, fmt.Errorf("%w: %w", sharedrepository.ErrInvalidInput, err)
	}
	if err := revision.Validate(); err != nil {
		return domain.Document{}, fmt.Errorf("%w: %w", sharedrepository.ErrInvalidInput, err)
	}
	var applied domain.Document
	err := repository.runtime.WithinTransaction(ctx, func(transactionCtx context.Context, transaction database.Transaction) error {
		var currentVersion, currentRevision int64
		var currentHash string
		if err := transaction.SQL.QueryRowContext(transactionCtx, `SELECT version, revision_no, coalesce(content_hash, '') FROM knowledge_documents WHERE id = $1 FOR UPDATE`, document.ID).Scan(&currentVersion, &currentRevision, &currentHash); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return sharedrepository.ErrNotFound
			}
			return databaserepository.MapError(err)
		}
		if currentVersion != document.Version-1 || currentRevision != document.RevisionNo-1 || currentHash != revision.PreviousHash {
			return sharedrepository.ErrConflict
		}
		proposalResult, err := transaction.SQL.ExecContext(transactionCtx, `UPDATE knowledge_change_proposals SET status = 'applied', version = $1, applied_at = now(), updated_at = now() WHERE id = $2 AND version = $3 AND status = 'approved'`, expectedVersion+1, proposalID, expectedVersion)
		if err != nil {
			return databaserepository.MapError(err)
		}
		proposalChanges, err := proposalResult.RowsAffected()
		if err != nil {
			return databaserepository.MapError(err)
		}
		if proposalChanges != 1 {
			return sharedrepository.ErrConflict
		}
		if _, err := transaction.SQL.ExecContext(transactionCtx, `UPDATE knowledge_documents SET version = $1, revision_no = $2, content_hash = $3, generated_hash = $4, status = $5, last_written_at = now(), updated_at = now() WHERE id = $6`, document.Version, document.RevisionNo, document.ContentHash, document.GeneratedHash, document.Status, document.ID); err != nil {
			return databaserepository.MapError(err)
		}
		if _, err := transaction.SQL.ExecContext(transactionCtx, `INSERT INTO knowledge_revisions (document_id, revision_no, source, proposal_id, previous_hash, new_hash, snapshot_object_key, frontmatter_snapshot) VALUES ($1,$2,$3,NULLIF($4,0),NULLIF($5,''),$6,NULLIF($7,''),$8::jsonb)`, revision.DocumentID, revision.RevisionNo, revision.Source, proposalID, revision.PreviousHash, revision.NewHash, revision.SnapshotObjectKey, nullableJSON(revision.Frontmatter)); err != nil {
			return databaserepository.MapError(err)
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
		return fmt.Errorf("%w: %w", sharedrepository.ErrInvalidInput, err)
	}
	frontmatter := proposal.ProposedFrontmatter
	if frontmatter == "" {
		frontmatter = "{}"
	}
	var raw json.RawMessage
	if err := json.Unmarshal([]byte(frontmatter), &raw); err != nil {
		return fmt.Errorf("%w: invalid proposal frontmatter: %w", sharedrepository.ErrInvalidInput, err)
	}
	_, err := repository.runtime.SQL.ExecContext(ctx, `
INSERT INTO knowledge_change_proposals (id, version, document_id, change_type, base_revision_no, base_hash, proposed_frontmatter, proposed_body, diff_summary, reason, status)
VALUES ($1, $2, $3, 'update', $4, NULLIF($5, ''), $6, $7, $8, $9, $10)
ON CONFLICT (id) DO UPDATE SET version = EXCLUDED.version, proposed_frontmatter = EXCLUDED.proposed_frontmatter,
proposed_body = EXCLUDED.proposed_body, diff_summary = EXCLUDED.diff_summary, reason = EXCLUDED.reason, status = EXCLUDED.status,
updated_at = now()`, proposal.ID, proposal.Version, proposal.DocumentID, proposal.BaseRevisionNo, proposal.BaseHash, raw,
		proposal.ProposedBody, proposal.DiffSummary, proposal.Reason, proposal.Status)
	return databaserepository.MapError(err)
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

func knowledgeDocumentSourceID(document domain.Document) (int64, error) {
	if countReferences(document) != 1 {
		return 0, domain.ErrVaultConflict
	}
	for _, sourceID := range []*int64{document.EventID, document.TopicID, document.ReportID} {
		if sourceID != nil && *sourceID > 0 {
			return *sourceID, nil
		}
	}
	return 0, domain.ErrVaultConflict
}

type knowledgeQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func knowledgeQueryerFor(ctx context.Context, runtime *database.Runtime) knowledgeQueryer {
	if transaction, ok := database.TransactionFromContext(ctx); ok {
		return transaction.SQL
	}
	return runtime.SQL
}
