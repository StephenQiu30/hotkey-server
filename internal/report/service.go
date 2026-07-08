package report

import (
	"context"
	"errors"
	"fmt"
	"html"
	"regexp"
	"strings"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/model/dto"
)

const (
	TypeDaily  = "daily"
	TypeWeekly = "weekly"

	StatusDraft = "draft"
	StatusSent  = "sent"

	defaultLimit = 50
)

var (
	ErrInvalidInput    = errors.New("invalid report input")
	ErrNoReportSources = errors.New("no report sources")
	ErrNotFound        = errors.New("report not found")
	ErrUnsupportedType = errors.New("unsupported report type")
)

type Service struct {
	repo Repository
	now  func() time.Time
}

func NewService(repo Repository, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{repo: repo, now: now}
}

func (s *Service) Create(ctx context.Context, userID int64, input dto.CreateInput) (dto.Report, error) {
	reportType := input.ReportType
	if reportType == "" {
		reportType = TypeDaily
	}
	start, end, err := s.resolvePeriod(reportType, input.PeriodStart, input.PeriodEnd)
	if err != nil {
		return dto.Report{}, err
	}

	monitors, err := s.repo.ListUserMonitors(ctx, userID)
	if err != nil {
		return dto.Report{}, err
	}
	if len(monitors) == 0 {
		return dto.Report{}, ErrNoReportSources
	}
	if input.MonitorID > 0 {
		filtered := make([]dto.MonitorSource, 0, 1)
		for _, m := range monitors {
			if m.ID == input.MonitorID {
				filtered = append(filtered, m)
			}
		}
		if len(filtered) == 0 {
			return dto.Report{}, ErrNoReportSources
		}
		monitors = filtered
	}

	monitorIDs := make([]int64, 0, len(monitors))
	for _, m := range monitors {
		monitorIDs = append(monitorIDs, m.ID)
	}

	topics, err := s.repo.ListTopics(ctx, monitorIDs, start, end, defaultLimit)
	if err != nil {
		return dto.Report{}, err
	}
	posts, err := s.repo.ListPosts(ctx, monitorIDs, start, end, defaultLimit)
	if err != nil {
		return dto.Report{}, err
	}

	subject := buildSubject(reportType, monitors, start, end)
	summary := buildSummary(reportType, len(topics), len(posts))
	content := renderMarkdown(subject, reportType, start, end, summary, monitors, topics, posts)

	rec := dto.CreateReportRecord{
		UserID:       userID,
		ReportType:   reportType,
		PeriodStart:  start,
		PeriodEnd:    end,
		Subject:      subject,
		Summary:      summary,
		Content:      content,
		HotspotCount: len(topics) + len(posts),
		Status:       StatusDraft,
	}
	created, err := s.repo.Create(ctx, rec)
	if err != nil {
		return dto.Report{}, err
	}
	if input.Send {
		return s.repo.MarkSent(ctx, created.ID, userID, s.now())
	}
	return created, nil
}

func (s *Service) List(ctx context.Context, userID int64, filter dto.ListFilter) ([]dto.Report, int64, error) {
	filter.UserID = userID
	if filter.Limit <= 0 || filter.Limit > 100 {
		filter.Limit = defaultLimit
	}
	return s.repo.List(ctx, filter)
}

func (s *Service) GetByID(ctx context.Context, id, userID int64) (dto.Report, error) {
	return s.repo.GetByID(ctx, id, userID)
}

func (s *Service) HTML(ctx context.Context, id, userID int64) (string, error) {
	item, err := s.GetByID(ctx, id, userID)
	if err != nil {
		return "", err
	}
	return markdownToHTML(item.Content), nil
}

func (s *Service) MarkSent(ctx context.Context, id, userID int64) (dto.Report, error) {
	return s.repo.MarkSent(ctx, id, userID, s.now())
}

func (s *Service) resolvePeriod(reportType string, start, end *time.Time) (time.Time, time.Time, error) {
	switch reportType {
	case TypeDaily:
		periodStart := startOfDay(s.now().AddDate(0, 0, -1))
		if start != nil {
			periodStart = startOfDay(*start)
		}
		periodEnd := periodStart
		if end != nil {
			periodEnd = startOfDay(*end)
		}
		return periodStart, periodEnd, nil
	case TypeWeekly:
		periodStart := startOfDay(s.now().AddDate(0, 0, -7))
		if start != nil {
			periodStart = startOfDay(*start)
		}
		periodEnd := periodStart.AddDate(0, 0, 6)
		if end != nil {
			periodEnd = startOfDay(*end)
		}
		if periodEnd.Before(periodStart) {
			return time.Time{}, time.Time{}, ErrInvalidInput
		}
		return periodStart, periodEnd, nil
	default:
		return time.Time{}, time.Time{}, ErrUnsupportedType
	}
}

func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func buildSubject(reportType string, monitors []dto.MonitorSource, start, end time.Time) string {
	name := "HotKey"
	if len(monitors) == 1 {
		name = monitors[0].Name
	}
	label := "日报"
	if reportType == TypeWeekly {
		label = "周报"
	}
	return fmt.Sprintf("%s %s %s-%s", name, label, start.Format("2006-01-02"), end.Format("2006-01-02"))
}

func buildSummary(reportType string, topicCount, postCount int) string {
	period := "本日"
	if reportType == TypeWeekly {
		period = "本周"
	}
	return fmt.Sprintf("%s共跟踪 %d 个热点主题、%d 条代表内容。", period, topicCount, postCount)
}

func renderMarkdown(subject, reportType string, start, end time.Time, summary string, monitors []dto.MonitorSource, topics []dto.TopicSource, posts []dto.PostSource) string {
	overviewTitle := "今日概览"
	if reportType == TypeWeekly {
		overviewTitle = "本周概览"
	}

	var b strings.Builder
	b.WriteString("# " + subject + "\n\n")
	b.WriteString(fmt.Sprintf("周期：%s 至 %s\n\n", start.Format("2006-01-02"), end.Format("2006-01-02")))
	b.WriteString("## " + overviewTitle + "\n\n")
	b.WriteString(summary + "\n\n")
	b.WriteString(fmt.Sprintf("- 监控数：%d\n", len(monitors)))
	b.WriteString(fmt.Sprintf("- 热点主题数：%d\n", len(topics)))
	b.WriteString(fmt.Sprintf("- 代表内容数：%d\n\n", len(posts)))

	b.WriteString("## 热点主题\n\n")
	if len(topics) == 0 {
		b.WriteString("暂无热点主题。\n\n")
	} else {
		for i, topic := range topics {
			b.WriteString(fmt.Sprintf("### %d. %s\n", i+1, topic.Title))
			b.WriteString(fmt.Sprintf("- 热度：%.1f\n", topic.HeatScore))
			b.WriteString(fmt.Sprintf("- 趋势：%s\n", fallback(topic.Trend, "stable")))
			b.WriteString(fmt.Sprintf("- 相关内容数：%d\n", topic.PostCount))
			if topic.Summary != "" {
				b.WriteString(fmt.Sprintf("- 摘要：%s\n", topic.Summary))
			}
			b.WriteString("\n")
		}
	}

	b.WriteString("## 代表内容\n\n")
	if len(posts) == 0 {
		b.WriteString("暂无代表内容。\n")
	} else {
		for i, post := range posts {
			title := fallback(post.Title, snippet(post.Content, 40))
			b.WriteString(fmt.Sprintf("### %d. %s\n", i+1, title))
			b.WriteString(fmt.Sprintf("- 平台：%s\n", fallback(post.Platform, "unknown")))
			b.WriteString(fmt.Sprintf("- 热度：%.1f\n", post.HeatScore))
			if post.Content != "" {
				b.WriteString(fmt.Sprintf("- 摘要：%s\n", snippet(post.Content, 120)))
			}
			if post.URL != "" {
				b.WriteString(fmt.Sprintf("- 链接：%s\n", post.URL))
			}
			b.WriteString("\n")
		}
	}
	return b.String()
}

func fallback(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func snippet(value string, max int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= max {
		return string(runes)
	}
	return string(runes[:max]) + "..."
}

func markdownToHTML(markdown string) string {
	lines := strings.Split(markdown, "\n")
	var b strings.Builder
	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "### "):
			b.WriteString("<h3>" + html.EscapeString(strings.TrimPrefix(line, "### ")) + "</h3>\n")
		case strings.HasPrefix(line, "## "):
			b.WriteString("<h2>" + html.EscapeString(strings.TrimPrefix(line, "## ")) + "</h2>\n")
		case strings.HasPrefix(line, "# "):
			b.WriteString("<h1>" + html.EscapeString(strings.TrimPrefix(line, "# ")) + "</h1>\n")
		case strings.HasPrefix(line, "- "):
			b.WriteString("<p>" + html.EscapeString(line) + "</p>\n")
		case strings.TrimSpace(line) == "":
		default:
			b.WriteString("<p>" + html.EscapeString(linkify(line)) + "</p>\n")
		}
	}
	return b.String()
}

var markdownLinkPattern = regexp.MustCompile(`\[(.*?)\]\((.*?)\)`)

func linkify(line string) string {
	return markdownLinkPattern.ReplaceAllString(line, "$1 ($2)")
}
