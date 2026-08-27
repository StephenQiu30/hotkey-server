package application

import (
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
)

func TestGroupMonitorScansDerivesPartialSuccessFromThreeIndependentSourceFacts(t *testing.T) {
	scheduledAt := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	finishedAt := scheduledAt.Add(4 * time.Second)
	sources := []domain.MonitorScanSource{
		{
			RunID: 41, MonitorID: 9, SourceConnectionID: 12, SourceName: "RSS",
			SourceType: "rss", TriggerType: domain.CollectionTriggerManual,
			Status: domain.CollectionRunSucceeded, CandidateCount: 3, AcceptedCount: 2,
			RejectedCount: 1, ScheduledAt: scheduledAt, FinishedAt: &finishedAt,
		},
		{
			RunID: 42, MonitorID: 9, SourceConnectionID: 13, SourceName: "Hacker News",
			SourceType: "hacker_news", TriggerType: domain.CollectionTriggerManual,
			Status:      domain.CollectionRunSucceeded,
			ScheduledAt: scheduledAt, FinishedAt: &finishedAt,
		},
		{
			RunID: 43, MonitorID: 9, SourceConnectionID: 14, SourceName: "X",
			SourceType: "x", TriggerType: domain.CollectionTriggerManual,
			Status: domain.CollectionRunFailed, ErrorCode: "rate_limited",
			ScheduledAt: scheduledAt, FinishedAt: &finishedAt,
		},
	}

	items := groupMonitorScans(sources)
	if len(items) != 1 {
		t.Fatalf("scan count = %d, want 1", len(items))
	}
	scan := items[0]
	if scan.ID != "manual:1786622400000000000" || scan.Status != domain.MonitorScanPartial || scan.RunOutcome != domain.MonitorScanOutcomePartialSuccess {
		t.Fatalf("scan identity/status/outcome = %q/%q/%q, want stable manual id, partial and partial_success", scan.ID, scan.Status, scan.RunOutcome)
	}
	if scan.CandidateCount != 3 || scan.AcceptedCount != 2 || scan.RejectedCount != 1 || len(scan.Sources) != 3 {
		t.Fatalf("scan totals = %#v", scan)
	}
	if scan.Sources[0].SourceType != "rss" || scan.Sources[1].SourceType != "hacker_news" || scan.Sources[2].ErrorCode != "rate_limited" {
		t.Fatalf("independent source facts = %#v", scan.Sources)
	}
}

func TestMonitorScanStatus(t *testing.T) {
	tests := []struct {
		name   string
		states []domain.CollectionRunStatus
		want   domain.MonitorScanStatus
	}{
		{name: "queued", states: []domain.CollectionRunStatus{domain.CollectionRunQueued}, want: domain.MonitorScanQueued},
		{name: "running", states: []domain.CollectionRunStatus{domain.CollectionRunSucceeded, domain.CollectionRunRunning}, want: domain.MonitorScanRunning},
		{name: "succeeded", states: []domain.CollectionRunStatus{domain.CollectionRunSucceeded, domain.CollectionRunSucceeded}, want: domain.MonitorScanSucceeded},
		{name: "failed", states: []domain.CollectionRunStatus{domain.CollectionRunFailed, domain.CollectionRunCancelled}, want: domain.MonitorScanFailed},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sources := make([]domain.MonitorScanSource, 0, len(test.states))
			for _, state := range test.states {
				sources = append(sources, domain.MonitorScanSource{Status: state})
			}
			if got := monitorScanStatus(sources); got != test.want {
				t.Fatalf("status = %q, want %q", got, test.want)
			}
		})
	}
}

func TestMonitorScanOutcomeExistsOnlyForTerminalScan(t *testing.T) {
	tests := []struct {
		status domain.MonitorScanStatus
		want   domain.MonitorScanOutcome
	}{
		{status: domain.MonitorScanQueued},
		{status: domain.MonitorScanRunning},
		{status: domain.MonitorScanSucceeded, want: domain.MonitorScanOutcomeSuccess},
		{status: domain.MonitorScanPartial, want: domain.MonitorScanOutcomePartialSuccess},
		{status: domain.MonitorScanFailed, want: domain.MonitorScanOutcomeFailed},
	}
	for _, test := range tests {
		if got := monitorScanOutcome(test.status); got != test.want {
			t.Fatalf("outcome(%q) = %q, want %q", test.status, got, test.want)
		}
	}
}
