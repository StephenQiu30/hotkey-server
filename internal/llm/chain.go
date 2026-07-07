package llm

import "context"

// Chain orchestrates multi-step LLM pipelines for content aggregation.
type Chain struct {
	svc Service
}

// NewChain creates a Chain backed by the given Service.
func NewChain(svc Service) *Chain {
	return &Chain{svc: svc}
}

// BuildDailyDigest runs the full digest pipeline: summarize each post,
// label each post, then compile the final digest.
func (c *Chain) BuildDailyDigest(ctx context.Context, input DigestInput, opts ...ChainOption) (DigestOutput, error) {
	cfg := defaultChainConfig()
	for _, fn := range opts {
		fn(&cfg)
	}

	posts := make([]PostItem, len(input.Posts))
	for i, p := range input.Posts {
		posts[i] = p

		// Summarize if not already provided
		if posts[i].Summary == "" && cfg.summarize {
			summary, err := c.svc.Summarize(ctx, truncateContent(p.Content, cfg.maxContentLen))
			if err != nil {
				// Non-blocking: skip summary on error, continue with empty
				posts[i].Summary = ""
			} else {
				posts[i].Summary = summary
			}
		}

		// Label if not already provided
		if len(posts[i].Labels) == 0 && cfg.label {
			labels, err := c.svc.LabelTopics(ctx, truncateContent(p.Content, cfg.maxContentLen))
			if err != nil {
				// Non-blocking: skip labels on error
				posts[i].Labels = nil
			} else {
				posts[i].Labels = labels
			}
		}
	}

	// Compile the final digest
	digestInput := DigestInput{
		Title: input.Title,
		Posts: posts,
	}

	return c.svc.GenerateDigest(ctx, digestInput)
}

// ChainOption configures the Chain pipeline.
type ChainOption func(*chainConfig)

type chainConfig struct {
	summarize     bool
	label         bool
	maxContentLen int
}

func defaultChainConfig() chainConfig {
	return chainConfig{
		summarize:     true,
		label:         true,
		maxContentLen: 4000,
	}
}

// WithSummarize enables or disables per-post summarization.
func WithSummarize(enabled bool) ChainOption {
	return func(c *chainConfig) { c.summarize = enabled }
}

// WithLabel enables or disables per-post topic labeling.
func WithLabel(enabled bool) ChainOption {
	return func(c *chainConfig) { c.label = enabled }
}

// WithMaxContentLen sets the maximum content length per post (in characters).
func WithMaxContentLen(n int) ChainOption {
	return func(c *chainConfig) { c.maxContentLen = n }
}

func truncateContent(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}
