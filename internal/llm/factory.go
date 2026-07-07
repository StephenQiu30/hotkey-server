package llm

import (
	"fmt"

	"github.com/StephenQiu30/hotkey-server/internal/config"
	"github.com/tmc/langchaingo/llms/openai"
)

// NewProvider creates a Provider from the given config.
func NewProvider(cfg config.Config) (Provider, error) {
	opts := Options{
		MaxTokens:   cfg.LLMMaxTokens,
		Temperature: cfg.LLMTemperature,
		Model:       cfg.LLMModel,
	}

	switch cfg.LLMProvider {
	case "openai":
		llm, err := openai.New(
			openai.WithModel(cfg.LLMModel),
			openai.WithBaseURL(cfg.LLMBaseURL),
			openai.WithToken(cfg.LLMAPIKey),
		)
		if err != nil {
			return nil, fmt.Errorf("create openai provider: %w", err)
		}
		return newLangchainAdapter(llm, opts), nil

	case "anthropic":
		return nil, fmt.Errorf("anthropic provider not yet implemented")
	case "ollama":
		return nil, fmt.Errorf("ollama provider not yet implemented")
	default:
		return nil, fmt.Errorf("unknown LLM provider: %q", cfg.LLMProvider)
	}
}
