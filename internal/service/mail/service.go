package mail

import (
	"context"
	"errors"
	"fmt"
	"html"
	"strings"
	"time"
)

var ErrNotFound = errors.New("not found")

type DeliveryStatus string

const (
	DeliveryStatusPending      DeliveryStatus = "pending"
	DeliveryStatusSent         DeliveryStatus = "sent"
	DeliveryStatusFailed       DeliveryStatus = "failed"
	DeliveryStatusFailedConfig DeliveryStatus = "failed_config"
	DeliveryStatusSkipped      DeliveryStatus = "skipped"
)

type ReportType string

const (
	ReportTypeDaily  ReportType = "daily"
	ReportTypeWeekly ReportType = "weekly"
)

type Config struct {
	Host       string
	Port       int
	Username   string
	Password   string
	From       string
	TLS        bool
	StartTLS   bool
	Configured bool
}

type Recipient struct {
	UserID        string
	Email         string
	EmailEnabled  bool
	WeeklyEnabled bool
	DailySendAt   string
	WeeklySendAt  string
}

type DailyReport struct {
	ID             string
	ReportDate     string
	ReportType     ReportType
	DailyReportIDs []string
	Title          string
	Summary        string
	BodyMarkdown   string
	BodyHTML       string
	URL            string
}

type Delivery struct {
	ID              string
	RecipientUserID string
	RecipientEmail  string
	ReportID        string
	Status          DeliveryStatus
	Attempt         int
	LastError       string
	SentAt          *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type Message struct {
	From     string
	To       string
	Subject  string
	TextBody string
	HTMLBody string
}

type Mailer interface {
	Send(context.Context, Message) error
}

type Repository interface {
	DailyReportByID(context.Context, string) (DailyReport, error)
	RecipientByUserID(context.Context, string) (Recipient, error)
	CreateDelivery(context.Context, Delivery) (Delivery, error)
	UpdateDelivery(context.Context, Delivery) (Delivery, error)
	FindDeliveryByReportAndUser(ctx context.Context, reportID, userID string) (Delivery, error)
}

type Service struct {
	repo   Repository
	mailer Mailer
	cfg    Config
	now    func() time.Time
}

type Option func(*Service)

func WithNow(now func() time.Time) Option {
	return func(s *Service) {
		if now != nil {
			s.now = now
		}
	}
}

func NewService(repo Repository, mailer Mailer, cfg Config, opts ...Option) *Service {
	service := &Service{repo: repo, mailer: mailer, cfg: cfg, now: time.Now}
	for _, opt := range opts {
		opt(service)
	}
	return service
}

type SendDailyEmailInput struct {
	ReportID        string
	RecipientUserID string
	Attempt         int
}

type SendWeeklyEmailInput struct {
	ReportID        string
	RecipientUserID string
	Attempt         int
}

func (s *Service) SendDailyEmail(ctx context.Context, input SendDailyEmailInput) (Delivery, error) {
	if s.repo == nil {
		return Delivery{}, errors.New("mail service requires repository")
	}
	recipient, err := s.repo.RecipientByUserID(ctx, input.RecipientUserID)
	if err != nil {
		return Delivery{}, err
	}
	report, err := s.repo.DailyReportByID(ctx, input.ReportID)
	if err != nil {
		return Delivery{}, err
	}

	delivery, err := s.repo.CreateDelivery(ctx, Delivery{
		RecipientUserID: recipient.UserID,
		RecipientEmail:  recipient.Email,
		ReportID:        report.ID,
		Status:          DeliveryStatusPending,
		Attempt:         input.Attempt,
	})
	if err != nil {
		return Delivery{}, err
	}

	if !recipient.EmailEnabled {
		delivery.Status = DeliveryStatusFailed
		delivery.LastError = "recipient email disabled"
		return s.repo.UpdateDelivery(ctx, delivery)
	}
	if !s.cfg.Configured || strings.TrimSpace(s.cfg.Host) == "" || strings.TrimSpace(s.cfg.From) == "" {
		delivery.Status = DeliveryStatusFailedConfig
		delivery.LastError = "smtp missing_config"
		return s.repo.UpdateDelivery(ctx, delivery)
	}
	if s.mailer == nil {
		delivery.Status = DeliveryStatusFailedConfig
		delivery.LastError = "smtp mailer missing_config"
		return s.repo.UpdateDelivery(ctx, delivery)
	}

	message := BuildDailyReportMessage(s.cfg.From, recipient.Email, report)
	if err := s.mailer.Send(ctx, message); err != nil {
		delivery.Status = DeliveryStatusFailed
		delivery.LastError = err.Error()
		updated, updateErr := s.repo.UpdateDelivery(ctx, delivery)
		if updateErr != nil {
			return updated, updateErr
		}
		return updated, err
	}

	sentAt := s.now().UTC()
	delivery.Status = DeliveryStatusSent
	delivery.LastError = ""
	delivery.SentAt = &sentAt
	return s.repo.UpdateDelivery(ctx, delivery)
}

func (s *Service) SendWeeklyEmail(ctx context.Context, input SendWeeklyEmailInput) (Delivery, error) {
	if s.repo == nil {
		return Delivery{}, errors.New("mail service requires repository")
	}
	recipient, err := s.repo.RecipientByUserID(ctx, input.RecipientUserID)
	if err != nil {
		return Delivery{}, err
	}
	report, err := s.repo.DailyReportByID(ctx, input.ReportID)
	if err != nil {
		return Delivery{}, err
	}

	delivery, err := s.repo.CreateDelivery(ctx, Delivery{
		RecipientUserID: recipient.UserID,
		RecipientEmail:  recipient.Email,
		ReportID:        report.ID,
		Status:          DeliveryStatusPending,
		Attempt:         input.Attempt,
	})
	if err != nil {
		return Delivery{}, err
	}

	if !recipient.EmailEnabled || !recipient.WeeklyEnabled {
		delivery.Status = DeliveryStatusSkipped
		delivery.LastError = "recipient weekly email disabled"
		return s.repo.UpdateDelivery(ctx, delivery)
	}
	if !s.cfg.Configured || strings.TrimSpace(s.cfg.Host) == "" || strings.TrimSpace(s.cfg.From) == "" {
		delivery.Status = DeliveryStatusFailedConfig
		delivery.LastError = "smtp missing_config"
		return s.repo.UpdateDelivery(ctx, delivery)
	}
	if s.mailer == nil {
		delivery.Status = DeliveryStatusFailedConfig
		delivery.LastError = "smtp mailer missing_config"
		return s.repo.UpdateDelivery(ctx, delivery)
	}

	message := BuildWeeklyReportMessage(s.cfg.From, recipient.Email, report)
	if err := s.mailer.Send(ctx, message); err != nil {
		delivery.Status = DeliveryStatusFailed
		delivery.LastError = err.Error()
		updated, updateErr := s.repo.UpdateDelivery(ctx, delivery)
		if updateErr != nil {
			return updated, updateErr
		}
		return updated, err
	}

	sentAt := s.now().UTC()
	delivery.Status = DeliveryStatusSent
	delivery.LastError = ""
	delivery.SentAt = &sentAt
	return s.repo.UpdateDelivery(ctx, delivery)
}

func BuildDailyReportMessage(from string, to string, report DailyReport) Message {
	title := strings.TrimSpace(report.Title)
	if title == "" {
		title = "HotKey AI 热点日报"
	}
	subject := fmt.Sprintf("[HotKey] %s", title)
	if report.ReportDate != "" && !strings.Contains(subject, report.ReportDate) {
		subject += " - " + report.ReportDate
	}
	textParts := []string{title}
	if report.Summary != "" {
		textParts = append(textParts, "", report.Summary)
	}
	if report.BodyMarkdown != "" {
		textParts = append(textParts, "", report.BodyMarkdown)
	}
	if report.URL != "" {
		textParts = append(textParts, "", "查看日报: "+report.URL)
	}
	textBody := strings.Join(textParts, "\n")

	htmlBody := strings.TrimSpace(report.BodyHTML)
	if htmlBody == "" {
		htmlBody = "<h1>" + html.EscapeString(title) + "</h1>"
		if report.Summary != "" {
			htmlBody += "<p>" + html.EscapeString(report.Summary) + "</p>"
		}
		if report.BodyMarkdown != "" {
			htmlBody += "<pre>" + html.EscapeString(report.BodyMarkdown) + "</pre>"
		}
		if report.URL != "" {
			safeURL := html.EscapeString(report.URL)
			htmlBody += `<p><a href="` + safeURL + `">查看日报</a></p>`
		}
	}
	return Message{From: from, To: to, Subject: subject, TextBody: textBody, HTMLBody: htmlBody}
}

func BuildWeeklyReportMessage(from string, to string, report DailyReport) Message {
	title := strings.TrimSpace(report.Title)
	if title == "" {
		title = "HotKey AI 热点周报"
	}
	subject := fmt.Sprintf("[HotKey 周报] %s", title)
	if report.ReportDate != "" && !strings.Contains(subject, report.ReportDate) {
		subject += " - " + report.ReportDate
	}
	textParts := []string{title}
	if report.Summary != "" {
		textParts = append(textParts, "", report.Summary)
	}
	if report.BodyMarkdown != "" {
		textParts = append(textParts, "", report.BodyMarkdown)
	}
	if report.URL != "" {
		textParts = append(textParts, "", "查看周报: "+report.URL)
	}
	textBody := strings.Join(textParts, "\n")

	htmlBody := strings.TrimSpace(report.BodyHTML)
	if htmlBody == "" {
		htmlBody = "<h1>" + html.EscapeString(title) + "</h1>"
		if report.Summary != "" {
			htmlBody += "<p>" + html.EscapeString(report.Summary) + "</p>"
		}
		if report.BodyMarkdown != "" {
			htmlBody += "<pre>" + html.EscapeString(report.BodyMarkdown) + "</pre>"
		}
		if report.URL != "" {
			safeURL := html.EscapeString(report.URL)
			htmlBody += `<p><a href="` + safeURL + `">查看周报</a></p>`
		}
	}
	return Message{From: from, To: to, Subject: subject, TextBody: textBody, HTMLBody: htmlBody}
}
