package jobs

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/export"
)

func TestPublishExportsJob_DailyExport(t *testing.T) {
	vaultRoot := t.TempDir()
	job := NewPublishExportsJob(vaultRoot)

	err := job.Run(context.Background(), export.ExportDaily, export.BuildExportBundleInput{
		Kind:      "daily",
		MonitorID: 1,
		DateRange: export.DateRange{Start: "2026-07-01", End: "2026-07-01"},
	})
	if err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	// Verify the export file was created under exports/daily/
	exportsDir := filepath.Join(vaultRoot, "HotKey", "exports", "daily")
	files, err := os.ReadDir(exportsDir)
	if err != nil {
		t.Fatalf("ReadDir(%s): %v", exportsDir, err)
	}
	if len(files) == 0 {
		t.Fatal("expected at least one export file")
	}
}

func TestPublishExportsJob_WeeklyExport(t *testing.T) {
	vaultRoot := t.TempDir()
	job := NewPublishExportsJob(vaultRoot)

	err := job.Run(context.Background(), export.ExportWeekly, export.BuildExportBundleInput{
		Kind:      "weekly",
		MonitorID: 2,
		DateRange: export.DateRange{Start: "2026-W27", End: "2026-W27"},
	})
	if err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	exportsDir := filepath.Join(vaultRoot, "HotKey", "exports", "weekly")
	files, err := os.ReadDir(exportsDir)
	if err != nil {
		t.Fatalf("ReadDir(%s): %v", exportsDir, err)
	}
	if len(files) == 0 {
		t.Fatal("expected at least one export file")
	}
}

func TestPublishExportsJob_MaterialExport(t *testing.T) {
	vaultRoot := t.TempDir()
	job := NewPublishExportsJob(vaultRoot)

	err := job.Run(context.Background(), export.ExportMaterial, export.BuildExportBundleInput{
		Kind:      "material",
		MonitorID: 1,
	})
	if err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	exportsDir := filepath.Join(vaultRoot, "HotKey", "exports", "material")
	files, err := os.ReadDir(exportsDir)
	if err != nil {
		t.Fatalf("ReadDir(%s): %v", exportsDir, err)
	}
	if len(files) == 0 {
		t.Fatal("expected at least one export file")
	}
}

func TestPublishExportsJob_UnknownKind(t *testing.T) {
	vaultRoot := t.TempDir()
	job := NewPublishExportsJob(vaultRoot)

	err := job.Run(context.Background(), export.ExportKind("unknown"), export.BuildExportBundleInput{})
	if err == nil {
		t.Fatal("expected error for unknown export kind")
	}
}
