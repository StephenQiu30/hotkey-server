package domain

import (
	"errors"
	"testing"
)

func TestDomainErrorsExposeOnlyStableAICodes(t *testing.T) {
	err := NewError(CodeAIProviderTimeout)
	if code, ok := CodeOf(err); !ok || code != CodeAIProviderTimeout {
		t.Fatalf("CodeOf() = %d/%t, want %d", code, ok, CodeAIProviderTimeout)
	}
	if !errors.Is(err, NewError(CodeAIProviderTimeout)) {
		t.Fatal("domain errors with the same stable code must match")
	}
	if !Retryable(CodeAIProviderTimeout) || Retryable(CodeAIOutputInvalid) {
		t.Fatal("Retryable() does not preserve the shared AI error contract")
	}
}
