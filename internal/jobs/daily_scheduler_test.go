package jobs

import (
	"testing"
	"time"
)

func TestDailyScheduler_ShouldRun_BeforeTargetTime(t *testing.T) {
	sched := NewDailyScheduler("08:00", "Asia/Shanghai")
	// 2026-06-14 07:30 CST = 2026-06-13 23:30 UTC
	now := time.Date(2026, 6, 13, 23, 30, 0, 0, time.UTC)
	if sched.ShouldRun(now, "") {
		t.Fatal("expected ShouldRun=false before 08:00 CST")
	}
}

func TestDailyScheduler_ShouldRun_AtTargetTime(t *testing.T) {
	sched := NewDailyScheduler("08:00", "Asia/Shanghai")
	// 2026-06-14 08:00 CST = 2026-06-14 00:00 UTC
	now := time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC)
	if !sched.ShouldRun(now, "") {
		t.Fatal("expected ShouldRun=true at 08:00 CST")
	}
}

func TestDailyScheduler_ShouldRun_AfterTargetTime(t *testing.T) {
	sched := NewDailyScheduler("08:00", "Asia/Shanghai")
	// 2026-06-14 12:00 CST = 2026-06-14 04:00 UTC
	now := time.Date(2026, 6, 14, 4, 0, 0, 0, time.UTC)
	if !sched.ShouldRun(now, "") {
		t.Fatal("expected ShouldRun=true after 08:00 CST")
	}
}

func TestDailyScheduler_ShouldRun_SameDayNoRepeat(t *testing.T) {
	sched := NewDailyScheduler("08:00", "Asia/Shanghai")
	// 2026-06-14 12:00 CST
	now := time.Date(2026, 6, 14, 4, 0, 0, 0, time.UTC)
	// Already ran today
	if sched.ShouldRun(now, "2026-06-14") {
		t.Fatal("expected ShouldRun=false when already ran today")
	}
}

func TestDailyScheduler_ShouldRun_NextDay(t *testing.T) {
	sched := NewDailyScheduler("08:00", "Asia/Shanghai")
	// 2026-06-15 09:00 CST = 2026-06-15 01:00 UTC
	now := time.Date(2026, 6, 15, 1, 0, 0, 0, time.UTC)
	// Last ran on 2026-06-14
	if !sched.ShouldRun(now, "2026-06-14") {
		t.Fatal("expected ShouldRun=true on next day")
	}
}

func TestDailyScheduler_ShouldRun_InvalidTargetTime(t *testing.T) {
	sched := NewDailyScheduler("bad", "Asia/Shanghai")
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	if sched.ShouldRun(now, "") {
		t.Fatal("expected ShouldRun=false for invalid target time")
	}
}

func TestDailyScheduler_MarkRun(t *testing.T) {
	sched := NewDailyScheduler("08:00", "Asia/Shanghai")
	// 2026-06-14 12:00 CST = 2026-06-14 04:00 UTC
	now := time.Date(2026, 6, 14, 4, 0, 0, 0, time.UTC)
	date := sched.MarkRun(now)
	if date != "2026-06-14" {
		t.Fatalf("expected 2026-06-14, got %s", date)
	}
}

func TestDailyScheduler_ShouldRun_ExactMinuteBoundary(t *testing.T) {
	sched := NewDailyScheduler("08:30", "Asia/Shanghai")
	// 2026-06-14 08:29 CST = 2026-06-14 00:29 UTC
	before := time.Date(2026, 6, 14, 0, 29, 0, 0, time.UTC)
	if sched.ShouldRun(before, "") {
		t.Fatal("expected ShouldRun=false at 08:29 CST when target is 08:30")
	}
	// 2026-06-14 08:30 CST = 2026-06-14 00:30 UTC
	at := time.Date(2026, 6, 14, 0, 30, 0, 0, time.UTC)
	if !sched.ShouldRun(at, "") {
		t.Fatal("expected ShouldRun=true at 08:30 CST")
	}
}
