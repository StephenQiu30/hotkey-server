package report

import (
	"context"
	"sort"
	"sync"
)

type MemoryReportRepository struct {
	mu        sync.RWMutex
	reports   map[string]DailyReport
	summaries map[string]AISummary
}

func NewMemoryReportRepository() *MemoryReportRepository {
	return &MemoryReportRepository{
		reports:   map[string]DailyReport{},
		summaries: map[string]AISummary{},
	}
}

func (r *MemoryReportRepository) SaveReport(_ context.Context, report DailyReport) (DailyReport, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reports[report.ID] = cloneReport(report)
	return cloneReport(report), nil
}

func (r *MemoryReportRepository) FindReportByDateChannelUser(_ context.Context, date string, channelID string, userID string) (DailyReport, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, report := range r.reports {
		if report.Date == date && report.ChannelID == channelID && report.UserID == userID {
			return cloneReport(report), nil
		}
	}
	return DailyReport{}, ErrNotFound
}

func (r *MemoryReportRepository) FindReportByID(_ context.Context, id string) (DailyReport, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	report, ok := r.reports[id]
	if !ok {
		return DailyReport{}, ErrNotFound
	}
	return cloneReport(report), nil
}

func (r *MemoryReportRepository) ListReportsByDate(_ context.Context, date string) ([]DailyReport, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	reports := []DailyReport{}
	for _, report := range r.reports {
		if report.Date == date {
			reports = append(reports, cloneReport(report))
		}
	}
	sort.Slice(reports, func(i, j int) bool {
		if reports[i].CreatedAt.Equal(reports[j].CreatedAt) {
			return reports[i].ID < reports[j].ID
		}
		return reports[i].CreatedAt.Before(reports[j].CreatedAt)
	})
	return reports, nil
}

func (r *MemoryReportRepository) SaveSummary(_ context.Context, summary AISummary) (AISummary, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.summaries[summary.ClusterID] = cloneSummary(summary)
	return cloneSummary(summary), nil
}

func (r *MemoryReportRepository) FindSummaryByClusterID(_ context.Context, clusterID string) (AISummary, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	summary, ok := r.summaries[clusterID]
	if !ok {
		return AISummary{}, ErrNotFound
	}
	return cloneSummary(summary), nil
}

func cloneReport(report DailyReport) DailyReport {
	report.InputHotspotIDs = append([]string(nil), report.InputHotspotIDs...)
	report.SourceRefs = append([]SourceRef(nil), report.SourceRefs...)
	return report
}

func cloneSummary(summary AISummary) AISummary {
	summary.SourceRefs = append([]SourceRef(nil), summary.SourceRefs...)
	return summary
}
