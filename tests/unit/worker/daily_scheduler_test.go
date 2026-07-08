package worker_test

import (
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/worker"
)

func TestResolveTargetDateYesterday(t *testing.T) {
	now := time.Date(2026, 7, 8, 8, 30, 0, 0, time.FixedZone("CST", 8*60*60))
	got, err := worker.ResolveTargetDate(now, worker.DailyScheduleConfig{
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
	got, err := worker.ResolveTargetDate(now, worker.DailyScheduleConfig{
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

func TestShouldRunAfterConfiguredTime(t *testing.T) {
	now := time.Date(2026, 7, 8, 8, 1, 0, 0, time.FixedZone("CST", 8*60*60))
	should, target, err := worker.ShouldRun(now, nil, worker.DailyScheduleConfig{
		Time:     "08:00",
		Timezone: "Asia/Shanghai",
		Target:   "yesterday",
	})
	if err != nil {
		t.Fatalf("ShouldRun returned error: %v", err)
	}
	if !should {
		t.Fatal("ShouldRun = false, want true")
	}
	if target.Format("2006-01-02") != "2026-07-07" {
		t.Fatalf("target = %s, want 2026-07-07", target.Format("2006-01-02"))
	}
}

func TestShouldRunBeforeConfiguredTime(t *testing.T) {
	now := time.Date(2026, 7, 8, 7, 59, 0, 0, time.FixedZone("CST", 8*60*60))
	should, _, err := worker.ShouldRun(now, nil, worker.DailyScheduleConfig{
		Time:     "08:00",
		Timezone: "Asia/Shanghai",
		Target:   "yesterday",
	})
	if err != nil {
		t.Fatalf("ShouldRun returned error: %v", err)
	}
	if should {
		t.Fatal("ShouldRun = true before configured time")
	}
}
