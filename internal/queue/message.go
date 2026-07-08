package queue

import (
	"encoding/json"
	"errors"
	"time"
)

var (
	ErrHandlerNotFound = errors.New("no handler registered for message type")
	ErrInvalidMessage  = errors.New("invalid message: missing id, type, or payload")
	ErrProducerClosed  = errors.New("producer is closed")
	ErrConsumerClosed  = errors.New("consumer is closed")
)

const (
	// Topic names
	TopicDigestRun  = "hotkey.digest.run"
	TopicCollectRun = "hotkey.collect.run"
	TopicNotifyRun  = "hotkey.notify.run"

	// DLQ topic names
	TopicDigestRunDLQ  = "hotkey.digest.run.dlq"
	TopicCollectRunDLQ = "hotkey.collect.run.dlq"
	TopicNotifyRunDLQ  = "hotkey.notify.run.dlq"

	// DLQ config
	MaxRetries = 3
)

// Message is the universal message envelope for all queue operations.
type Message struct {
	ID         string          `json:"id"`
	Type       string          `json:"type"`
	Payload    json.RawMessage `json:"payload"`
	CreatedAt  time.Time       `json:"created_at"`
	RetryCount int             `json:"retry_count"`
}

func NewMessage(msgType string, payload json.RawMessage) Message {
	return Message{
		ID:        msgType + "-" + time.Now().Format("150405.000000"),
		Type:      msgType,
		Payload:   payload,
		CreatedAt: time.Now(),
	}
}
