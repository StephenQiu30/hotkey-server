package report

import (
	"testing"
	"time"
)

func TestTenantDailyReportsAreIsolated(t *testing.T) {
	service := NewService()
	date := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	hotspots := []HotspotSnapshot{
		{
			EventID:     "event-alpha",
			TenantID:    "tenant-alpha",
			Title:       "Alpha AI launch",
			Keywords:    []string{"Alpha"},
			HeatScore:   80,
			TrustScore:  90,
			EvidenceIDs: []string{"item-alpha"},
		},
		{
			EventID:     "event-beta",
			TenantID:    "tenant-beta",
			Title:       "Beta model update",
			Keywords:    []string{"Beta"},
			HeatScore:   70,
			TrustScore:  85,
			EvidenceIDs: []string{"item-beta"},
		},
	}

	alpha, err := service.GenerateTenantDailyReport(date, "tenant-alpha", hotspots)
	if err != nil {
		t.Fatalf("generate alpha report: %v", err)
	}
	beta, err := service.GenerateTenantDailyReport(date, "tenant-beta", hotspots)
	if err != nil {
		t.Fatalf("generate beta report: %v", err)
	}

	if alpha.TenantID != "tenant-alpha" || beta.TenantID != "tenant-beta" {
		t.Fatalf("tenant ids = %q/%q", alpha.TenantID, beta.TenantID)
	}
	if len(alpha.Items) != 1 || alpha.Items[0].EventID != "event-alpha" {
		t.Fatalf("alpha items = %#v", alpha.Items)
	}
	if len(beta.Items) != 1 || beta.Items[0].EventID != "event-beta" {
		t.Fatalf("beta items = %#v", beta.Items)
	}
}
