package app

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestRunMonitorJobReturnsJoinedMonitorErrors(t *testing.T) {
	err := runMonitorJob(context.Background(), "poll_monitor", []int64{1, 2, 3}, func(_ context.Context, monitorID int64) error {
		if monitorID == 2 {
			return errors.New("x api timeout")
		}
		return nil
	})

	if err == nil {
		t.Fatal("expected monitor job error")
	}
	if !strings.Contains(err.Error(), "poll_monitor monitor 2") {
		t.Fatalf("expected monitor id in error, got %v", err)
	}
	if !strings.Contains(err.Error(), "x api timeout") {
		t.Fatalf("expected original cause in error, got %v", err)
	}
}

func TestRunMonitorJobReturnsNilWhenAllMonitorsSucceed(t *testing.T) {
	err := runMonitorJob(context.Background(), "poll_monitor", []int64{1, 2}, func(_ context.Context, _ int64) error {
		return nil
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}
