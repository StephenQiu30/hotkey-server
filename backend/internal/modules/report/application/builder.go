// Package application implements Report use cases and consumer-owned ports.
package application

import (
	"fmt"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/report/domain"
)

type EventSnapshot struct {
	// EventID/EventUpdateID are retained for legacy report reads and tests.
	// New report builds always receive the Product Event v2 identity below.
	EventID, EventUpdateID                              int64
	MicroEventID, MicroEventVersion, MicroEventUpdateID int64
	MicroEventSummaryID                                 int64
	Title, Summary                                      string
	HeatScore                                           float64
	EvidenceSetHash                                     string
	ReasonCodes                                         []string
	Sentences                                           []domain.Sentence
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
		item := domain.Item{EventID: event.EventID, EventUpdateID: event.EventUpdateID, Title: event.Title,
			Summary: event.Summary, InclusionReason: "period_latest_event_update", HeatScore: event.HeatScore,
			EvidenceSetHash: event.EvidenceSetHash, ReasonCodes: append([]string(nil), event.ReasonCodes...)}
		if event.MicroEventID > 0 {
			item.EventID, item.EventUpdateID = 0, 0
			item.MicroEventID, item.MicroEventVersion = event.MicroEventID, event.MicroEventVersion
			item.MicroEventUpdateID, item.MicroEventSummaryID = event.MicroEventUpdateID, event.MicroEventSummaryID
			item.InclusionReason = "period_latest_product_event_update"
			item.Sentences = cloneReportSentences(event.Sentences)
		}
		items = append(items, item)
	}
	items = domain.SortItems(items)
	title := "日报"
	if reportType == domain.ReportWeekly {
		title = "周报"
	}
	report := domain.Report{ID: id, Version: 1, VersionNo: 1, Type: reportType, Period: period, Title: title, Status: domain.ReportDraft, Items: items}
	report.InputSnapshotHash = domain.ComputeInputSnapshotHash(report)
	if err := report.Validate(); err != nil {
		return domain.Report{}, err
	}
	return report, nil
}

func cloneReportSentences(sentences []domain.Sentence) []domain.Sentence {
	result := append([]domain.Sentence(nil), sentences...)
	for index := range result {
		result[index].ClaimEvidenceVersionIDs = append([]int64(nil), sentences[index].ClaimEvidenceVersionIDs...)
	}
	return result
}
