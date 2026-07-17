package application

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/StephenQiu30/hotkey-server/internal/modules/knowledge/domain"
)

type DocumentLister interface {
	ListDocuments(context.Context) ([]domain.Document, error)
}

type FileLister interface {
	ListFiles() ([]domain.VaultFile, error)
}

// Reconciler compares the durable document projection with the Vault scan.
// It reports conflicts without silently overwriting either side; a separate
// approved proposal is required to repair a mismatch.
type Reconciler struct {
	documents DocumentLister
	vault     FileLister
}

func NewReconciler(documents DocumentLister, vault FileLister) *Reconciler {
	return &Reconciler{documents: documents, vault: vault}
}

func (reconciler *Reconciler) Reconcile(ctx context.Context) (domain.ReconciliationReport, error) {
	if reconciler == nil || reconciler.documents == nil || reconciler.vault == nil {
		return domain.ReconciliationReport{}, fmt.Errorf("reconciliation dependencies are required")
	}
	documents, err := reconciler.documents.ListDocuments(ctx)
	if err != nil {
		return domain.ReconciliationReport{}, err
	}
	files, err := reconciler.vault.ListFiles()
	if err != nil {
		return domain.ReconciliationReport{}, err
	}
	byPath := make(map[string]string, len(files))
	for _, file := range files {
		byPath[filepath.ToSlash(filepath.Clean(file.Path))] = file.Hash
	}
	report := domain.ReconciliationReport{Issues: make([]domain.ReconciliationIssue, 0)}
	for _, document := range documents {
		report.Scanned++
		path := filepath.ToSlash(filepath.Clean(document.VaultPath))
		actual, ok := byPath[path]
		if !ok {
			report.Conflict++
			report.Issues = append(report.Issues, domain.ReconciliationIssue{Path: path, Kind: "missing_file", ExpectedHash: document.ContentHash})
			continue
		}
		if strings.TrimSpace(document.ContentHash) != "" && actual != document.ContentHash {
			report.Conflict++
			report.Issues = append(report.Issues, domain.ReconciliationIssue{Path: path, Kind: "hash_conflict", ExpectedHash: document.ContentHash, ActualHash: actual})
		}
		delete(byPath, path)
	}
	for path, hash := range byPath {
		report.Changed++
		report.Issues = append(report.Issues, domain.ReconciliationIssue{Path: path, Kind: "orphan_file", ActualHash: hash})
	}
	return report, nil
}
