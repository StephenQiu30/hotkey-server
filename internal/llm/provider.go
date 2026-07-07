package llm

import "context"

// Provider defines the interface for LLM model access.
// Implementations wrap langchaingo or other backends.
type Provider interface {
	// Chat sends a chat completion request and returns the response text.
	Chat(ctx context.Context, prompt string, opts ...Option) (string, error)
}

// Option configures a Chat request.
type Option func(*Options)

// Options holds optional parameters for Chat requests.
type Options struct {
	MaxTokens   int
	Temperature float64
	Model       string // per-request model override
}

// WithMaxTokens sets the maximum tokens for the response.
func WithMaxTokens(n int) Option {
	return func(o *Options) { o.MaxTokens = n }
}

// WithTemperature sets the response temperature.
func WithTemperature(t float64) Option {
	return func(o *Options) { o.Temperature = t }
}

// WithModel overrides the default model for this request.
func WithModel(m string) Option {
	return func(o *Options) { o.Model = m }
}
