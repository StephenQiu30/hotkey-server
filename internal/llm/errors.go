package llm

import "errors"

var (
	// ErrProviderError is returned when the underlying LLM API call fails.
	ErrProviderError = errors.New("llm provider error")
	// ErrEmptyResponse is returned when the LLM returns empty content.
	ErrEmptyResponse = errors.New("llm returned empty response")
)
