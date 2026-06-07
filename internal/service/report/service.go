package report

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

type Service struct {
	reports  ReportRepository
	qwen     QwenClient
	clusters ClusterRepository
	scores   ScoreRepository
	sources  SourceRepository
	prefs    PreferenceRepository
	now      func() time.Time
}

func NewService(repo ReportRepository, qwen QwenClient, clusters ClusterRepository, scores ScoreRepository, sources SourceRepository, prefs PreferenceRepository) *Service {
	return &Service{
		reports:  repo,
		qwen:     qwen,
		clusters: clusters,
		scores:   scores,
		sources:  sources,
		prefs:    prefs,
		now:      time.Now,
	}
}

func (s *Service) Repository() ReportRepository {
	return s.reports
}

func (s *Service) SetClock(clock func() time.Time) {
	if clock != nil {
		s.now = clock
	}
}

func (s *Service) GenerateChannelReport(ctx context.Context, input GenerateReportInput) (DailyReport, error) {
	input.Date = strings.TrimSpace(input.Date)
	input.ChannelID = strings.TrimSpace(input.ChannelID)
	if input.Date == "" || input.ChannelID == "" {
		return DailyReport{}, ErrInvalidInput
	}
	if existing, err := s.reports.FindReportByDateChannelUser(ctx, input.Date, input.ChannelID, ""); err == nil {
		return existing, nil
	} else if !errorsIsNotFound(err) {
		return DailyReport{}, err
	}
	hotspots, err := s.gatherHotspotData(ctx, input.ChannelID, filter{})
	if err != nil {
		return DailyReport{}, err
	}
	return s.generateReport(ctx, input, hotspots)
}

func (s *Service) GenerateUserReport(ctx context.Context, input GenerateReportInput) (DailyReport, error) {
	input.Date = strings.TrimSpace(input.Date)
	input.UserID = strings.TrimSpace(input.UserID)
	if input.Date == "" || input.UserID == "" {
		return DailyReport{}, ErrInvalidInput
	}
	if existing, err := s.reports.FindReportByDateChannelUser(ctx, input.Date, "", input.UserID); err == nil {
		return existing, nil
	} else if !errorsIsNotFound(err) {
		return DailyReport{}, err
	}
	channelIDs, keywords, err := s.userFilters(ctx, input.UserID)
	if err != nil {
		return DailyReport{}, err
	}
	hotspots, err := s.gatherHotspotData(ctx, "", filter{channelIDs: channelIDs, keywords: keywords})
	if err != nil {
		return DailyReport{}, err
	}
	return s.generateReport(ctx, input, hotspots)
}

func (s *Service) GenerateWeeklyReport(ctx context.Context, input GenerateWeeklyReportInput) (WeeklyReport, error) {
	input.WeekOf = strings.TrimSpace(input.WeekOf)
	input.ChannelID = strings.TrimSpace(input.ChannelID)
	if input.WeekOf == "" {
		return WeeklyReport{}, ErrInvalidInput
	}
	startDate, endDate, err := parseWeekRange(input.WeekOf)
	if err != nil {
		return WeeklyReport{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	dailyReports, err := s.reports.ListReportsByDateRange(ctx, startDate, endDate, input.ChannelID)
	if err != nil {
		return WeeklyReport{}, err
	}
	if len(dailyReports) == 0 {
		now := s.now().UTC()
		return WeeklyReport{
			DailyReport: DailyReport{
				ID:             newID("wrpt"),
				Date:           input.WeekOf,
				ReportType:     "weekly",
				ChannelID:      input.ChannelID,
				UserID:         input.UserID,
				Body:           "本周无日报数据，暂不生成周报。",
				Status:         ReportStatusDegraded,
				SourceRefs:     []SourceRef{},
				DailyReportIDs: []string{},
				CreatedAt:      now,
				UpdatedAt:      now,
			},
		}, nil
	}
	var dailyIDs []string
	var allRefs []SourceRef
	seen := map[string]bool{}
	for _, dr := range dailyReports {
		dailyIDs = append(dailyIDs, dr.ID)
		for _, ref := range dr.SourceRefs {
			key := ref.SourceID + ":" + ref.ItemID
			if !seen[key] {
				seen[key] = true
				allRefs = append(allRefs, ref)
			}
		}
	}
	prompt := BuildWeeklyReportPrompt(input.WeekOf, dailyReports)
	body, err := s.callQwen(ctx, prompt, allRefs)
	status := ReportStatusSucceeded
	lastError := ""
	if err != nil {
		status = ReportStatusDegraded
		if errorsIs(err, ErrFailedConfig) {
			status = ReportStatusFailedConfig
		}
		lastError = err.Error()
		body = buildDegradedWeeklyReportBody(input.WeekOf, dailyReports, allRefs, status)
	}
	now := s.now().UTC()
	report := DailyReport{
		ID:             newID("wrpt"),
		Date:           input.WeekOf,
		ReportType:     "weekly",
		ChannelID:      input.ChannelID,
		UserID:         input.UserID,
		PromptVersion:  PromptVersion,
		Body:           body,
		Status:         status,
		LastError:      lastError,
		SourceRefs:     allRefs,
		DailyReportIDs: dailyIDs,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	saved, err := s.reports.SaveReport(ctx, report)
	if err != nil {
		return WeeklyReport{}, err
	}
	return WeeklyReport{DailyReport: saved}, nil
}

func (s *Service) GenerateSummary(ctx context.Context, input GenerateSummaryInput) (AISummary, error) {
	clusterID := strings.TrimSpace(input.ClusterID)
	if clusterID == "" {
		return AISummary{}, ErrInvalidInput
	}
	if existing, err := s.reports.FindSummaryByClusterID(ctx, clusterID); err == nil {
		return existing, nil
	} else if !errorsIsNotFound(err) {
		return AISummary{}, err
	}
	hotspots, err := s.gatherHotspotData(ctx, "", filter{})
	if err != nil {
		return AISummary{}, err
	}
	var selected *HotspotData
	for i := range hotspots {
		if hotspots[i].Cluster.ID == clusterID {
			selected = &hotspots[i]
			break
		}
	}
	if selected == nil {
		return AISummary{}, ErrNotFound
	}
	refs := sourceRefs([]HotspotData{*selected})
	prompt := BuildSummaryPrompt(*selected)
	body, err := s.callQwen(ctx, prompt, refs)
	if err != nil {
		return s.saveDegradedSummary(ctx, clusterID, refs, err.Error())
	}
	now := s.now().UTC()
	return s.reports.SaveSummary(ctx, AISummary{
		ID:            newID("sum"),
		ClusterID:     clusterID,
		PromptVersion: PromptVersion,
		Summary:       body,
		Status:        ReportStatusSucceeded,
		SourceRefs:    refs,
		CreatedAt:     now,
		UpdatedAt:     now,
	})
}

func (s *Service) generateReport(ctx context.Context, input GenerateReportInput, hotspots []HotspotData) (DailyReport, error) {
	refs := sourceRefs(hotspots)
	hotspotIDs := hotspotIDs(hotspots)
	prompt := BuildDailyReportPrompt(input.Date, hotspots)
	body, err := s.callQwen(ctx, prompt, refs)
	status := ReportStatusSucceeded
	lastError := ""
	if err != nil {
		status = ReportStatusDegraded
		if errorsIs(err, ErrFailedConfig) {
			status = ReportStatusFailedConfig
		}
		lastError = err.Error()
		body = buildDegradedReportBody(input.Date, hotspots, refs, status)
	}
	now := s.now().UTC()
	return s.reports.SaveReport(ctx, DailyReport{
		ID:              newID("rpt"),
		Date:            input.Date,
		ChannelID:       input.ChannelID,
		UserID:          input.UserID,
		PromptVersion:   PromptVersion,
		InputHotspotIDs: hotspotIDs,
		Body:            body,
		Status:          status,
		LastError:       lastError,
		SourceRefs:      refs,
		CreatedAt:       now,
		UpdatedAt:       now,
	})
}

func (s *Service) callQwen(ctx context.Context, prompt string, refs []SourceRef) (string, error) {
	if len(refs) < 2 {
		return "", ErrInsufficientEvidence
	}
	if s.qwen == nil {
		return "", ErrFailedConfig
	}
	body, err := s.qwen.GenerateReport(ctx, prompt)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(body) == "" {
		return "", ErrInsufficientEvidence
	}
	return body, nil
}

func (s *Service) saveDegradedSummary(ctx context.Context, clusterID string, refs []SourceRef, reason string) (AISummary, error) {
	now := s.now().UTC()
	return s.reports.SaveSummary(ctx, AISummary{
		ID:            newID("sum"),
		ClusterID:     clusterID,
		PromptVersion: PromptVersion,
		Summary:       "证据不足，暂不生成影响分析。\n\n来源引用：\n" + formatRefs(refs),
		Status:        ReportStatusDegraded,
		LastError:     reason,
		SourceRefs:    refs,
		CreatedAt:     now,
		UpdatedAt:     now,
	})
}

func newID(prefix string) string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return prefix + "-" + time.Now().UTC().Format("20060102150405")
	}
	return prefix + "-" + hex.EncodeToString(b[:])
}

// parseWeekRange parses a week identifier like "2026-W23" and returns the start and end dates (YYYY-MM-DD).
func parseWeekRange(weekOf string) (startDate, endDate string, err error) {
	year, week := 0, 0
	if _, parseErr := fmt.Sscanf(weekOf, "%d-W%d", &year, &week); parseErr != nil {
		return "", "", fmt.Errorf("invalid week format %q: %w", weekOf, parseErr)
	}
	if year < 2000 || year > 2100 || week < 1 || week > 53 {
		return "", "", fmt.Errorf("invalid week range: year=%d week=%d", year, week)
	}
	// ISO week: find the Thursday of the given week, then derive Monday and Sunday
	jan4 := time.Date(year, 1, 4, 0, 0, 0, 0, time.UTC)
	// Find the Monday of week 1
	weekday := jan4.Weekday()
	if weekday == time.Sunday {
		weekday = 7
	}
	mondayWeek1 := jan4.Add(-time.Duration(weekday-1) * 24 * time.Hour)
	start := mondayWeek1.Add(time.Duration(week-1) * 7 * 24 * time.Hour)
	end := start.Add(6 * 24 * time.Hour)
	// Validate the computed date actually belongs to the requested ISO week
	thursday := start.Add(3 * 24 * time.Hour)
	isoYear, isoWeek := thursday.ISOWeek()
	if isoYear != year || isoWeek != week {
		return "", "", fmt.Errorf("invalid ISO week: %d-W%02d does not exist", year, week)
	}
	return start.Format("2006-01-02"), end.Format("2006-01-02"), nil
}
