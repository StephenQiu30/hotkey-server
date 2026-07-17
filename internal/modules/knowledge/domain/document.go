package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
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

func StablePath(root, kind, key string) (string, error) {
	if strings.TrimSpace(root) == "" || strings.TrimSpace(kind) == "" || strings.TrimSpace(key) == "" || filepath.IsAbs(key) || strings.ContainsAny(key, `/\\`) || key == "." || key == ".." {
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
