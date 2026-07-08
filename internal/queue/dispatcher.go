package queue

import (
	"context"
	"log"
	"sync"
)

// Handler processes a single message. Each handler is registered for one message Type.
type Handler interface {
	// Type returns the message type this handler can process (e.g. "digest.run").
	Type() string

	// Handle processes a message. Return nil on success, error on failure.
	Handle(ctx context.Context, msg Message) error

	// DedupeEnabled returns true if this handler expects message-id deduplication.
	DedupeEnabled() bool
}

// HandlerFunc is a convenience adapter for handlers that don't need dedup.
type HandlerFunc struct {
	MsgType string
	Fn      func(ctx context.Context, msg Message) error
}

func (h HandlerFunc) Type() string                           { return h.MsgType }
func (h HandlerFunc) Handle(ctx context.Context, msg Message) error { return h.Fn(ctx, msg) }
func (h HandlerFunc) DedupeEnabled() bool                     { return false }

// Dispatcher routes incoming messages to registered handlers.
type Dispatcher struct {
	mu       sync.RWMutex
	handlers map[string]Handler
	producer *Producer // used to publish to DLQ on retry exhaustion
}

func NewDispatcher(producer *Producer) *Dispatcher {
	return &Dispatcher{
		handlers: make(map[string]Handler),
		producer: producer,
	}
}

// Register adds a handler. Panics if a handler for the same type is already registered.
func (d *Dispatcher) Register(h Handler) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, exists := d.handlers[h.Type()]; exists {
		log.Panicf("dispatcher: handler for type %q already registered", h.Type())
	}
	d.handlers[h.Type()] = h
}

// Dispatch routes a message to its registered handler.
// Returns nil on success. On failure, publishes to DLQ if retries exhausted.
func (d *Dispatcher) Dispatch(ctx context.Context, msg Message) error {
	if msg.ID == "" || msg.Type == "" {
		return ErrInvalidMessage
	}

	d.mu.RLock()
	h, ok := d.handlers[msg.Type]
	d.mu.RUnlock()

	if !ok {
		return ErrHandlerNotFound
	}

	err := h.Handle(ctx, msg)
	if err == nil {
		return nil
	}

	// Handler failed — decide: retry or DLQ
	msg.RetryCount++
	if msg.RetryCount < MaxRetries {
		// Re-publish to original topic for retry
		if pubErr := d.producer.Publish(ctx, topicForType(msg.Type), msg); pubErr != nil {
			log.Printf("dispatcher: retry publish failed for %s/%s: %v", msg.Type, msg.ID, pubErr)
		}
		return err
	}

	// Retries exhausted — publish to DLQ
	dlqTopic := topicForDLQ(msg.Type)
	log.Printf("dispatcher: routing %s/%s to DLQ %s after %d retries: %v", msg.Type, msg.ID, dlqTopic, msg.RetryCount, err)
	if pubErr := d.producer.Publish(ctx, dlqTopic, msg); pubErr != nil {
		log.Printf("dispatcher: DLQ publish failed for %s/%s: %v", msg.Type, msg.ID, pubErr)
	}
	return err
}

func topicForType(msgType string) string {
	switch msgType {
	case "digest.run":
		return TopicDigestRun
	default:
		return msgType
	}
}

func topicForDLQ(msgType string) string {
	switch msgType {
	case "digest.run":
		return TopicDigestRunDLQ
	default:
		return msgType + ".dlq"
	}
}
