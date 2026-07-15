package errors

import "fmt"

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

func New(code int, status int, message string) *AppError {
	return &AppError{Code: code, HTTPStatus: status, Message: message}
}

func Wrap(code int, status int, message string, cause error) *AppError {
	return &AppError{Code: code, HTTPStatus: status, Message: message, Cause: cause}
}
