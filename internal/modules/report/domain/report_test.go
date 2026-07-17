package domain

import (
	"testing"
	"time"
)

func TestPeriodUsesSubscriberTimezone(t *testing.T) {
	location, _ := time.LoadLocation("Asia/Shanghai")
	period, err := PeriodFor(time.Date(2026, 7, 16, 23, 30, 0, 0, time.UTC), ReportDaily, location)
	if err != nil {
		t.Fatal(err)
	}
	if period.Start.Day() != 17 {
		t.Fatalf("period start = %v", period.Start)
	}
}
func TestSortItemsIsDeterministic(t *testing.T) {
	items := SortItems([]Item{{EventID: 2, Title: "b", HeatScore: 90}, {EventID: 1, Title: "a", HeatScore: 90}})
	if items[0].EventID != 1 || items[0].Rank != 1 {
		t.Fatalf("items = %#v", items)
	}
}
