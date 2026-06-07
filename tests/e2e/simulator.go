package e2e_test

import (
	"context"
	"time"
)

// ProviderBehavior enumerates the five simulator behaviors required by V1 E2E.
type ProviderBehavior string

const (
	BehaviorNormal       ProviderBehavior = "normal"
	BehaviorRateLimit    ProviderBehavior = "rate_limit"
	BehaviorAuthInvalid  ProviderBehavior = "auth_invalid"
	BehaviorSchemaChange ProviderBehavior = "schema_change"
	BehaviorEmptyResult  ProviderBehavior = "empty_result"
)

// AllBehaviors returns all five required simulator behaviors.
func AllBehaviors() []ProviderBehavior {
	return []ProviderBehavior{
		BehaviorNormal,
		BehaviorRateLimit,
		BehaviorAuthInvalid,
		BehaviorSchemaChange,
		BehaviorEmptyResult,
	}
}

// --- AI Provider Simulator Contract ---

// AIResponse represents a simulated AI provider response.
type AIResponse struct {
	Text   string
	Vector []float64
	Model  string
}

// AIProviderSimulator simulates DashScope embedding + chat behaviors.
type AIProviderSimulator interface {
	SetBehavior(b ProviderBehavior)
	Embed(ctx context.Context, text string) ([]float64, error)
	Chat(ctx context.Context, prompt string) (string, error)
	Reset()
}

// --- SMTP Sink Contract ---

// EmailRecord captures a simulated email send.
type EmailRecord struct {
	From    string
	To      string
	Subject string
	Body    string
	SentAt  time.Time
}

// SMTPSink simulates an SMTP server that captures emails.
type SMTPSink interface {
	Addr() string
	Records() []EmailRecord
	Reset()
}

// --- Fetcher Simulator Contract ---

// FetcherSimulator simulates RSS/public page fetcher behaviors.
type FetcherSimulator interface {
	SetBehavior(b ProviderBehavior)
	Fetch(ctx context.Context, sourceURL string) ([]map[string]string, error)
	Reset()
}
