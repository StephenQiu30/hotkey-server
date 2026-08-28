package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	AutomaticRegionBegin = "<!-- HOTKEY:AUTO:BEGIN -->"
	AutomaticRegionEnd   = "<!-- HOTKEY:AUTO:END -->"
	HumanRegionBegin     = "<!-- HOTKEY:HUMAN:BEGIN -->"
	HumanRegionEnd       = "<!-- HOTKEY:HUMAN:END -->"
)

type DocumentType string

const (
	DocumentEvent  DocumentType = "event"
	DocumentTopic  DocumentType = "topic"
	DocumentReport DocumentType = "report"
)

type DocumentStatus string

const (
	DocumentPlanned  DocumentStatus = "planned"
	DocumentActive   DocumentStatus = "active"
	DocumentConflict DocumentStatus = "conflict"
	DocumentArchived DocumentStatus = "archived"
	DocumentMissing  DocumentStatus = "missing"
)

type Document struct {
	ID, Version, RevisionNo               int64
	Type                                  DocumentType
	VaultPath, ContentHash, GeneratedHash string
	Status                                DocumentStatus
	EventID, TopicID, ReportID            *int64
}

func (document Document) Validate() error {
	if document.ID <= 0 || document.Version <= 0 || document.RevisionNo < 0 || strings.TrimSpace(document.VaultPath) == "" {
		return fmt.Errorf("invalid knowledge document")
	}
	if document.Type != DocumentEvent && document.Type != DocumentTopic && document.Type != DocumentReport {
		return fmt.Errorf("invalid document type")
	}
	if document.Status != DocumentPlanned && document.Status != DocumentActive && document.Status != DocumentConflict && document.Status != DocumentArchived && document.Status != DocumentMissing {
		return fmt.Errorf("invalid document status")
	}
	return nil
}

type ProposalStatus string

const (
	ProposalPending  ProposalStatus = "pending"
	ProposalApproved ProposalStatus = "approved"
	ProposalRejected ProposalStatus = "rejected"
	ProposalConflict ProposalStatus = "conflict"
	ProposalApplied  ProposalStatus = "applied"
	ProposalFailed   ProposalStatus = "failed"
)

type Proposal struct {
	ID, Version, DocumentID, BaseRevisionNo                          int64
	BaseHash, ProposedFrontmatter, ProposedBody, DiffSummary, Reason string
	Status                                                           ProposalStatus
}

func (proposal Proposal) ValidateCreate() error {
	copy := proposal
	copy.ID = 1
	return copy.Validate()
}

type Revision struct {
	ID, DocumentID, RevisionNo, ProposalID   int64
	Source                                   string
	PreviousHash, NewHash, SnapshotObjectKey string
	Frontmatter                              string
}

func (revision Revision) Validate() error {
	if revision.DocumentID <= 0 || revision.RevisionNo < 0 || strings.TrimSpace(revision.Source) == "" || len(revision.NewHash) != 64 {
		return fmt.Errorf("invalid knowledge revision")
	}
	switch revision.Source {
	case "user", "proposal", "reconcile":
	default:
		return fmt.Errorf("invalid knowledge revision source")
	}
	return nil
}

type ReconciliationIssue struct {
	Path, Kind, ExpectedHash, ActualHash string
}

type ReconciliationReport struct {
	Scanned, Changed, Conflict int
	Issues                     []ReconciliationIssue
}

type VaultFile struct {
	Path string
	Hash string
}

// VaultDocumentRenderInput contains only stable business identity and the
// database-owned automatic body. Human-authored bytes are deliberately not an
// input: a new document starts with an empty human region, while later writes
// must preserve the region already stored in Vault or an immutable revision.
type VaultDocumentRenderInput struct {
	DocumentID int64
	RevisionNo int64
	Type       DocumentType
	SourceID   int64
	Title      string
	Generated  string
}

func (proposal Proposal) Validate() error {
	if proposal.ID <= 0 || proposal.Version <= 0 || proposal.DocumentID <= 0 || proposal.BaseRevisionNo < 0 || len(proposal.BaseHash) != 64 || proposal.Status == "" {
		return fmt.Errorf("invalid knowledge proposal")
	}
	return nil
}

func HashContent(frontmatter, body string) string {
	sum := sha256.Sum256([]byte(frontmatter + "\n---\n" + body))
	return hex.EncodeToString(sum[:])
}

// RenderVaultDocument emits the canonical, deterministic Markdown shape for
// a new human-maintainable projection. The fixed key order is intentional: it
// makes the same PostgreSQL facts byte-for-byte reproducible without relying
// on map iteration or a YAML encoder's formatting choices.
func RenderVaultDocument(input VaultDocumentRenderInput) (string, error) {
	title := strings.TrimSpace(input.Title)
	generated := strings.ReplaceAll(strings.ReplaceAll(input.Generated, "\r\n", "\n"), "\r", "\n")
	if input.DocumentID <= 0 || input.RevisionNo < 0 || input.SourceID <= 0 || !validDocumentType(input.Type) ||
		title == "" || title != input.Title || strings.ContainsAny(title, "\r\n\x00") || strings.ContainsRune(generated, 0) {
		return "", fmt.Errorf("invalid Vault document render input")
	}
	for _, marker := range []string{AutomaticRegionBegin, AutomaticRegionEnd, HumanRegionBegin, HumanRegionEnd} {
		if strings.Contains(generated, marker) {
			return "", fmt.Errorf("generated content must not contain Vault region markers")
		}
	}

	var result strings.Builder
	fmt.Fprintf(&result, "---\nhotkey_schema: 1\nhotkey_document_id: %d\nhotkey_document_type: %s\nhotkey_source_id: %d\nhotkey_revision: %d\n", input.DocumentID, input.Type, input.SourceID, input.RevisionNo)
	fmt.Fprintf(&result, "hotkey_generated_sha256: %s\ntitle: %s\n---\n\n", HashContent("", generated), strconv.QuoteToGraphic(title))
	result.WriteString(AutomaticRegionBegin)
	result.WriteByte('\n')
	result.WriteString(generated)
	if !strings.HasSuffix(generated, "\n") {
		result.WriteByte('\n')
	}
	result.WriteString(AutomaticRegionEnd)
	result.WriteString("\n\n")
	result.WriteString(HumanRegionBegin)
	result.WriteByte('\n')
	result.WriteString(HumanRegionEnd)
	result.WriteByte('\n')
	return result.String(), nil
}

func validDocumentType(documentType DocumentType) bool {
	return documentType == DocumentEvent || documentType == DocumentTopic || documentType == DocumentReport
}

func StablePath(root, kind, key string) (string, error) {
	if strings.TrimSpace(root) == "" || !legacyKnowledgeKind(kind) || strings.TrimSpace(key) == "" || filepath.IsAbs(key) || strings.ContainsAny(key, `/\\`) || key == "." || key == ".." {
		return "", fmt.Errorf("invalid vault path")
	}
	cleanRoot := filepath.Clean(root)
	path := filepath.Join(cleanRoot, kind, key+".md")
	rel, err := filepath.Rel(cleanRoot, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("vault path escapes root")
	}
	return path, nil
}

func legacyKnowledgeKind(kind string) bool {
	return kind == "events" || kind == "topics" || kind == "reports"
}

// MergeAutomaticRegion changes only the generated region. Existing human
// notes outside the markers are retained byte-for-byte. When a document has
// no generated region yet, the region is appended instead of replacing it.
func MergeAutomaticRegion(existing, generated string) (string, error) {
	if strings.Contains(generated, AutomaticRegionBegin) || strings.Contains(generated, AutomaticRegionEnd) {
		return "", fmt.Errorf("generated content must not contain automatic markers")
	}
	auto := AutomaticRegionBegin + "\n" + generated + "\n" + AutomaticRegionEnd
	start := strings.Index(existing, AutomaticRegionBegin)
	end := strings.Index(existing, AutomaticRegionEnd)
	if start >= 0 || end >= 0 {
		if start < 0 || end < start {
			return "", fmt.Errorf("knowledge automatic region is malformed")
		}
		end += len(AutomaticRegionEnd)
		return existing[:start] + auto + existing[end:], nil
	}
	if strings.TrimSpace(existing) == "" {
		return auto + "\n", nil
	}
	return strings.TrimRight(existing, "\n") + "\n\n" + auto + "\n", nil
}
