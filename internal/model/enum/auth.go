package enum

// VerificationPurpose defines the purpose of a verification code.
type VerificationPurpose string

const (
	VerificationPurposeRegister     VerificationPurpose = "register"
	VerificationPurposeResetPassword VerificationPurpose = "reset_password"
)

// AccountStatus defines the lifecycle state of a user account.
type AccountStatus string

const (
	AccountStatusActive     AccountStatus = "active"
	AccountStatusDisabled   AccountStatus = "disabled"
	AccountStatusUnverified AccountStatus = "unverified"
)

// SessionRevokeReason defines why a session was revoked.
type SessionRevokeReason string

const (
	SessionRevokeReasonLogout        SessionRevokeReason = "logout"
	SessionRevokeReasonPasswordReset SessionRevokeReason = "password_reset"
	SessionRevokeReasonTokenReuse    SessionRevokeReason = "token_reuse"
	SessionRevokeReasonAdmin         SessionRevokeReason = "admin"
)
