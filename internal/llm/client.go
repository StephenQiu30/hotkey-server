// Package llm provides LLM-based summarization for topic digests.
// This is a stub implementation; full logic will be added in STE-305.
package llm

import (
	"context"
	"fmt"
)

// SummarizeInput holds the data needed to generate a topic summary.
type SummarizeInput struct {
	MonitorName string
	TopicTitle  string
	TopicKey    string
	HeatScore   float64
	Trend       string
	PostCount   int
	Posts       []Post
}

// Post represents a representative post for summarization.
type Post struct {
	AuthorName string
	Text       string
	URL        string
}

// Client provides LLM summarization.
type Client struct {
	apiKey  string
	baseURL string
	model   string
}

// NewClient creates an LLM client.
func NewClient(apiKey, baseURL, model string) *Client {
	return &Client{
		apiKey:  apiKey,
		baseURL: baseURL,
		model:   model,
	}
}

// SummarizeTopic generates a summary for the given topic input.
// TODO(STE-305): implement real LLM call.
func (c *Client) SummarizeTopic(_ context.Context, _ SummarizeInput) (string, error) {
	return "", fmt.Errorf("llm: not implemented (stub from STE-305)")
}
