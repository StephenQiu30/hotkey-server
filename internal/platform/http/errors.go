package http

import (
	"github.com/gin-gonic/gin"

	"github.com/StephenQiu30/hotkey-server/internal/model/enum"
)

// ErrorBody is the unified JSON error response body.
type ErrorBody struct {
	Error     string `json:"error"`
	Code      string `json:"code,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}

// AppError carries stable error metadata for the HTTP responder.
type AppError struct {
	Code       enum.ErrorCode
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
func NewAppError(code enum.ErrorCode, status int, message string, cause error) *AppError {
	return &AppError{
		Code:       code,
		HTTPStatus: status,
		Message:    message,
		Cause:      cause,
	}
}

// RespondErrorCode writes a registered application error by stable code.
func RespondErrorCode(c *gin.Context, code enum.ErrorCode, message string, cause error) {
	appErr := newRegisteredAppError(code, message, cause)
	c.JSON(appErr.HTTPStatus, ErrorBody{
		Error:     appErr.Message,
		Code:      string(appErr.Code),
		RequestID: requestIDFromContext(c),
	})
}

func newRegisteredAppError(code enum.ErrorCode, message string, cause error) *AppError {
	status := errorCodeToHTTPStatus(code)
	return NewAppError(code, status, message, cause)
}
