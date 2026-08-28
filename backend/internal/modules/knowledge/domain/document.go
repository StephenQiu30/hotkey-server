package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

var (
	ErrVaultConflict               = errors.New("Vault document conflict")
	ErrVaultHumanRegionUnavailable = errors.New("Vault human region unavailable")
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

type VaultRecoverySource string

const (
	VaultRecoveryCurrent  VaultRecoverySource = "current_vault"
	VaultRecoveryRevision VaultRecoverySource = "knowledge_revision"
	VaultRecoveryBackup   VaultRecoverySource = "backup"
)

// VaultRecoverySources are ordered protected copies of the last committed
// human-maintainable file. The current Vault always wins over a Revision,
// which always wins over an explicit backup. A present but conflicting source
// is never skipped in favour of an older copy.
type VaultRecoverySources struct {
	ExpectedHash string
	Current      string
	Revision     string
	Backup       string
}

type VaultRecoveryResult struct {
	Content string
	Source  VaultRecoverySource
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
	if err := ValidateVaultMarkdown(title + "\n" + generated); err != nil {
		return "", err
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

// UpdateVaultDocument rebuilds only system-owned bytes and carries the exact
// existing human region forward. Identity and revision checks make a stale,
// renamed or malformed file a conflict instead of an overwrite candidate.
func UpdateVaultDocument(existing string, input VaultDocumentRenderInput) (string, error) {
	if err := ValidateVaultMarkdown(existing); err != nil {
		return "", err
	}
	identity, human, err := parseVaultDocument(existing)
	if err != nil {
		return "", err
	}
	if identity.documentID != input.DocumentID || identity.documentType != input.Type || identity.sourceID != input.SourceID {
		return "", fmt.Errorf("Vault document identity conflict")
	}
	if input.RevisionNo != identity.revisionNo && input.RevisionNo != identity.revisionNo+1 {
		return "", fmt.Errorf("Vault document revision conflict")
	}
	candidate, err := RenderVaultDocument(input)
	if err != nil {
		return "", err
	}
	emptyHuman := HumanRegionBegin + "\n" + HumanRegionEnd
	candidate = strings.Replace(candidate, emptyHuman, human, 1)
	if input.RevisionNo == identity.revisionNo {
		if candidate == existing {
			return existing, nil
		}
		return "", fmt.Errorf("Vault document revision content conflict")
	}
	return candidate, nil
}

// RecoverVaultDocument rebuilds only database-owned bytes while carrying the
// human region from a verified protected source. It deliberately cannot
// create an empty human region when every protected copy is missing.
func RecoverVaultDocument(sources VaultRecoverySources, input VaultDocumentRenderInput) (VaultRecoveryResult, error) {
	if len(sources.ExpectedHash) != 64 {
		return VaultRecoveryResult{}, fmt.Errorf("%w: invalid expected hash", ErrVaultConflict)
	}
	candidates := []struct {
		content string
		source  VaultRecoverySource
	}{
		{content: sources.Current, source: VaultRecoveryCurrent},
		{content: sources.Revision, source: VaultRecoveryRevision},
		{content: sources.Backup, source: VaultRecoveryBackup},
	}
	for _, candidate := range candidates {
		if candidate.content == "" {
			continue
		}
		if HashContent("", candidate.content) != sources.ExpectedHash {
			return VaultRecoveryResult{}, fmt.Errorf("%w: protected source hash changed", ErrVaultConflict)
		}
		content, err := UpdateVaultDocument(candidate.content, input)
		if err != nil {
			if errors.Is(err, ErrVaultContentUnsafe) {
				return VaultRecoveryResult{}, err
			}
			return VaultRecoveryResult{}, fmt.Errorf("%w: protected source identity changed", ErrVaultConflict)
		}
		return VaultRecoveryResult{Content: content, Source: candidate.source}, nil
	}
	return VaultRecoveryResult{}, ErrVaultHumanRegionUnavailable
}

// VaultHumanRegionSHA256 returns a non-reversible recovery invariant for the
// exact validated human-maintained region, including its boundary markers and
// whitespace. Callers can prove that a rebuild preserved human bytes without
// putting those bytes or an absolute Vault path into logs or evidence files.
func VaultHumanRegionSHA256(content string) (string, error) {
	if err := ValidateVaultMarkdown(content); err != nil {
		return "", err
	}
	_, human, err := parseVaultDocument(content)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256([]byte(human))
	return hex.EncodeToString(digest[:]), nil
}

type vaultDocumentIdentity struct {
	documentID   int64
	documentType DocumentType
	sourceID     int64
	revisionNo   int64
}

func parseVaultDocument(content string) (vaultDocumentIdentity, string, error) {
	if strings.Count(content, AutomaticRegionBegin) != 1 || strings.Count(content, AutomaticRegionEnd) != 1 ||
		strings.Count(content, HumanRegionBegin) != 1 || strings.Count(content, HumanRegionEnd) != 1 {
		return vaultDocumentIdentity{}, "", fmt.Errorf("Vault document region conflict")
	}
	automaticStart := strings.Index(content, AutomaticRegionBegin)
	automaticEnd := strings.Index(content, AutomaticRegionEnd)
	humanStart := strings.Index(content, HumanRegionBegin)
	humanEnd := strings.Index(content, HumanRegionEnd)
	if automaticStart < 0 || automaticEnd <= automaticStart || humanStart <= automaticEnd || humanEnd <= humanStart {
		return vaultDocumentIdentity{}, "", fmt.Errorf("Vault document regions overlap")
	}
	humanEnd += len(HumanRegionEnd)

	if !strings.HasPrefix(content, "---\n") {
		return vaultDocumentIdentity{}, "", fmt.Errorf("Vault document frontmatter is missing")
	}
	frontmatterEnd := strings.Index(content[len("---\n"):], "\n---\n")
	if frontmatterEnd < 0 {
		return vaultDocumentIdentity{}, "", fmt.Errorf("Vault document frontmatter is malformed")
	}
	frontmatterEnd += len("---\n")
	if frontmatterEnd >= automaticStart {
		return vaultDocumentIdentity{}, "", fmt.Errorf("Vault document frontmatter overlaps automatic region")
	}
	values := make(map[string]string)
	for _, line := range strings.Split(content[len("---\n"):frontmatterEnd], "\n") {
		key, value, found := strings.Cut(line, ":")
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if !found || key == "" || value == "" {
			return vaultDocumentIdentity{}, "", fmt.Errorf("Vault document frontmatter is malformed")
		}
		if _, duplicated := values[key]; duplicated {
			return vaultDocumentIdentity{}, "", fmt.Errorf("Vault document frontmatter key is duplicated")
		}
		values[key] = value
	}
	if values["hotkey_schema"] != "1" || len(values["hotkey_generated_sha256"]) != 64 {
		return vaultDocumentIdentity{}, "", fmt.Errorf("Vault document frontmatter version is unsupported")
	}
	documentID, documentErr := strconv.ParseInt(values["hotkey_document_id"], 10, 64)
	sourceID, sourceErr := strconv.ParseInt(values["hotkey_source_id"], 10, 64)
	revisionNo, revisionErr := strconv.ParseInt(values["hotkey_revision"], 10, 64)
	documentType := DocumentType(values["hotkey_document_type"])
	if documentErr != nil || sourceErr != nil || revisionErr != nil || documentID <= 0 || sourceID <= 0 || revisionNo < 0 || !validDocumentType(documentType) {
		return vaultDocumentIdentity{}, "", fmt.Errorf("Vault document identity is invalid")
	}
	return vaultDocumentIdentity{
		documentID: documentID, documentType: documentType, sourceID: sourceID, revisionNo: revisionNo,
	}, content[humanStart:humanEnd], nil
}

func validDocumentType(documentType DocumentType) bool {
	return documentType == DocumentEvent || documentType == DocumentTopic || documentType == DocumentReport
}

func StablePath(root, kind, key string) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "", ErrVaultPathInvalid
	}
	if err := ValidateVaultLocation(kind, key); err != nil {
		return "", err
	}
	cleanRoot := filepath.Clean(root)
	path := filepath.Join(cleanRoot, kind, key+".md")
	rel, err := filepath.Rel(cleanRoot, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", ErrVaultPathInvalid
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
