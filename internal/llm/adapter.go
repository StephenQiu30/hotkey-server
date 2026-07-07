package llm

import (
	"context"
	"log"

	"github.com/tmc/langchaingo/llms"
)

// langchainAdapter wraps a langchaingo llms.Model as a Provider.
type langchainAdapter struct {
	model llms.Model
	opts  Options // default options
}

func newLangchainAdapter(model llms.Model, opts Options) *langchainAdapter {
	return &langchainAdapter{model: model, opts: opts}
}

func (a *langchainAdapter) Chat(ctx context.Context, prompt string, opts ...Option) (string, error) {
	o := a.opts
	for _, fn := range opts {
		fn(&o)
	}

	llmOpts := make([]llms.CallOption, 0)
	if o.MaxTokens > 0 {
		llmOpts = append(llmOpts, llms.WithMaxTokens(o.MaxTokens))
	}
	if o.Temperature > 0 {
		llmOpts = append(llmOpts, llms.WithTemperature(o.Temperature))
	}

	resp, err := llms.GenerateFromSinglePrompt(ctx, a.model, prompt, llmOpts...)
	if err != nil {
		log.Printf("llm provider error: %v", err)
		return "", ErrProviderError
	}
	if resp == "" {
		return "", ErrEmptyResponse
	}
	return resp, nil
}
