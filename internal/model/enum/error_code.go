package enum

// ErrorCode is a stable application error code for API responses.
// Values use UPPER_SNAKE to follow algorithm-cloud convention and be
// distinct from plain-text error messages.
type ErrorCode string

const (
	// Success / generic
	ErrorCodeSuccess               ErrorCode = "SUCCESS"
	ErrorCodeBadRequest            ErrorCode = "BAD_REQUEST"
	ErrorCodeUnauthorized          ErrorCode = "UNAUTHORIZED"
	ErrorCodeForbidden             ErrorCode = "FORBIDDEN"
	ErrorCodeNotFound              ErrorCode = "NOT_FOUND"
	ErrorCodeConflict              ErrorCode = "CONFLICT"
	ErrorCodeInternal              ErrorCode = "INTERNAL_ERROR"
	ErrorCodeRateLimited           ErrorCode = "RATE_LIMITED"
	ErrorCodeServiceUnavailable    ErrorCode = "SERVICE_UNAVAILABLE"
	ErrorCodeMethodNotAllowed     ErrorCode = "METHOD_NOT_ALLOWED"

	// Auth
	ErrorCodeInvalidCredentials        ErrorCode = "INVALID_CREDENTIALS"
	ErrorCodeEmailExists               ErrorCode = "EMAIL_EXISTS"
	ErrorCodeInvalidVerificationCode   ErrorCode = "INVALID_VERIFICATION_CODE"
	ErrorCodeTokenExpired              ErrorCode = "TOKEN_EXPIRED"
	ErrorCodeTokenRevoked              ErrorCode = "TOKEN_REVOKED"
	ErrorCodeSessionExpired            ErrorCode = "SESSION_EXPIRED"
	ErrorCodePasswordMismatch          ErrorCode = "PASSWORD_MISMATCH"
	ErrorCodeEmailNotVerified          ErrorCode = "EMAIL_NOT_VERIFIED"
	ErrorCodeAccountDisabled           ErrorCode = "ACCOUNT_DISABLED"
	ErrorCodeInvalidResetToken         ErrorCode = "INVALID_RESET_TOKEN"
	ErrorCodeTokenReused               ErrorCode = "AUTH_TOKEN_REUSED"
)
