package entity

import "time"

// DLQRecord represents a message that was routed to the dead letter queue.
type DLQRecord struct {
	Topic       string    `json:"topic"`
	MessageID   string    `json:"message_id"`
	MessageType string    `json:"message_type"`
	Payload     string    `json:"payload"`
	ErrorMsg    string    `json:"error_msg"`
	RetryCount  int       `json:"retry_count"`
	CreatedAt   time.Time `json:"created_at"`
}
