package entity

import "time"

type User struct {
	ID                 int64      `gorm:"column:id;primaryKey"`
	Email              string     `gorm:"column:email"`
	PasswordHash       string     `gorm:"column:password_hash"`
	DisplayName        string     `gorm:"column:display_name"`
	Status             string     `gorm:"column:status"`
	PlanType           string     `gorm:"column:plan_type"`
	VerificationStatus string     `gorm:"column:verification_status"`
	EmailVerifiedAt    *time.Time `gorm:"column:email_verified_at"`
	PasswordChangedAt  *time.Time `gorm:"column:password_changed_at"`
	LastLoginAt        *time.Time `gorm:"column:last_login_at"`
	CreatedAt          time.Time  `gorm:"column:created_at"`
	UpdatedAt          time.Time  `gorm:"column:updated_at"`
}

func (User) TableName() string { return "users" }
