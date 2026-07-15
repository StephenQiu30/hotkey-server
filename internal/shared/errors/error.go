package errors

import (
	"fmt"
	stdhttp "net/http"
	"sync"
)

const (
	CodeInvalidRequest   = 10000
	CodeValidation       = 10001
	CodeConflict         = 10002
	CodeNotFound         = 10003
	CodeRateLimited      = 10004
	CodeUnauthenticated  = 20000
	CodeForbidden        = 20001
	CodeInternal         = 90000
	CodeUnavailable      = 90001
	CodeBadGateway       = 90002
	CodeDeadlineExceeded = 90003
)

type CodeDefinition struct {
	Code       int
	HTTPStatus int
	Message    string
	Retryable  bool
}

var (
	catalogMu sync.RWMutex
	catalog   = make(map[int]CodeDefinition)
)

func init() {
	for _, definition := range []CodeDefinition{
		{Code: CodeInvalidRequest, HTTPStatus: stdhttp.StatusBadRequest, Message: "invalid request"},
		{Code: CodeValidation, HTTPStatus: stdhttp.StatusBadRequest, Message: "validation failed"},
		{Code: CodeConflict, HTTPStatus: stdhttp.StatusConflict, Message: "conflict"},
		{Code: CodeNotFound, HTTPStatus: stdhttp.StatusNotFound, Message: "not found"},
		{Code: CodeRateLimited, HTTPStatus: stdhttp.StatusTooManyRequests, Message: "rate limited", Retryable: true},
		{Code: CodeUnauthenticated, HTTPStatus: stdhttp.StatusUnauthorized, Message: "unauthenticated"},
		{Code: CodeForbidden, HTTPStatus: stdhttp.StatusForbidden, Message: "forbidden"},
		{Code: CodeInternal, HTTPStatus: stdhttp.StatusInternalServerError, Message: "internal server error"},
		{Code: CodeUnavailable, HTTPStatus: stdhttp.StatusServiceUnavailable, Message: "service unavailable", Retryable: true},
		{Code: CodeBadGateway, HTTPStatus: stdhttp.StatusBadGateway, Message: "bad gateway", Retryable: true},
		{Code: CodeDeadlineExceeded, HTTPStatus: stdhttp.StatusGatewayTimeout, Message: "deadline exceeded", Retryable: true},
	} {
		if err := RegisterCode(definition); err != nil {
			panic(err)
		}
	}
}

type AppError struct {
	Code       int
	HTTPStatus int
	Message    string
	Retryable  bool
	Cause      error
}

func (e *AppError) Error() string {
	if e.Cause == nil {
		return e.Message
	}
	return fmt.Sprintf("%s: %v", e.Message, e.Cause)
}

func (e *AppError) Unwrap() error {
	return e.Cause
}

func RegisterCode(definition CodeDefinition) error {
	if definition.Code <= 0 {
		return fmt.Errorf("error code must be positive")
	}
	if definition.HTTPStatus < 400 || definition.HTTPStatus > 599 {
		return fmt.Errorf("error code %d must use an error HTTP status", definition.Code)
	}
	if definition.Message == "" {
		return fmt.Errorf("error code %d must have a message", definition.Code)
	}

	catalogMu.Lock()
	defer catalogMu.Unlock()
	if _, exists := catalog[definition.Code]; exists {
		return fmt.Errorf("error code %d is already registered", definition.Code)
	}
	catalog[definition.Code] = definition
	return nil
}

func Lookup(code int) (CodeDefinition, bool) {
	catalogMu.RLock()
	defer catalogMu.RUnlock()
	definition, ok := catalog[code]
	return definition, ok
}

// New constructs an AppError for a registered code. The compatibility status
// parameter must match the code's registered HTTP status; callers may provide
// a safe, context-specific message, but clients must key behavior on Code.
func New(code int, status int, message string) *AppError {
	return newAppError(code, status, message, nil)
}

// Wrap is New with an internal cause. Cause must only be used for logs and
// tracing; HTTP writers never expose it.
func Wrap(code int, status int, message string, cause error) *AppError {
	return newAppError(code, status, message, cause)
}

func newAppError(code int, status int, message string, cause error) *AppError {
	definition, ok := Lookup(code)
	if !ok {
		panic(fmt.Sprintf("error code %d is not registered", code))
	}
	if status != definition.HTTPStatus {
		panic(fmt.Sprintf("error code %d must use HTTP status %d, got %d", code, definition.HTTPStatus, status))
	}
	if message == "" {
		message = definition.Message
	}
	return &AppError{
		Code:       code,
		HTTPStatus: definition.HTTPStatus,
		Message:    message,
		Retryable:  definition.Retryable,
		Cause:      cause,
	}
}
