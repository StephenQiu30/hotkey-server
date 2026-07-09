package enum

// ErrorCode is a stable application error code for API responses.
// Values use UPPER_SNAKE to follow algorithm-cloud convention and be
// distinct from plain-text error messages.
type ErrorCode string

const (
	ErrorCodeBadRequest    ErrorCode = "BAD_REQUEST"
	ErrorCodeUnauthorized  ErrorCode = "UNAUTHORIZED"
	ErrorCodeForbidden     ErrorCode = "FORBIDDEN"
	ErrorCodeNotFound      ErrorCode = "NOT_FOUND"
	ErrorCodeConflict      ErrorCode = "CONFLICT"
	ErrorCodeInternal      ErrorCode = "INTERNAL_ERROR"
)