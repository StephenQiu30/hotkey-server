package entity

import "time"

// AuthSession represents a persistent user authentication session.
type AuthSession struct {
	ID                int64     `gorm:"column:id;primaryKey"`
	UserID            int64     `gorm:"column:user_id;not null;index:idx_auth_sessions_user_id;constraint:OnDelete:CASCADE"`
	TokenHash         string    `gorm:"column:token_hash;not null;uniqueIndex"`
	FamilyHash        string    `gorm:"column:family_hash;not null;index:idx_auth_sessions_family_hash"`
	Status            string    `gorm:"column:status;not null;default:'active'"`
	IPAddress         string    `gorm:"column:ip_address;not null;default:''"`
	UserAgent         string    `gorm:"column:user_agent;not null;default:''"`
	ExpiresAt         time.Time `gorm:"column:expires_at;not null;index:idx_auth_sessions_expires_at"`
	AbsoluteExpiresAt time.Time `gorm:"column:absolute_expires_at;not null"`
	LastRefreshedAt   *time.Time `gorm:"column:last_refreshed_at"`
	CreatedAt         time.Time `gorm:"column:created_at;not null"`
	UpdatedAt         time.Time `gorm:"column:updated_at;not null"`
}

func (AuthSession) TableName() string { return "auth_sessions" }
