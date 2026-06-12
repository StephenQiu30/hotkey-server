package auth

import "time"

type User struct {
	ID          int64     `json:"id"`
	Email       string    `json:"email"`
	DisplayName string    `json:"display_name"`
	Status      string    `json:"status"`
	PlanType    string    `json:"plan_type"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
