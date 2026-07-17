package application

import (
	"fmt"
	"github.com/StephenQiu30/hotkey-server/internal/modules/report/domain"
	"time"
)

type EventSnapshot struct {
	EventID        int64
	Title, Summary string
	HeatScore      float64
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
		items = append(items, domain.Item{EventID: event.EventID, Title: event.Title, Summary: event.Summary, InclusionReason: "deterministic_heat_snapshot", HeatScore: event.HeatScore})
	}
	items = domain.SortItems(items)
	report := domain.Report{ID: id, Version: 1, VersionNo: 1, Type: reportType, Period: period, Title: fmt.Sprintf("%s report", reportType), Status: domain.ReportDraft, Items: items}
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
