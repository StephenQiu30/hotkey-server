package application

import (
	"context"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/modules/knowledge/domain"
)

type reconciliationDocuments struct{ items []domain.Document }

func (fake reconciliationDocuments) ListDocuments(context.Context) ([]domain.Document, error) {
	return fake.items, nil
}

type reconciliationFiles struct{ items []domain.VaultFile }

func (fake reconciliationFiles) ListFiles() ([]domain.VaultFile, error) { return fake.items, nil }

func TestReconcilerReportsMissingConflictAndOrphan(t *testing.T) {
	reconciler := NewReconciler(
		reconciliationDocuments{items: []domain.Document{{VaultPath: "events/one.md", ContentHash: "one"}, {VaultPath: "events/missing.md", ContentHash: "missing"}}},
		reconciliationFiles{items: []domain.VaultFile{{Path: "events/one.md", Hash: "different"}, {Path: "topics/orphan.md", Hash: "orphan"}}},
	)
	report, err := reconciler.Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if report.Scanned != 2 || report.Conflict != 2 || report.Changed != 1 || len(report.Issues) != 3 {
		t.Fatalf("reconciliation report = %#v", report)
	}
}
