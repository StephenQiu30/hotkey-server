package obsidian

import (
	"testing"
)

func TestBuildKnowledgePath_Event(t *testing.T) {
	got := BuildKnowledgePath("/vault", PathInput{
		Kind:        "event",
		MonitorSlug: "ai-regulation",
		Date:        "2026-07-01",
		StableID:    "evt-101",
		TitleSlug:   "ai-guize",
	})
	want := "/vault/HotKey/events/ai-regulation/2026-07-01-evt-101-ai-guize.md"
	if got != want {
		t.Fatalf("BuildKnowledgePath = %q, want %q", got, want)
	}
}

func TestBuildKnowledgePath_Topic(t *testing.T) {
	got := BuildKnowledgePath("/vault", PathInput{
		Kind:        "topic",
		MonitorSlug: "ai-regulation",
		Date:        "2026-07-01",
		StableID:    "42",
		TitleSlug:   "ai-zhengce",
	})
	want := "/vault/HotKey/topics/ai-regulation/2026-07-01-topic-42-ai-zhengce.md"
	if got != want {
		t.Fatalf("BuildKnowledgePath = %q, want %q", got, want)
	}
}

func TestBuildKnowledgePath_DailyDigest(t *testing.T) {
	got := BuildKnowledgePath("/vault", PathInput{
		Kind:        "daily-digest",
		MonitorSlug: "ai-regulation",
		Date:        "2026-07-01",
		StableID:    "ddigest-101",
	})
	want := "/vault/HotKey/digests/daily/ai-regulation/2026-07-01-ddigest-101.md"
	if got != want {
		t.Fatalf("BuildKnowledgePath = %q, want %q", got, want)
	}
}

func TestBuildKnowledgePath_Theme(t *testing.T) {
	got := BuildKnowledgePath("/vault", PathInput{
		Kind:      "theme",
		StableID:  "thm-42",
		TitleSlug: "ai-security",
	})
	want := "/vault/HotKey/themes/thm-42-ai-security.md"
	if got != want {
		t.Fatalf("BuildKnowledgePath = %q, want %q", got, want)
	}
}

func TestBuildKnowledgePath_WeeklyExport(t *testing.T) {
	got := BuildKnowledgePath("/vault", PathInput{
		Kind:        "weekly-export",
		MonitorSlug: "ai-regulation",
		Date:        "2026-W27",
		StableID:    "wexp-101",
	})
	want := "/vault/HotKey/exports/weekly/ai-regulation/2026-W27-wexp-101.md"
	if got != want {
		t.Fatalf("BuildKnowledgePath = %q, want %q", got, want)
	}
}

func TestBuildKnowledgePath_ThematicExport(t *testing.T) {
	got := BuildKnowledgePath("/vault", PathInput{
		Kind:      "thematic-export",
		Date:      "2026-07-01",
		StableID:  "texp-101",
		TitleSlug: "ai-overview",
	})
	want := "/vault/HotKey/exports/thematic/2026-07-01-texp-101-ai-overview.md"
	if got != want {
		t.Fatalf("BuildKnowledgePath = %q, want %q", got, want)
	}
}

func TestBuildKnowledgePath_MaterialExport(t *testing.T) {
	got := BuildKnowledgePath("/vault", PathInput{
		Kind:     "material-export",
		Date:     "2026-07-01",
		StableID: "mexp-101",
	})
	want := "/vault/HotKey/exports/material/2026-07-01-mexp-101.md"
	if got != want {
		t.Fatalf("BuildKnowledgePath = %q, want %q", got, want)
	}
}

func TestBuildKnowledgePath_UnknownKind(t *testing.T) {
	got := BuildKnowledgePath("/vault", PathInput{
		Kind:     "unknown",
		Date:     "2026-07-01",
		StableID: "unk-1",
	})
	want := "/vault/HotKey/misc/2026-07-01-unk-1.md"
	if got != want {
		t.Fatalf("BuildKnowledgePath = %q, want %q", got, want)
	}
}

func TestBuildKnowledgePath_EmptyKind(t *testing.T) {
	got := BuildKnowledgePath("/vault", PathInput{})
	want := "/vault/HotKey/misc/unfiled.md"
	if got != want {
		t.Fatalf("BuildKnowledgePath = %q, want %q", got, want)
	}
}
