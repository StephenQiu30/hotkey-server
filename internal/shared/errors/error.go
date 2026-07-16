package errors

import (
	"fmt"
	stdhttp "net/http"
	"sync"
)

const (
	CodeInvalidRequest      = 10000
	CodeValidation          = 10001
	CodeConflict            = 10002
	CodeNotFound            = 10003
	CodeRateLimited         = 10004
	CodeUnauthenticated     = 20000
	CodeForbidden           = 20001
	CodeInvalidCredentials  = 20002
	CodeSessionInvalid      = 20003
	CodeVerificationInvalid = 20004
	CodeLastActiveAdmin     = 20005
	// Monitor/source configuration errors are stable control-plane outcomes;
	// callers must key behavior on these values rather than error messages.
	CodeInvalidMonitorState         = 30000
	CodeMonitorVersionConflict      = 30001
	CodeInvalidMonitorConfiguration = 30002
	CodeMonitorDraftUnavailable     = 30003
	CodeMonitorNameConflict         = 30004
	CodeInvalidSourceConfiguration  = 40000
	CodeSourceConnectionRequired    = 40001
	CodeUnsupportedSourceType       = 40002
	CodeSourceConnectionUnavailable = 40003
	CodeCollectionRunNotFound       = 40004
	CodeCollectionRunConflict       = 40005
	CodeInvalidCollectionRequest    = 40006
	CodeAIModelProfileInvalid       = 70000
	CodeAIModelUnavailable          = 70001
	CodeAIBudgetExhausted           = 70002
	CodeAIProviderRateLimited       = 70003
	CodeAIProviderTransient         = 70004
	CodeAIProviderTimeout           = 70005
	CodeAIOutputInvalid             = 70006
	CodeAIRunInProgress             = 70007
	CodeAIEmbeddingInvalid          = 70008
	CodeAIRunLeaseExpired           = 70009
	CodeInternal                    = 90000
	CodeUnavailable                 = 90001
	CodeBadGateway                  = 90002
	CodeDeadlineExceeded            = 90003
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
		{Code: CodeInvalidCredentials, HTTPStatus: stdhttp.StatusUnauthorized, Message: "invalid credentials"},
		{Code: CodeSessionInvalid, HTTPStatus: stdhttp.StatusUnauthorized, Message: "session invalid"},
		{Code: CodeVerificationInvalid, HTTPStatus: stdhttp.StatusBadRequest, Message: "verification invalid"},
		{Code: CodeLastActiveAdmin, HTTPStatus: stdhttp.StatusConflict, Message: "last active admin"},
		{Code: CodeInvalidMonitorState, HTTPStatus: stdhttp.StatusConflict, Message: "invalid monitor state"},
		{Code: CodeMonitorVersionConflict, HTTPStatus: stdhttp.StatusConflict, Message: "monitor version conflict"},
		{Code: CodeInvalidMonitorConfiguration, HTTPStatus: stdhttp.StatusBadRequest, Message: "invalid monitor configuration"},
		{Code: CodeMonitorDraftUnavailable, HTTPStatus: stdhttp.StatusConflict, Message: "monitor draft unavailable"},
		{Code: CodeMonitorNameConflict, HTTPStatus: stdhttp.StatusConflict, Message: "monitor name conflict"},
		{Code: CodeInvalidSourceConfiguration, HTTPStatus: stdhttp.StatusBadRequest, Message: "invalid source configuration"},
		{Code: CodeSourceConnectionRequired, HTTPStatus: stdhttp.StatusConflict, Message: "source connection required"},
		{Code: CodeUnsupportedSourceType, HTTPStatus: stdhttp.StatusBadRequest, Message: "unsupported source type"},
		{Code: CodeSourceConnectionUnavailable, HTTPStatus: stdhttp.StatusConflict, Message: "source connection unavailable"},
		{Code: CodeCollectionRunNotFound, HTTPStatus: stdhttp.StatusNotFound, Message: "collection run not found"},
		{Code: CodeCollectionRunConflict, HTTPStatus: stdhttp.StatusConflict, Message: "collection run conflict"},
		{Code: CodeInvalidCollectionRequest, HTTPStatus: stdhttp.StatusBadRequest, Message: "invalid collection request"},
		{Code: CodeAIModelProfileInvalid, HTTPStatus: stdhttp.StatusBadRequest, Message: "AI model profile invalid"},
		{Code: CodeAIModelUnavailable, HTTPStatus: stdhttp.StatusServiceUnavailable, Message: "AI model unavailable", Retryable: true},
		{Code: CodeAIBudgetExhausted, HTTPStatus: stdhttp.StatusTooManyRequests, Message: "AI budget exhausted", Retryable: true},
		{Code: CodeAIProviderRateLimited, HTTPStatus: stdhttp.StatusTooManyRequests, Message: "AI provider rate limited", Retryable: true},
		{Code: CodeAIProviderTransient, HTTPStatus: stdhttp.StatusBadGateway, Message: "AI provider transient failure", Retryable: true},
		{Code: CodeAIProviderTimeout, HTTPStatus: stdhttp.StatusGatewayTimeout, Message: "AI provider timeout", Retryable: true},
		{Code: CodeAIOutputInvalid, HTTPStatus: stdhttp.StatusBadGateway, Message: "AI output invalid"},
		{Code: CodeAIRunInProgress, HTTPStatus: stdhttp.StatusConflict, Message: "AI run in progress", Retryable: true},
		{Code: CodeAIEmbeddingInvalid, HTTPStatus: stdhttp.StatusBadRequest, Message: "AI embedding invalid"},
		{Code: CodeAIRunLeaseExpired, HTTPStatus: stdhttp.StatusServiceUnavailable, Message: "AI run lease expired", Retryable: true},
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
