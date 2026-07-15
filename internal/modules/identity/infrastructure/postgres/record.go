package postgres

import (
	"database/sql"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/modules/identity/domain"
)

type userRecord struct {
	ID           int64
	Email        string
	PasswordHash string
	DisplayName  string
	Role         string
	Status       string
	LastLoginAt  sql.NullTime
	CreatedAt    time.Time
	UpdatedAt    time.Time
	DeletedAt    sql.NullTime
}

func (record userRecord) domainUser() domain.User {
	user := domain.User{
		ID:           record.ID,
		Email:        record.Email,
		PasswordHash: record.PasswordHash,
		DisplayName:  record.DisplayName,
		Role:         domain.Role(record.Role),
		Status:       domain.UserStatus(record.Status),
		CreatedAt:    record.CreatedAt.UTC(),
		UpdatedAt:    record.UpdatedAt.UTC(),
	}
	if record.LastLoginAt.Valid {
		value := record.LastLoginAt.Time.UTC()
		user.LastLoginAt = &value
	}
	if record.DeletedAt.Valid {
		value := record.DeletedAt.Time.UTC()
		user.DeletedAt = &value
	}
	return user
}

type sessionRecord struct {
	ID                int64
	UserID            int64
	FamilyID          string
	AbsoluteExpiresAt time.Time
	LastSeenAt        time.Time
	RevokedAt         sql.NullTime
	RevokeReason      sql.NullString
	CreatedAt         time.Time
}

func (record sessionRecord) domainSession() domain.Session {
	session := domain.Session{
		ID:                record.ID,
		UserID:            record.UserID,
		FamilyID:          record.FamilyID,
		AbsoluteExpiresAt: record.AbsoluteExpiresAt.UTC(),
		LastSeenAt:        record.LastSeenAt.UTC(),
		RevokeReason:      record.RevokeReason.String,
		CreatedAt:         record.CreatedAt.UTC(),
	}
	if record.RevokedAt.Valid {
		value := record.RevokedAt.Time.UTC()
		session.RevokedAt = &value
	}
	return session
}

type refreshTokenRecord struct {
	ID        int64
	SessionID int64
	TokenHash string
	ExpiresAt time.Time
	UsedAt    sql.NullTime
	RevokedAt sql.NullTime
	CreatedAt time.Time
}

func (record refreshTokenRecord) domainRefreshToken() domain.RefreshToken {
	token := domain.RefreshToken{
		ID:        record.ID,
		SessionID: record.SessionID,
		TokenHash: record.TokenHash,
		ExpiresAt: record.ExpiresAt.UTC(),
		CreatedAt: record.CreatedAt.UTC(),
	}
	if record.UsedAt.Valid {
		value := record.UsedAt.Time.UTC()
		token.UsedAt = &value
	}
	if record.RevokedAt.Valid {
		value := record.RevokedAt.Time.UTC()
		token.RevokedAt = &value
	}
	return token
}
