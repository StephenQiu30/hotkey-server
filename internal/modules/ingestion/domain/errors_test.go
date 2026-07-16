package domain

import (
	"errors"
	"testing"
)

func TestErrorCodeOfPreservesStableIngestionCode(t *testing.T) {
	t.Parallel()

	err := NewError(ErrorCodeInvalidCanonicalURL)
	code, ok := ErrorCodeOf(err)
	if !ok || code != ErrorCodeInvalidCanonicalURL {
		t.Fatalf("ErrorCodeOf(%v) = %q, %t", err, code, ok)
	}
	if !errors.Is(err, NewError(ErrorCodeInvalidCanonicalURL)) {
		t.Fatalf("errors.Is(%v, same code) = false", err)
	}
}
