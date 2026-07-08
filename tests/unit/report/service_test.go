package report_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/report"
)

type fakeReportRepo struct {
	monitors []report.MonitorSource
	topics   []report.TopicSource
	posts    []report.PostSource
	saved    []report.Report
}

func (r *fakeReportRepo) ListUserMonitors(_ context.Context, userID int64) ([]report.MonitorSource, error) {
	var out []report.MonitorSource
	for _, m := range r.monitors {
		if m.UserID == userID {
			out = append(out, m)
		}
	}
	return out, nil
}

func (r *fakeReportRepo) ListTopics(_ context.Context, monitorIDs []int64, start, end time.Time, limit int) ([]report.TopicSource, error) {
	return r.topics, nil
}

func (r *fakeReportRepo) ListPosts(_ context.Context, monitorIDs []int64, start, end time.Time, limit int) ([]report.PostSource, error) {
	return r.posts, nil
}

func (r *fakeReportRepo) Create(_ context.Context, in report.CreateReportRecord) (report.Report, error) {
	out := report.Report{
		ID:           int64(len(r.saved) + 1),
		UserID:       in.UserID,
		ReportType:   in.ReportType,
		PeriodStart:  in.PeriodStart,
		PeriodEnd:    in.PeriodEnd,
		Subject:      in.Subject,
		Summary:      in.Summary,
		Content:      in.Content,
		HotspotCount: in.HotspotCount,
		Status:       in.Status,
		CreatedAt:    time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC),
		UpdatedAt:    time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC),
	}
	r.saved = append(r.saved, out)
	return out, nil
}

func (r *fakeReportRepo) List(_ context.Context, filter report.ListFilter) ([]report.Report, int64, error) {
	return r.saved, int64(len(r.saved)), nil
}

func (r *fakeReportRepo) GetByID(_ context.Context, id, userID int64) (report.Report, error) {
	for _, item := range r.saved {
		if item.ID == id && item.UserID == userID {
			return item, nil
		}
	}
	return report.Report{}, report.ErrNotFound
}

func (r *fakeReportRepo) MarkSent(_ context.Context, id, userID int64, sentAt time.Time) (report.Report, error) {
	item, err := r.GetByID(context.Background(), id, userID)
	if err != nil {
		return report.Report{}, err
	}
	item.Status = report.StatusSent
	item.SentAt = &sentAt
	return item, nil
}

func TestServiceCreateWeeklyReportAggregatesTopicsAndPosts(t *testing.T) {
	start := time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC)
	repo := &fakeReportRepo{
		monitors: []report.MonitorSource{{ID: 10, UserID: 7, Name: "AI Regulation"}},
		topics: []report.TopicSource{{
			ID: 1, MonitorID: 10, Title: "AI 监管新规", Summary: "监管框架更新", HeatScore: 92.5, Trend: "rising", PostCount: 4,
		}},
		posts: []report.PostSource{{
			ID: 20, MonitorID: 10, Title: "监管机构发布 AI 指南", Content: "指南要求模型风险评估", URL: "https://example.com/ai", Platform: "x", HeatScore: 88,
		}},
	}

	svc := report.NewService(repo, func() time.Time {
		return time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC)
	})

	got, err := svc.Create(context.Background(), 7, report.CreateInput{
		ReportType:  report.TypeWeekly,
		PeriodStart: &start,
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	if got.ReportType != report.TypeWeekly {
		t.Fatalf("ReportType = %q, want %q", got.ReportType, report.TypeWeekly)
	}
	if got.PeriodEnd.Format("2006-01-02") != "2026-06-30" {
		t.Fatalf("PeriodEnd = %s, want 2026-06-30", got.PeriodEnd.Format("2006-01-02"))
	}
	if got.HotspotCount != 2 {
		t.Fatalf("HotspotCount = %d, want 2", got.HotspotCount)
	}
	for _, want := range []string{"AI Regulation 周报", "## 本周概览", "AI 监管新规", "监管机构发布 AI 指南"} {
		if !strings.Contains(got.Content, want) {
			t.Fatalf("Content missing %q:\n%s", want, got.Content)
		}
	}
	if got.Summary == "" {
		t.Fatal("Summary should not be empty")
	}
}

func TestServiceCreateWeeklyReportRejectsUserWithoutMonitors(t *testing.T) {
	svc := report.NewService(&fakeReportRepo{}, func() time.Time {
		return time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC)
	})

	_, err := svc.Create(context.Background(), 7, report.CreateInput{ReportType: report.TypeWeekly})
	if err != report.ErrNoReportSources {
		t.Fatalf("error = %v, want %v", err, report.ErrNoReportSources)
	}
}

func TestServiceCreateDailyReportWithMonitorID(t *testing.T) {
	start := time.Date(2026, 7, 7, 0, 0, 0, 0, time.UTC)
	repo := &fakeReportRepo{
		monitors: []report.MonitorSource{
			{ID: 10, UserID: 7, Name: "AI Regulation"},
			{ID: 20, UserID: 7, Name: "Tech Policy"},
		},
		topics: []report.TopicSource{{
			ID: 1, MonitorID: 10, Title: "AI 监管新规", Summary: "监管框架更新", HeatScore: 92.5, Trend: "rising", PostCount: 4,
		}},
		posts: []report.PostSource{{
			ID: 20, MonitorID: 10, Title: "监管机构发布 AI 指南", Content: "指南要求模型风险评估", URL: "https://example.com/ai", Platform: "x", HeatScore: 88,
		}},
	}

	svc := report.NewService(repo, func() time.Time {
		return time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC)
	})

	got, err := svc.Create(context.Background(), 7, report.CreateInput{
		ReportType:  report.TypeDaily,
		PeriodStart: &start,
		MonitorID:   10,
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if got.HotspotCount != 2 {
		t.Fatalf("HotspotCount = %d, want 2", got.HotspotCount)
	}
	if !strings.Contains(got.Content, "AI Regulation") {
		t.Fatalf("Content should reference monitor 'AI Regulation':\n%s", got.Content)
	}
	if !strings.Contains(got.Subject, "AI Regulation") {
		t.Fatalf("Subject should contain monitor name: %s", got.Subject)
	}
}
