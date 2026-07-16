// Package domain contains provider-neutral AI facts and stable outcomes.
package domain

import (
	stdErrors "errors"

	sharederrors "github.com/StephenQiu30/hotkey-server/internal/shared/errors"
)

const (
	CodeAIModelProfileInvalid = sharederrors.CodeAIModelProfileInvalid
	CodeAIModelUnavailable    = sharederrors.CodeAIModelUnavailable
	CodeAIBudgetExhausted     = sharederrors.CodeAIBudgetExhausted
	CodeAIProviderRateLimited = sharederrors.CodeAIProviderRateLimited
	CodeAIProviderTransient   = sharederrors.CodeAIProviderTransient
	CodeAIProviderTimeout     = sharederrors.CodeAIProviderTimeout
	CodeAIOutputInvalid       = sharederrors.CodeAIOutputInvalid
	CodeAIRunInProgress       = sharederrors.CodeAIRunInProgress
	CodeAIEmbeddingInvalid    = sharederrors.CodeAIEmbeddingInvalid
	CodeAIRunLeaseExpired     = sharederrors.CodeAIRunLeaseExpired
)

// Error retains only a registered numeric AI code. Provider/SDK error text is
// deliberately excluded so it cannot leak across the infrastructure boundary.
type Error struct{ Code int }

func (err *Error) Error() string {
	if err == nil {
		return ""
	}
	if definition, ok := sharederrors.Lookup(err.Code); ok {
		return definition.Message
	}
	return "AI operation failed"
}

func (err *Error) Is(target error) bool {
	other, ok := target.(*Error)
	return ok && err != nil && other != nil && err.Code == other.Code
}

func NewError(code int) *Error { return &Error{Code: code} }

func CodeOf(err error) (int, bool) {
	var domainErr *Error
	if !stdErrors.As(err, &domainErr) || domainErr == nil {
		return 0, false
	}
	return domainErr.Code, true
}

func Retryable(code int) bool {
	definition, ok := sharederrors.Lookup(code)
	return ok && definition.Retryable
}
