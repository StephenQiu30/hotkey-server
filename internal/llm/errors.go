package llm

import "errors"

var (
	// ErrProviderError is returned when the underlying LLM API call fails.
	ErrProviderError = errors.New("llm provider error")
	// ErrContentTooLong is returned when input exceeds the model context window.
	ErrContentTooLong = errors.New("content exceeds model context length")
	// ErrEmptyResponse is returned when the LLM returns empty content.
	ErrEmptyResponse = errors.New("llm returned empty response")
)
