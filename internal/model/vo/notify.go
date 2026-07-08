package vo

// NotificationData is the JSON representation of a notification.
type NotificationData struct {
	ID             int64   `json:"id"`
	UserID         int64   `json:"user_id"`
	AlertID        int64   `json:"alert_id"`
	Channel        string  `json:"channel"`
	DeliveryStatus string  `json:"delivery_status"`
	ReadAt         *string `json:"read_at,omitempty"`
	CreatedAt      string  `json:"created_at"`
}

// MarkNotificationReadData is the JSON representation of marking a notification as read.
type MarkNotificationReadData struct {
	Read bool `json:"read" example:"true"`
}
