package worker

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/queue"
	servicereport "github.com/StephenQiu30/hotkey-server/internal/service/report"
)

// 测试：GenerateReportHandler 生成日报成功后，自动入队 send_daily_email 给所有 recipients
func TestGenerateReportHandlerEnqueuesEmailsForRecipients(t *testing.T) {
	body, _ := json.Marshal(queue.GenerateDailyReportPayload{Date: "2026-05-31"})
	service := &mockReportService{}
	producer := &mockProducer{}
	recipients := []string{"user-1", "user-2"}

	handler := NewGenerateReportHandlerWithMail(service, producer, recipients)
	if err := handler.Handle(context.Background(), queue.Job{Payload: body}); err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}

	// 验证日报生成
	if service.input.Date != "2026-05-31" || service.input.ChannelID != "default" {
		t.Fatalf("unexpected input: %+v", service.input)
	}

	// 验证入队了 2 个 send_daily_email 任务
	if len(producer.requests) != 2 {
		t.Fatalf("expected 2 enqueue requests, got %d", len(producer.requests))
	}

	for i, req := range producer.requests {
		if req.Type != queue.JobTypeSendDailyEmail {
			t.Fatalf("request %d: expected send_daily_email job, got %s", i, req.Type)
		}
		var payload queue.SendDailyEmailPayload
		if err := json.Unmarshal(req.Payload, &payload); err != nil {
			t.Fatalf("request %d: payload was not send_daily_email payload: %v", i, err)
		}
		if payload.ReportID != "rpt-1" {
			t.Fatalf("request %d: expected report ID rpt-1, got %s", i, payload.ReportID)
		}
	}

	// 验证两个不同用户
	var payload0, payload1 queue.SendDailyEmailPayload
	_ = json.Unmarshal(producer.requests[0].Payload, &payload0)
	_ = json.Unmarshal(producer.requests[1].Payload, &payload1)
	if payload0.RecipientUserID != "user-1" || payload1.RecipientUserID != "user-2" {
		t.Fatalf("unexpected recipients: %s, %s", payload0.RecipientUserID, payload1.RecipientUserID)
	}
}

// 测试：日报生成失败（failed_config）时不应入队邮件任务
func TestGenerateReportHandlerSkipsEmailsOnFailedConfig(t *testing.T) {
	body, _ := json.Marshal(queue.GenerateDailyReportPayload{Date: "2026-05-31"})
	service := &mockReportService{err: servicereport.ErrFailedConfig}
	producer := &mockProducer{}
	recipients := []string{"user-1"}

	handler := NewGenerateReportHandlerWithMail(service, producer, recipients)
	if err := handler.Handle(context.Background(), queue.Job{Payload: body}); err != nil {
		t.Fatalf("expected failed_config to be non-fatal, got %v", err)
	}

	// 验证没有入队邮件任务
	if len(producer.requests) != 0 {
		t.Fatalf("expected 0 enqueue requests on failed_config, got %d", len(producer.requests))
	}
}

// 测试：无 recipients 时不入队邮件任务
func TestGenerateReportHandlerSkipsEmailsWhenNoRecipients(t *testing.T) {
	body, _ := json.Marshal(queue.GenerateDailyReportPayload{Date: "2026-05-31"})
	service := &mockReportService{}
	producer := &mockProducer{}

	handler := NewGenerateReportHandlerWithMail(service, producer, nil)
	if err := handler.Handle(context.Background(), queue.Job{Payload: body}); err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}

	if len(producer.requests) != 0 {
		t.Fatalf("expected 0 enqueue requests with no recipients, got %d", len(producer.requests))
	}
}

// 测试：日报生成返回错误（非 failed_config）时应传播错误且不入队邮件
func TestGenerateReportHandlerPropagatesErrorAndSkipsEmails(t *testing.T) {
	body, _ := json.Marshal(queue.GenerateDailyReportPayload{Date: "2026-05-31"})
	service := &mockReportService{err: context.DeadlineExceeded}
	producer := &mockProducer{}
	recipients := []string{"user-1"}

	handler := NewGenerateReportHandlerWithMail(service, producer, recipients)
	err := handler.Handle(context.Background(), queue.Job{Payload: body})
	if err == nil {
		t.Fatal("expected error to be propagated")
	}

	if len(producer.requests) != 0 {
		t.Fatalf("expected 0 enqueue requests on error, got %d", len(producer.requests))
	}
}

// 测试：idempotency key 格式正确
func TestGenerateReportHandlerEmailIdempotencyKeyFormat(t *testing.T) {
	body, _ := json.Marshal(queue.GenerateDailyReportPayload{Date: "2026-06-08"})
	service := &mockReportService{}
	producer := &mockProducer{}
	recipients := []string{"user-42"}

	handler := NewGenerateReportHandlerWithMail(service, producer, recipients)
	if err := handler.Handle(context.Background(), queue.Job{Payload: body}); err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}

	if len(producer.requests) != 1 {
		t.Fatalf("expected 1 enqueue request, got %d", len(producer.requests))
	}
	req := producer.requests[0]
	expectedKey := "send_daily_email:rpt-1:user-42:2026-06-08"
	if req.IdempotencyKey != expectedKey {
		t.Fatalf("expected idempotency key %q, got %q", expectedKey, req.IdempotencyKey)
	}
}

// mockProducer 记录入队请求
type mockProducer struct {
	requests []queue.EnqueueRequest
}

func (p *mockProducer) Enqueue(_ context.Context, req queue.EnqueueRequest) (queue.Job, error) {
	p.requests = append(p.requests, req)
	return queue.Job{ID: "job-mock", Type: req.Type}, nil
}
