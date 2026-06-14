package digest

import (
	"testing"
	"time"
)

func TestDayWindow_CSTBoundary(t *testing.T) {
	cst := time.FixedZone("CST", 8*3600)

	tests := []struct {
		name      string
		inputDate string // "2006-01-02" in CST
		wantStart string // RFC3339 in UTC
		wantEnd   string // RFC3339 in UTC
	}{
		{
			name:      "normal day",
			inputDate: "2026-06-14",
			wantStart: "2026-06-13T16:00:00Z", // Jun 14 00:00 CST = Jun 13 16:00 UTC
			wantEnd:   "2026-06-14T16:00:00Z", // Jun 15 00:00 CST = Jun 14 16:00 UTC
		},
		{
			name:      "UTC midnight same calendar day",
			inputDate: "2026-01-01",
			wantStart: "2025-12-31T16:00:00Z", // Jan 1 00:00 CST = Dec 31 16:00 UTC
			wantEnd:   "2026-01-01T16:00:00Z", // Jan 2 00:00 CST = Jan 1 16:00 UTC
		},
		{
			name:      "leap day",
			inputDate: "2028-02-29",
			wantStart: "2028-02-28T16:00:00Z",
			wantEnd:   "2028-02-29T16:00:00Z",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			date, err := time.ParseInLocation("2006-01-02", tt.inputDate, cst)
			if err != nil {
				t.Fatalf("parse date: %v", err)
			}
			got := DayWindow(date)

			wantStart, _ := time.Parse(time.RFC3339, tt.wantStart)
			wantEnd, _ := time.Parse(time.RFC3339, tt.wantEnd)

			if !got.Start.Equal(wantStart) {
				t.Errorf("Start = %v, want %v", got.Start.Format(time.RFC3339), tt.wantStart)
			}
			if !got.End.Equal(wantEnd) {
				t.Errorf("End = %v, want %v", got.End.Format(time.RFC3339), tt.wantEnd)
			}
		})
	}
}

func TestDayWindow_HalfOpenInterval(t *testing.T) {
	cst := time.FixedZone("CST", 8*3600)
	date, _ := time.ParseInLocation("2006-01-02", "2026-06-14", cst)
	w := DayWindow(date)

	// inclusive start
	if !w.Contains(w.Start) {
		t.Error("window should contain its start")
	}
	// exclusive end
	if w.Contains(w.End) {
		t.Error("window should NOT contain its end")
	}
}

func TestResolveExportDate_Today(t *testing.T) {
	cst := time.FixedZone("CST", 8*3600)
	// 2026-06-14 10:30 CST
	now, _ := time.ParseInLocation("2006-01-02 15:04", "2026-06-14 10:30", cst)

	got := ResolveExportDate(now, "today")
	want, _ := time.ParseInLocation("2006-01-02", "2026-06-14", cst)

	if !got.Equal(want) {
		t.Errorf("ResolveExportDate(today) = %v, want %v", got, want)
	}
}

func TestResolveExportDate_Yesterday(t *testing.T) {
	cst := time.FixedZone("CST", 8*3600)
	// 2026-06-14 10:30 CST
	now, _ := time.ParseInLocation("2006-01-02 15:04", "2026-06-14 10:30", cst)

	got := ResolveExportDate(now, "yesterday")
	want, _ := time.ParseInLocation("2006-01-02", "2026-06-13", cst)

	if !got.Equal(want) {
		t.Errorf("ResolveExportDate(yesterday) = %v, want %v", got, want)
	}
}

func TestResolveExportDate_MidnightEdge(t *testing.T) {
	cst := time.FixedZone("CST", 8*3600)
	// exactly midnight CST
	now, _ := time.ParseInLocation("2006-01-02 15:04", "2026-06-14 00:00", cst)

	got := ResolveExportDate(now, "today")
	want, _ := time.ParseInLocation("2006-01-02", "2026-06-14", cst)

	if !got.Equal(want) {
		t.Errorf("ResolveExportDate(midnight) = %v, want %v", got, want)
	}
}
