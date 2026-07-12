package enum

// ErrorCode is a stable application error code for API responses.
// Values use UPPER_SNAKE to follow algorithm-cloud convention and be
// distinct from plain-text error messages.
type ErrorCode string

const (
	// Success / generic
	ErrorCodeSuccess            ErrorCode = "SUCCESS"
	ErrorCodeBadRequest         ErrorCode = "BAD_REQUEST"
	ErrorCodeUnauthorized       ErrorCode = "UNAUTHORIZED"
	ErrorCodeForbidden          ErrorCode = "FORBIDDEN"
	ErrorCodeNotFound           ErrorCode = "NOT_FOUND"
	ErrorCodeConflict           ErrorCode = "CONFLICT"
	ErrorCodeInternal           ErrorCode = "INTERNAL_ERROR"
	ErrorCodeRateLimited        ErrorCode = "RATE_LIMITED"
	ErrorCodeServiceUnavailable ErrorCode = "SERVICE_UNAVAILABLE"
	ErrorCodeMethodNotAllowed   ErrorCode = "METHOD_NOT_ALLOWED"

	// Auth
	ErrorCodeAuthInvalidInput            ErrorCode = "AUTH_INVALID_INPUT"
	ErrorCodeInvalidCredentials          ErrorCode = "AUTH_INVALID_CREDENTIALS"
	ErrorCodeEmailAlreadyRegistered      ErrorCode = "AUTH_EMAIL_ALREADY_REGISTERED"
	ErrorCodeVerificationInvalid         ErrorCode = "AUTH_VERIFICATION_INVALID"
	ErrorCodeVerificationExpired         ErrorCode = "AUTH_VERIFICATION_EXPIRED"
	ErrorCodeVerificationTooManyAttempts ErrorCode = "AUTH_VERIFICATION_TOO_MANY_ATTEMPTS"
	ErrorCodeVerificationSendTooFrequent ErrorCode = "AUTH_VERIFICATION_SEND_TOO_FREQUENT"
	ErrorCodeSessionExpired              ErrorCode = "AUTH_SESSION_EXPIRED"
	ErrorCodeSessionRevoked              ErrorCode = "AUTH_SESSION_REVOKED"
	ErrorCodeTokenInvalid                ErrorCode = "AUTH_TOKEN_INVALID"
	ErrorCodeTokenReused                 ErrorCode = "AUTH_TOKEN_REUSED"
	ErrorCodeAccountDisabled             ErrorCode = "AUTH_ACCOUNT_DISABLED"
	ErrorCodePasswordPolicyViolation     ErrorCode = "AUTH_PASSWORD_POLICY_VIOLATION"

	// Deprecated aliases kept temporarily while call sites migrate to canonical names.
	ErrorCodeEmailExists             = ErrorCodeEmailAlreadyRegistered
	ErrorCodeInvalidVerificationCode = ErrorCodeVerificationInvalid
	ErrorCodeTokenExpired            = ErrorCodeSessionExpired
	ErrorCodeTokenRevoked            = ErrorCodeSessionRevoked
	ErrorCodePasswordMismatch        = ErrorCodePasswordPolicyViolation
	ErrorCodeEmailNotVerified        = ErrorCodeAuthInvalidInput
	ErrorCodeInvalidResetToken       = ErrorCodeVerificationInvalid
)
