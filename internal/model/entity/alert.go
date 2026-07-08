package entity

import "time"

type Alert struct {
	ID            int64     `gorm:"column:id;primaryKey"`
	MonitorID     int64     `gorm:"column:monitor_id"`
	TopicID       *int64    `gorm:"column:topic_id"`
	AlertType     string    `gorm:"column:alert_type"`
	Title         string    `gorm:"column:title"`
	Message       string    `gorm:"column:message"`
	Severity      string    `gorm:"column:severity"`
	TriggerScore  float64   `gorm:"column:trigger_score"`
	TriggerReason string    `gorm:"column:trigger_reason"`
	CreatedAt     time.Time `gorm:"column:created_at"`
}

func (Alert) TableName() string { return "alerts" }

type UserNotification struct {
	ID             int64      `gorm:"column:id;primaryKey"`
	UserID         int64      `gorm:"column:user_id"`
	AlertID        int64      `gorm:"column:alert_id"`
	Channel        string     `gorm:"column:channel"`
	DeliveryStatus string     `gorm:"column:delivery_status"`
	ReadAt         *time.Time `gorm:"column:read_at"`
	SentAt         *time.Time `gorm:"column:sent_at"`
	CreatedAt      time.Time  `gorm:"column:created_at"`
}

func (UserNotification) TableName() string { return "user_notifications" }

type EmailDelivery struct {
	ID                int64      `gorm:"column:id;primaryKey"`
	NotificationID    int64      `gorm:"column:notification_id"`
	RecipientEmail    string     `gorm:"column:recipient_email"`
	Provider          string     `gorm:"column:provider"`
	ProviderMessageID string     `gorm:"column:provider_message_id"`
	Status            string     `gorm:"column:status"`
	ErrorMessage      string     `gorm:"column:error_message"`
	SentAt            *time.Time `gorm:"column:sent_at"`
}

func (EmailDelivery) TableName() string { return "email_deliveries" }
