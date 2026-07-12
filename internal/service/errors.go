package service

import (
	"errors"

	"github.com/StephenQiu30/hotkey-server/internal/model/enum"
)

// CodedError carries a stable public business status while preserving its
// internal cause for structured logging.
type CodedError struct {
	code  enum.ErrorCode
	cause error
}

func (e *CodedError) Error() string             { return e.cause.Error() }
func (e *CodedError) Unwrap() error             { return e.cause }
func (e *CodedError) ErrorCode() enum.ErrorCode { return e.code }

// PublicError converts authentication domain failures into the canonical
// external ErrorCode contract. Unknown failures are intentionally generic.
func PublicError(err error) *CodedError {
	code := enum.ErrorCodeInternal
	switch {
	case errors.Is(err, AuthErrInvalidInput):
		code = enum.ErrorCodeAuthInvalidInput
	case errors.Is(err, AuthErrInvalidCredentials):
		code = enum.ErrorCodeInvalidCredentials
	case errors.Is(err, AuthErrEmailExists):
		code = enum.ErrorCodeEmailAlreadyRegistered
	case errors.Is(err, AuthErrAccountDisabled):
		code = enum.ErrorCodeAccountDisabled
	case errors.Is(err, VerificationErrNotFound), errors.Is(err, VerificationErrTicketNotFound):
		code = enum.ErrorCodeVerificationExpired
	case errors.Is(err, VerificationErrInvalidCode), errors.Is(err, VerificationErrTicketClaimed):
		code = enum.ErrorCodeVerificationInvalid
	case errors.Is(err, VerificationErrLocked), errors.Is(err, VerificationErrSendLimit), errors.Is(err, VerificationErrIPLimit):
		code = enum.ErrorCodeVerificationSendTooFrequent
	case errors.Is(err, VerificationErrRedisDown):
		code = enum.ErrorCodeServiceUnavailable
	case errors.Is(err, ErrSessionNotFound), errors.Is(err, ErrSessionExpired):
		code = enum.ErrorCodeSessionExpired
	case errors.Is(err, ErrSessionRevoked):
		code = enum.ErrorCodeSessionRevoked
	case errors.Is(err, ErrTokenReused):
		code = enum.ErrorCodeTokenReused
	}
	return &CodedError{code: code, cause: err}
}
