package application

import (
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
)

func TestGroupMonitorScansBuildsOneSimpleScanWithSourceProgress(t *testing.T) {
	scheduledAt := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	finishedAt := scheduledAt.Add(4 * time.Second)
	sources := []domain.MonitorScanSource{
		{
			RunID: 41, MonitorID: 9, SourceConnectionID: 12, SourceName: "Hacker News",
			SourceType: "hacker_news", TriggerType: domain.CollectionTriggerManual,
			Status: domain.CollectionRunSucceeded, CandidateCount: 8, AcceptedCount: 3,
			RejectedCount: 5, ScheduledAt: scheduledAt, FinishedAt: &finishedAt,
		},
		{
			RunID: 42, MonitorID: 9, SourceConnectionID: 13, SourceName: "RSS",
			SourceType: "rss", TriggerType: domain.CollectionTriggerManual,
			Status: domain.CollectionRunFailed, ErrorCode: "source_unavailable",
			ScheduledAt: scheduledAt, FinishedAt: &finishedAt,
		},
	}

	items := groupMonitorScans(sources)
	if len(items) != 1 {
		t.Fatalf("scan count = %d, want 1", len(items))
	}
	scan := items[0]
	if scan.ID != "manual:1786622400000000000" || scan.Status != domain.MonitorScanPartial {
		t.Fatalf("scan identity/status = %q/%q, want stable manual id and partial", scan.ID, scan.Status)
	}
	if scan.CandidateCount != 8 || scan.AcceptedCount != 3 || scan.RejectedCount != 5 || len(scan.Sources) != 2 {
		t.Fatalf("scan totals = %#v", scan)
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
