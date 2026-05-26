package redisinfra

import (
	"testing"
	"time"
)

func TestTaskLockPreventsDuplicateExecution(t *testing.T) {
	service := NewService()

	first, err := service.AcquireTaskLock("refresh:global", "worker-1", time.Minute)
	if err != nil {
		t.Fatalf("first AcquireTaskLock returned error: %v", err)
	}
	if !first.Acquired {
		t.Fatalf("first lock acquired = false, want true")
	}

	second, err := service.AcquireTaskLock("refresh:global", "worker-2", time.Minute)
	if err != nil {
		t.Fatalf("second AcquireTaskLock returned error: %v", err)
	}
	if second.Acquired {
		t.Fatalf("second lock acquired = true, want false")
	}
	if second.Status != StatusDuplicate {
		t.Fatalf("second status = %q, want %q", second.Status, StatusDuplicate)
	}

	if err := service.ReleaseTaskLock("refresh:global", "worker-1"); err != nil {
		t.Fatalf("ReleaseTaskLock returned error: %v", err)
	}
	third, err := service.AcquireTaskLock("refresh:global", "worker-2", time.Minute)
	if err != nil {
		t.Fatalf("third AcquireTaskLock returned error: %v", err)
	}
	if !third.Acquired {
		t.Fatalf("third lock acquired = false, want true")
	}
}

func TestManualRefreshRateLimitAndQueue(t *testing.T) {
	service := NewService()

	first, err := service.EnqueueRefresh(RefreshRequest{
		UserID: "user-1",
		Scope:  "keyword",
		Target: "OpenAI",
		Now:    time.Date(2026, 5, 26, 8, 0, 0, 0, time.UTC),
		Limit:  2,
		Window: time.Hour,
	})
	if err != nil {
		t.Fatalf("first EnqueueRefresh returned error: %v", err)
	}
	if !first.Accepted {
		t.Fatalf("first accepted = false, want true")
	}

	second, _ := service.EnqueueRefresh(RefreshRequest{
		UserID: "user-1", Scope: "keyword", Target: "OpenAI", Now: first.EnqueuedAt.Add(10 * time.Minute), Limit: 2, Window: time.Hour,
	})
	if !second.Accepted {
		t.Fatalf("second accepted = false, want true")
	}

	third, _ := service.EnqueueRefresh(RefreshRequest{
		UserID: "user-1", Scope: "keyword", Target: "OpenAI", Now: first.EnqueuedAt.Add(20 * time.Minute), Limit: 2, Window: time.Hour,
	})
	if third.Accepted {
		t.Fatalf("third accepted = true, want false")
	}
	if third.Status != StatusRateLimited {
		t.Fatalf("third status = %q, want %q", third.Status, StatusRateLimited)
	}
	if len(service.ListRefreshQueue()) != 2 {
		t.Fatalf("queue len = %d, want 2", len(service.ListRefreshQueue()))
	}
}

func TestShortTermDeduplication(t *testing.T) {
	service := NewService()

	first := service.MarkSeen("source:item:1", time.Minute)
	if !first.Accepted {
		t.Fatalf("first accepted = false, want true")
	}
	second := service.MarkSeen("source:item:1", time.Minute)
	if second.Accepted {
		t.Fatalf("second accepted = true, want false")
	}
	if second.Status != StatusDuplicate {
		t.Fatalf("second status = %q, want %q", second.Status, StatusDuplicate)
	}
}

func TestRedisUnavailableReadDegrades(t *testing.T) {
	service := NewService()
	service.SetAvailable(false)

	queue := service.ListRefreshQueue()
	if queue == nil {
		t.Fatalf("queue is nil, want empty degraded response")
	}
	status := service.Health()
	if status.Available {
		t.Fatalf("available = true, want false")
	}
	if status.Mode != ModeDegraded {
		t.Fatalf("mode = %q, want %q", status.Mode, ModeDegraded)
	}
}
