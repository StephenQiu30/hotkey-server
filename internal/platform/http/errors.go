package http

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

type ErrorCode string

type ErrorBody struct {
	Error     string `json:"error"`
	Code      string `json:"code,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}

const (
	// ErrorCodeInternal is the common internal server error code.
	ErrorCodeInternal ErrorCode = "internal_error"
	// ErrorCodeBadRequest is for malformed or invalid requests.
	ErrorCodeBadRequest ErrorCode = "bad_request"
	// ErrorCodeUnauthorized is returned when authentication is missing or invalid.
	ErrorCodeUnauthorized ErrorCode = "unauthorized"
	// ErrorCodeForbidden is returned when the caller is not allowed to access a resource.
	ErrorCodeForbidden ErrorCode = "forbidden"
	// ErrorCodeNotFound is returned when a resource is missing.
	ErrorCodeNotFound ErrorCode = "not_found"
	// ErrorCodeConflict is returned when the request conflicts with existing state.
	ErrorCodeConflict ErrorCode = "conflict"
	// ErrorCodeMonitorNotFound is for monitor resource not found.
	ErrorCodeMonitorNotFound ErrorCode = "MONITOR_NOT_FOUND"
)

var errorStatusRegistry = map[ErrorCode]int{
	ErrorCodeBadRequest:      http.StatusBadRequest,
	ErrorCodeUnauthorized:    http.StatusUnauthorized,
	ErrorCodeForbidden:       http.StatusForbidden,
	ErrorCodeNotFound:        http.StatusNotFound,
	ErrorCodeConflict:        http.StatusConflict,
	ErrorCodeInternal:        http.StatusInternalServerError,
	ErrorCodeMonitorNotFound: http.StatusNotFound,
}

// AppError carries stable error metadata for the HTTP responder.
type AppError struct {
	Code       ErrorCode
	Message    string
	HTTPStatus int
	Cause      error
}

func (e *AppError) Error() string {
	if e == nil {
		return ""
	}
	if e.Cause != nil {
		return e.Message + ": " + e.Cause.Error()
	}
	return e.Message
}

func (e *AppError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// NewAppError creates an application error with a stable external contract.
func NewAppError(code ErrorCode, status int, message string, cause error) *AppError {
	return &AppError{
		Code:       code,
		HTTPStatus: status,
		Message:    message,
		Cause:      cause,
	}
}

func newRegisteredAppError(code ErrorCode, message string, cause error) *AppError {
	status, ok := errorStatusRegistry[code]
	if !ok {
		status = http.StatusInternalServerError
		code = ErrorCodeInternal
	}
	return NewAppError(code, status, message, cause)
}

func newInternalErrorBody(requestID string) ErrorBody {
	return ErrorBody{
		Error:     "internal server error",
		Code:      string(ErrorCodeInternal),
		RequestID: requestID,
	}
}

func respondError(c *gin.Context, status int, message string) {
	body := ErrorBody{
		Error:     message,
		Code:      string(errorCodeForHTTPStatus(status)),
		RequestID: requestIDFromContext(c),
	}
	c.JSON(status, body)
}

func respondInternalError(c *gin.Context) {
	c.JSON(http.StatusInternalServerError, newInternalErrorBody(requestIDFromContext(c)))
}

// RespondAppError writes an AppError as a unified error response.
func RespondAppError(c *gin.Context, err error) {
	var appErr *AppError
	if errors.As(err, &appErr) {
		c.JSON(appErr.HTTPStatus, ErrorBody{
			Error:     appErr.Message,
			Code:      string(appErr.Code),
			RequestID: requestIDFromContext(c),
		})
		return
	}
	respondInternalError(c)
}

// RespondErrorCode writes a registered application error by stable code.
func RespondErrorCode(c *gin.Context, code ErrorCode, message string, cause error) {
	RespondAppError(c, newRegisteredAppError(code, message, cause))
}

func errorCodeForHTTPStatus(status int) ErrorCode {
	switch status {
	case http.StatusBadRequest:
		return ErrorCodeBadRequest
	case http.StatusUnauthorized:
		return ErrorCodeUnauthorized
	case http.StatusForbidden:
		return ErrorCodeForbidden
	case http.StatusNotFound:
		return ErrorCodeNotFound
	case http.StatusConflict:
		return ErrorCodeConflict
	default:
		if status >= http.StatusInternalServerError {
			return ErrorCodeInternal
		}
		return ErrorCodeInternal
	}
}
