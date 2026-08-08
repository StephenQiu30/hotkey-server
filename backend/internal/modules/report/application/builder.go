package application

import (
	"fmt"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/report/domain"
)

type EventSnapshot struct {
	EventID, EventUpdateID int64
	Title, Summary         string
	HeatScore              float64
	EvidenceSetHash        string
	ReasonCodes            []string
}
type Builder struct{}

func NewBuilder() *Builder { return &Builder{} }

func (builder *Builder) Build(id int64, reportType domain.ReportType, at time.Time, location *time.Location, events []EventSnapshot) (domain.Report, error) {
	if builder == nil || id <= 0 {
		return domain.Report{}, fmt.Errorf("invalid report builder")
	}
	period, err := domain.PeriodFor(at, reportType, location)
	if err != nil {
		return domain.Report{}, err
	}
	items := make([]domain.Item, 0, len(events))
	for _, event := range events {
		items = append(items, domain.Item{EventID: event.EventID, EventUpdateID: event.EventUpdateID, Title: event.Title, Summary: event.Summary, InclusionReason: "period_latest_event_update", HeatScore: event.HeatScore, EvidenceSetHash: event.EvidenceSetHash, ReasonCodes: append([]string(nil), event.ReasonCodes...)})
	}
	items = domain.SortItems(items)
	title := "日报"
	if reportType == domain.ReportWeekly {
		title = "周报"
	}
	report := domain.Report{ID: id, Version: 1, VersionNo: 1, Type: reportType, Period: period, Title: title, Status: domain.ReportDraft, Items: items}
	if err := report.Validate(); err != nil {
		return domain.Report{}, err
	}
	return report, nil
}

func (builder *Builder) Publish(report domain.Report) (domain.Report, error) {
	if err := report.Validate(); err != nil {
		return domain.Report{}, err
	}
	report.Status = domain.ReportPublished
	report.Frozen = true
	report.Version++
	return report, nil
}
