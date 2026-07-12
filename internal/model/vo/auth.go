package vo

import "time"

// UserData is the JSON representation of a user (nested inside ResponseBody.Data).
type UserData struct {
	ID          int64  `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
}

// LoginData is the JSON representation of a login response (nested inside ResponseBody.Data).
type LoginData struct {
	User  UserData `json:"user"`
	Token string   `json:"token"`
}

// VerificationSendData is the response data for sending a verification code.
type VerificationSendData struct {
	Email          string `json:"email"`
	ExpiresInSecs  int    `json:"expires_in_secs"`
	CoolDownSecs   int    `json:"cooldown_secs"`
}

// VerificationTicketData is the response data containing a verification ticket for registration.
type VerificationTicketData struct {
	Ticket string `json:"ticket"`
	ExpiresInSecs int `json:"expires_in_secs"`
}

// AuthenticatedUserData is the public-safe representation of an authenticated user.
type AuthenticatedUserData struct {
	ID           int64      `json:"id"`
	Email        string     `json:"email"`
	DisplayName  string     `json:"display_name"`
	Status       string     `json:"status"`
	PlanType     string     `json:"plan_type"`
	EmailVerifiedAt *time.Time `json:"email_verified_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

// AuthTokenData is the public-safe representation of an authentication token pair.
type AuthTokenData struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
}

// SessionData is the public-safe representation of an active session.
type SessionData struct {
	SessionID    int64     `json:"session_id"`
	Status       string    `json:"status"`
	IPAddress    string    `json:"ip_address"`
	UserAgent    string    `json:"user_agent"`
	ExpiresAt    time.Time `json:"expires_at"`
	LastRefreshedAt time.Time `json:"last_refreshed_at"`
	CreatedAt    time.Time `json:"created_at"`
}

// OperationResultData is a generic success indicator.
type OperationResultData struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}
