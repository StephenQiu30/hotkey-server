package report

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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
	hotspots, err := s.gatherHotspotData(ctx, input.ChannelID, "", filter{})
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
	hotspots, err := s.gatherHotspotData(ctx, "", input.UserID, filter{channelIDs: channelIDs, keywords: keywords})
	if err != nil {
		return DailyReport{}, err
	}
	return s.generateReport(ctx, input, hotspots)
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
	hotspots, err := s.gatherHotspotData(ctx, "", "", filter{})
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
