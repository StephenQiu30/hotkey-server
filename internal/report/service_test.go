package report

import (
	"testing"
	"time"
)

func TestGeneratePlatformDailyReportForPreviousDay(t *testing.T) {
	service := NewService()
	reportDate := time.Date(2026, 5, 25, 0, 0, 0, 0, time.UTC)

	report, err := service.GeneratePlatformDailyReport(reportDate, []HotspotSnapshot{
		{
			EventID:     "cluster_1",
			Title:       "OpenAI releases new reasoning model",
			Keywords:    []string{"OpenAI", "model"},
			HeatScore:   90,
			TrustScore:  95,
			EvidenceIDs: []string{"item_1", "item_2"},
		},
		{
			EventID:     "cluster_2",
			Title:       "Anthropic publishes AI safety report",
			Keywords:    []string{"Anthropic", "safety"},
			HeatScore:   70,
			TrustScore:  90,
			EvidenceIDs: []string{"item_3"},
		},
	})
	if err != nil {
		t.Fatalf("GeneratePlatformDailyReport returned error: %v", err)
	}

	if report.Scope != ScopePlatform {
		t.Fatalf("scope = %q, want %q", report.Scope, ScopePlatform)
	}
	if !report.ReportDate.Equal(reportDate) {
		t.Fatalf("reportDate = %s, want %s", report.ReportDate, reportDate)
	}
	if len(report.Items) != 2 {
		t.Fatalf("items len = %d, want 2", len(report.Items))
	}
	if report.Items[0].EventID != "cluster_1" {
		t.Fatalf("first event = %q, want cluster_1", report.Items[0].EventID)
	}
	if len(report.Items[0].EvidenceIDs) != 2 {
		t.Fatalf("first evidence IDs = %#v", report.Items[0].EvidenceIDs)
	}
}

func TestGenerateUserDailyReportFiltersByFollowedKeywords(t *testing.T) {
	service := NewService()
	reportDate := time.Date(2026, 5, 25, 0, 0, 0, 0, time.UTC)
	hotspots := []HotspotSnapshot{
		{
			EventID:     "cluster_1",
			Title:       "OpenAI releases new reasoning model",
			Keywords:    []string{"OpenAI", "model"},
			HeatScore:   90,
			TrustScore:  95,
			EvidenceIDs: []string{"item_1"},
		},
		{
			EventID:     "cluster_2",
			Title:       "Anthropic publishes AI safety report",
			Keywords:    []string{"Anthropic", "safety"},
			HeatScore:   70,
			TrustScore:  90,
			EvidenceIDs: []string{"item_2"},
		},
	}

	report, err := service.GenerateUserDailyReport(reportDate, "user-1", []string{"openai"}, hotspots)
	if err != nil {
		t.Fatalf("GenerateUserDailyReport returned error: %v", err)
	}

	if report.Scope != ScopeUser {
		t.Fatalf("scope = %q, want %q", report.Scope, ScopeUser)
	}
	if report.UserID != "user-1" {
		t.Fatalf("userID = %q, want user-1", report.UserID)
	}
	if len(report.Items) != 1 {
		t.Fatalf("items len = %d, want 1", len(report.Items))
	}
	if report.Items[0].EventID != "cluster_1" {
		t.Fatalf("eventID = %q, want cluster_1", report.Items[0].EventID)
	}
}

func TestGenerateDailyReportRejectsItemsWithoutEvidence(t *testing.T) {
	service := NewService()

	_, err := service.GeneratePlatformDailyReport(time.Now().UTC(), []HotspotSnapshot{
		{
			EventID:    "cluster_1",
			Title:      "OpenAI releases new reasoning model",
			Keywords:   []string{"OpenAI"},
			HeatScore:  90,
			TrustScore: 95,
		},
	})

	if err != ErrMissingEvidenceLink {
		t.Fatalf("err = %v, want %v", err, ErrMissingEvidenceLink)
	}
}
