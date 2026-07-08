package obsidian_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/model/dto"
	"github.com/StephenQiu30/hotkey-server/internal/service"
)

func TestBuildPathDailyDigest(t *testing.T) {
	got, err := service.BuildPath("/vault", dto.PathInput{
		Kind:        dto.ExportDailyDigest,
		Date:        time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC),
		MonitorName: "AI Regulation",
	})
	if err != nil {
		t.Fatalf("BuildPath returned error: %v", err)
	}
	want := filepath.Join("/vault", "HotKey", "digests", "daily", "2026-07-08", "ai-regulation.md")
	if got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
}

func TestBuildPathPublishDraft(t *testing.T) {
	got, err := service.BuildPath("/vault", dto.PathInput{
		Kind:        dto.ExportPublishDraft,
		Date:        time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC),
		MonitorName: "AI Regulation",
	})
	if err != nil {
		t.Fatalf("BuildPath returned error: %v", err)
	}
	want := filepath.Join("/vault", "HotKey", "publish", "drafts", "2026-07-08", "ai-regulation.md")
	if got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
}

func TestBuildPathRejectsMissingVault(t *testing.T) {
	_, err := service.BuildPath("", dto.PathInput{
		Kind:        dto.ExportDailyDigest,
		Date:        time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC),
		MonitorName: "AI Regulation",
	})
	if err != service.ErrMissingVaultRoot {
		t.Fatalf("error = %v, want %v", err, service.ErrMissingVaultRoot)
	}
}
