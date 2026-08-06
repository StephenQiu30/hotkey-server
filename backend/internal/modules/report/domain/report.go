package domain

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

type ReportType string

const (
	ReportDaily  ReportType = "daily"
	ReportWeekly ReportType = "weekly"
)

type ReportStatus string

const (
	ReportDraft     ReportStatus = "draft"
	ReportPublished ReportStatus = "published"
	ReportFailed    ReportStatus = "failed"
	ReportArchived  ReportStatus = "archived"
)

type Period struct {
	Start, End time.Time
	Location   *time.Location
}

func (period Period) Validate() error {
	if period.Start.IsZero() || period.End.IsZero() || !period.End.After(period.Start) || period.Location == nil {
		return fmt.Errorf("invalid report period")
	}
	return nil
}

func PeriodFor(at time.Time, reportType ReportType, location *time.Location) (Period, error) {
	if location == nil {
		return Period{}, fmt.Errorf("report timezone is required")
	}
	local := at.In(location)
	start := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location)
	switch reportType {
	case ReportDaily:
		return Period{Start: start, End: start.AddDate(0, 0, 1), Location: location}, nil
	case ReportWeekly:
		delta := (int(start.Weekday()) + 6) % 7
		week := start.AddDate(0, 0, -delta)
		return Period{Start: week, End: week.AddDate(0, 0, 7), Location: location}, nil
	default:
		return Period{}, fmt.Errorf("invalid report type")
	}
}

type Item struct {
	EventID                         int64
	Rank                            int
	Title, Summary, InclusionReason string
	HeatScore                       float64
}
type Report struct {
	ID, Version, VersionNo int64
	Type                   ReportType
	MonitorID              *int64
	Period                 Period
	Title, Summary, Body   string
	Status                 ReportStatus
	Items                  []Item
	Frozen                 bool
	GeneratedAt            *time.Time
	PublishedAt            *time.Time
}

type ListQuery struct {
	Cursor int64
	Limit  int
	Type   *ReportType
	Status *ReportStatus
}

type Page struct {
	Items      []Report
	NextCursor int64
}

func (query ListQuery) Validate() error {
	if query.Cursor < 0 || query.Limit < 1 || query.Limit > 100 {
		return fmt.Errorf("invalid report list query")
	}
	if query.Type != nil && *query.Type != ReportDaily && *query.Type != ReportWeekly {
		return fmt.Errorf("invalid report type")
	}
	if query.Status != nil && !validStatus(*query.Status) {
		return fmt.Errorf("invalid report status")
	}
	return nil
}

func (report Report) Validate() error {
	if report.ID <= 0 || report.Version <= 0 || report.VersionNo <= 0 || (report.Type != ReportDaily && report.Type != ReportWeekly) || !validStatus(report.Status) {
		return fmt.Errorf("invalid report")
	}
	if err := report.Period.Validate(); err != nil {
		return err
	}
	if report.Status == ReportPublished && !report.Frozen {
		return fmt.Errorf("published report must be frozen")
	}
	for _, item := range report.Items {
		if item.EventID <= 0 || item.Rank <= 0 || strings.TrimSpace(item.Title) == "" || item.HeatScore < 0 {
			return fmt.Errorf("invalid report item")
		}
	}
	return nil
}

func validStatus(status ReportStatus) bool {
	switch status {
	case ReportDraft, ReportPublished, ReportFailed, ReportArchived:
		return true
	default:
		return false
	}
}

func SortItems(items []Item) []Item {
	result := append([]Item(nil), items...)
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].HeatScore != result[j].HeatScore {
			return result[i].HeatScore > result[j].HeatScore
		}
		return result[i].EventID < result[j].EventID
	})
	for index := range result {
		result[index].Rank = index + 1
	}
	return result
}
