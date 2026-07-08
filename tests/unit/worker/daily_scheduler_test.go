package worker_test

import (
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/worker"
)

func TestResolveTargetDateYesterday(t *testing.T) {
	now := time.Date(2026, 7, 8, 8, 30, 0, 0, time.FixedZone("CST", 8*60*60))
	got, err := worker.ResolveTargetDate(now, struct {
		Timezone string
		Target   string
	}{
		Timezone: "Asia/Shanghai",
		Target:   "yesterday",
	})
	if err != nil {
		t.Fatalf("ResolveTargetDate returned error: %v", err)
	}
	if got.Format("2006-01-02") != "2026-07-07" {
		t.Fatalf("target date = %s, want 2026-07-07", got.Format("2006-01-02"))
	}
}

func TestResolveTargetDateToday(t *testing.T) {
	now := time.Date(2026, 7, 8, 8, 30, 0, 0, time.FixedZone("CST", 8*60*60))
	got, err := worker.ResolveTargetDate(now, struct {
		Timezone string
		Target   string
	}{
		Timezone: "Asia/Shanghai",
		Target:   "today",
	})
	if err != nil {
		t.Fatalf("ResolveTargetDate returned error: %v", err)
	}
	if got.Format("2006-01-02") != "2026-07-08" {
		t.Fatalf("target date = %s, want 2026-07-08", got.Format("2006-01-02"))
	}
}

func TestRunKeyForDate(t *testing.T) {
	date := time.Date(2026, 7, 7, 0, 0, 0, 0, time.UTC)
	got := worker.RunKeyForDate(date)
	want := "daily-obsidian-publish:2026-07-07"
	if got != want {
		t.Fatalf("run key = %q, want %q", got, want)
	}
}
