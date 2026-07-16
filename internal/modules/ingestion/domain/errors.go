// Package domain defines ingestion-owned Content facts and contracts.
package domain

import "errors"

// ErrorCode is a stable, transport-independent ingestion failure category.
// Application services may persist these values on a captured item without
// exposing parser details or upstream payload content.
type ErrorCode string

const (
	ErrorCodeInvalidCapturedItem      ErrorCode = "invalid_captured_item"
	ErrorCodeInvalidCanonicalURL      ErrorCode = "invalid_canonical_url"
	ErrorCodeInvalidContentType       ErrorCode = "invalid_content_type"
	ErrorCodeEmptyContent             ErrorCode = "empty_content"
	ErrorCodeInvalidNormalizedContent ErrorCode = "invalid_normalized_content"
	ErrorCodeInvalidContentCandidate  ErrorCode = "invalid_content_candidate"
	ErrorCodeInvalidDedupeDecision    ErrorCode = "invalid_dedupe_decision"
)

// Error contains only the stable code. Concrete parsing and persistence
// details stay at their respective boundaries and never become Content facts.
type Error struct {
	Code ErrorCode
}

func (err *Error) Error() string {
	if err == nil {
		return ""
	}
	return string(err.Code)
}

func (err *Error) Is(target error) bool {
	other, ok := target.(*Error)
	return ok && err != nil && err.Code == other.Code
}

func NewError(code ErrorCode) *Error {
	return &Error{Code: code}
}

// ErrorCodeOf returns a stable ingestion code when an error originated at the
// ingestion domain boundary.
func ErrorCodeOf(err error) (ErrorCode, bool) {
	var domainErr *Error
	if !errors.As(err, &domainErr) || domainErr == nil {
		return "", false
	}
	return domainErr.Code, true
}
