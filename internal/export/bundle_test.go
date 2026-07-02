package export

import (
	"testing"
)

func TestBuildExportBundle_WeeklyDigest(t *testing.T) {
	bundle := BuildExportBundle(BuildExportBundleInput{
		Kind:      "weekly",
		MonitorID: 1,
		DateRange: DateRange{Start: "2026-06-24", End: "2026-06-30"},
	})
	if bundle.Kind != "weekly" {
		t.Fatalf("got kind %q, want %q", bundle.Kind, "weekly")
	}
	if bundle.DateRange.Start != "2026-06-24" {
		t.Fatalf("got start %q, want %q", bundle.DateRange.Start, "2026-06-24")
	}
	if bundle.DateRange.End != "2026-06-30" {
		t.Fatalf("got end %q, want %q", bundle.DateRange.End, "2026-06-30")
	}
}

func TestBuildExportBundle_MonthlyDigest(t *testing.T) {
	bundle := BuildExportBundle(BuildExportBundleInput{
		Kind:      "monthly",
		MonitorID: 2,
		DateRange: DateRange{Start: "2026-06-01", End: "2026-06-30"},
	})
	if bundle.Kind != "monthly" {
		t.Fatalf("got kind %q, want %q", bundle.Kind, "monthly")
	}
	if bundle.MonitorID != 2 {
		t.Fatalf("got monitor_id %d, want %d", bundle.MonitorID, 2)
	}
}

func TestBuildExportBundle_Thematic(t *testing.T) {
	bundle := BuildExportBundle(BuildExportBundleInput{
		Kind:     "thematic",
		ThemeIDs: []int64{1, 2, 3},
	})
	if bundle.Kind != "thematic" {
		t.Fatalf("got kind %q, want %q", bundle.Kind, "thematic")
	}
	if len(bundle.ThemeIDs) != 3 {
		t.Fatalf("got %d theme ids, want 3", len(bundle.ThemeIDs))
	}
}

func TestBuildExportBundle_Material(t *testing.T) {
	bundle := BuildExportBundle(BuildExportBundleInput{
		Kind:     "material",
		TopicIDs: []int64{10, 20},
		EventIDs: []int64{100, 200},
	})
	if bundle.Kind != "material" {
		t.Fatalf("got kind %q, want %q", bundle.Kind, "material")
	}
	if len(bundle.TopicIDs) != 2 {
		t.Fatalf("got %d topic ids, want 2", len(bundle.TopicIDs))
	}
}

func TestExportKind_Values(t *testing.T) {
	if ExportDaily != "daily" {
		t.Fatalf("ExportDaily = %q", ExportDaily)
	}
	if ExportWeekly != "weekly" {
		t.Fatalf("ExportWeekly = %q", ExportWeekly)
	}
	if ExportMonthly != "monthly" {
		t.Fatalf("ExportMonthly = %q", ExportMonthly)
	}
	if ExportThematic != "thematic" {
		t.Fatalf("ExportThematic = %q", ExportThematic)
	}
	if ExportMaterial != "material" {
		t.Fatalf("ExportMaterial = %q", ExportMaterial)
	}
}
