package mail

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestSendDailyEmailUsesFakeMailerAndRecordsSentDelivery(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 5, 31, 8, 30, 0, 0, time.UTC)
	repo := newMemoryRepository(now)
	repo.reports["report-1"] = DailyReport{
		ID:           "report-1",
		ReportDate:   "2026-05-31",
		Title:        "AI 热点日报",
		Summary:      "今日重点：模型发布和企业落地。",
		BodyMarkdown: "## 今日摘要\n\n中文日报正文。",
		URL:          "https://hotkey.example/reports/report-1",
	}
	repo.recipients["user-1"] = Recipient{
		UserID:       "user-1",
		Email:        "reader@example.com",
		EmailEnabled: true,
	}
	fake := &fakeMailer{}
	service := NewService(repo, fake, Config{
		Host:       "smtp.example.com",
		Port:       587,
		From:       "HotKey <daily@example.com>",
		StartTLS:   true,
		Configured: true,
	}, WithNow(func() time.Time { return now }))

	delivery, err := service.SendDailyEmail(ctx, SendDailyEmailInput{
		ReportID:        "report-1",
		RecipientUserID: "user-1",
		Attempt:         2,
	})
	if err != nil {
		t.Fatalf("send daily email failed: %v", err)
	}

	if delivery.Status != DeliveryStatusSent {
		t.Fatalf("expected sent delivery, got %#v", delivery)
	}
	if delivery.Attempt != 2 || delivery.SentAt == nil || !delivery.SentAt.Equal(now) {
		t.Fatalf("expected attempt and sent_at to be recorded, got %#v", delivery)
	}
	if len(fake.messages) != 1 {
		t.Fatalf("expected fake mailer to receive one message, got %d", len(fake.messages))
	}
	message := fake.messages[0]
	if message.To != "reader@example.com" || message.From != "HotKey <daily@example.com>" {
		t.Fatalf("unexpected message addresses: %#v", message)
	}
	for _, want := range []string{"AI 热点日报", "今日重点", "中文日报正文", "https://hotkey.example/reports/report-1"} {
		if !strings.Contains(message.Subject+message.TextBody+message.HTMLBody, want) {
			t.Fatalf("expected message to include %q, got %#v", want, message)
		}
	}
}

func TestSendDailyEmailMarksFailedConfigWhenSMTPMissing(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryRepository(time.Now())
	repo.reports["report-1"] = DailyReport{ID: "report-1", Title: "日报", Summary: "摘要", BodyMarkdown: "正文"}
	repo.recipients["user-1"] = Recipient{UserID: "user-1", Email: "reader@example.com", EmailEnabled: true}
	service := NewService(repo, &fakeMailer{}, Config{})

	delivery, err := service.SendDailyEmail(ctx, SendDailyEmailInput{
		ReportID:        "report-1",
		RecipientUserID: "user-1",
		Attempt:         1,
	})
	if err != nil {
		t.Fatalf("missing SMTP config should not return retry error: %v", err)
	}
	if delivery.Status != DeliveryStatusFailedConfig {
		t.Fatalf("expected failed_config delivery, got %#v", delivery)
	}
	if !strings.Contains(delivery.LastError, "smtp") {
		t.Fatalf("expected config error to mention smtp, got %q", delivery.LastError)
	}
}

func TestSendDailyEmailRecordsFailedDeliveryAndReturnsRetryError(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryRepository(time.Now())
	repo.reports["report-1"] = DailyReport{ID: "report-1", Title: "日报", Summary: "摘要", BodyMarkdown: "正文"}
	repo.recipients["user-1"] = Recipient{UserID: "user-1", Email: "reader@example.com", EmailEnabled: true}
	fake := &fakeMailer{err: errors.New("smtp rejected")}
	service := NewService(repo, fake, Config{Host: "smtp.example.com", Port: 587, From: "daily@example.com", Configured: true})

	delivery, err := service.SendDailyEmail(ctx, SendDailyEmailInput{
		ReportID:        "report-1",
		RecipientUserID: "user-1",
		Attempt:         3,
	})
	if err == nil {
		t.Fatal("expected retry error from failed mailer")
	}
	if delivery.Status != DeliveryStatusFailed || delivery.LastError != "smtp rejected" {
		t.Fatalf("expected failed delivery record, got %#v", delivery)
	}
}

func TestBuildWeeklyReportMessageIncludesWeekRangeAndUnsubscribeLink(t *testing.T) {
	report := DailyReport{
		ID:           "wr-1",
		ReportDate:   "2026-W23",
		ReportType:   ReportTypeWeekly,
		Title:        "AI 热点周报",
		Summary:      "本周重点：大模型竞赛白热化。",
		BodyMarkdown: "## 本周摘要\n\n周报正文内容。",
		URL:          "https://hotkey.example/reports/wr-1",
	}
	msg := BuildWeeklyReportMessage("HotKey <weekly@example.com>", "reader@example.com", report)
	if msg.From != "HotKey <weekly@example.com>" {
		t.Fatalf("unexpected from: %s", msg.From)
	}
	if msg.To != "reader@example.com" {
		t.Fatalf("unexpected to: %s", msg.To)
	}
	for _, want := range []string{"[HotKey 周报]", "AI 热点周报", "2026-W23"} {
		if !strings.Contains(msg.Subject, want) {
			t.Fatalf("expected subject to contain %q, got %q", want, msg.Subject)
		}
	}
	if !strings.Contains(msg.HTMLBody, "退订") {
		t.Fatalf("expected HTML body to contain unsubscribe link, got %s", msg.HTMLBody)
	}
	if !strings.Contains(msg.TextBody, "退订") {
		t.Fatalf("expected text body to contain unsubscribe link, got %s", msg.TextBody)
	}
}

func TestSendDailyEmailSkipsWhenReportHasNoContent(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 5, 31, 8, 30, 0, 0, time.UTC)
	repo := newMemoryRepository(now)
	repo.reports["report-empty"] = DailyReport{
		ID:         "report-empty",
		ReportDate: "2026-05-31",
		Title:      "",
		Summary:    "",
		BodyHTML:   "",
	}
	repo.recipients["user-1"] = Recipient{
		UserID:       "user-1",
		Email:        "reader@example.com",
		EmailEnabled: true,
	}
	fake := &fakeMailer{}
	service := NewService(repo, fake, Config{
		Host: "smtp.example.com", Port: 587, From: "daily@example.com", Configured: true,
	}, WithNow(func() time.Time { return now }))

	delivery, err := service.SendDailyEmail(ctx, SendDailyEmailInput{
		ReportID: "report-empty", RecipientUserID: "user-1", Attempt: 1,
	})
	if err != nil {
		t.Fatalf("expected no error for empty report skip, got %v", err)
	}
	if delivery.Status != DeliveryStatusSkipped {
		t.Fatalf("expected skipped status, got %v", delivery.Status)
	}
	if len(fake.messages) != 0 {
		t.Fatalf("expected no message sent for empty report, got %d", len(fake.messages))
	}
}

func TestSendDailyEmailIdempotentByReportAndUser(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 5, 31, 8, 30, 0, 0, time.UTC)
	repo := newMemoryRepository(now)
	repo.reports["report-1"] = DailyReport{
		ID: "report-1", ReportDate: "2026-05-31", Title: "日报", Summary: "摘要", BodyMarkdown: "正文",
	}
	repo.recipients["user-1"] = Recipient{
		UserID: "user-1", Email: "reader@example.com", EmailEnabled: true,
	}
	fake := &fakeMailer{}
	service := NewService(repo, fake, Config{
		Host: "smtp.example.com", Port: 587, From: "daily@example.com", Configured: true,
	}, WithNow(func() time.Time { return now }))

	// First send succeeds
	d1, err := service.SendDailyEmail(ctx, SendDailyEmailInput{
		ReportID: "report-1", RecipientUserID: "user-1", Attempt: 1,
	})
	if err != nil {
		t.Fatalf("first send failed: %v", err)
	}
	if d1.Status != DeliveryStatusSent {
		t.Fatalf("first send expected sent, got %v", d1.Status)
	}

	// Second send with same report+user should be idempotent (skipped)
	d2, err := service.SendDailyEmail(ctx, SendDailyEmailInput{
		ReportID: "report-1", RecipientUserID: "user-1", Attempt: 1,
	})
	if err != nil {
		t.Fatalf("second send failed: %v", err)
	}
	if d2.Status != DeliveryStatusSkipped {
		t.Fatalf("second send expected skipped (idempotent), got %v", d2.Status)
	}
	if len(fake.messages) != 1 {
		t.Fatalf("expected only one message sent, got %d", len(fake.messages))
	}
}

func TestSendDailyEmailSkipsWhenRecipientEmailDisabled(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryRepository(time.Now())
	repo.reports["report-1"] = DailyReport{
		ID: "report-1", ReportDate: "2026-05-31", Title: "日报", Summary: "摘要", BodyMarkdown: "正文",
	}
	repo.recipients["user-1"] = Recipient{
		UserID: "user-1", Email: "reader@example.com", EmailEnabled: false,
	}
	fake := &fakeMailer{}
	service := NewService(repo, fake, Config{
		Host: "smtp.example.com", Port: 587, From: "daily@example.com", Configured: true,
	})

	delivery, err := service.SendDailyEmail(ctx, SendDailyEmailInput{
		ReportID: "report-1", RecipientUserID: "user-1", Attempt: 1,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if delivery.Status != DeliveryStatusSkipped {
		t.Fatalf("expected skipped when email disabled, got %v", delivery.Status)
	}
	if !strings.Contains(delivery.LastError, "disabled") {
		t.Fatalf("expected error to mention disabled, got %q", delivery.LastError)
	}
	if len(fake.messages) != 0 {
		t.Fatalf("expected no message sent, got %d", len(fake.messages))
	}
}

type fakeMailer struct {
	messages []Message
	err      error
}

func (m *fakeMailer) Send(_ context.Context, message Message) error {
	m.messages = append(m.messages, message)
	return m.err
}
