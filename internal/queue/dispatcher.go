package queue

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/StephenQiu30/hotkey-server/internal/platform/logging"
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

func (h HandlerFunc) Type() string                                  { return h.MsgType }
func (h HandlerFunc) Handle(ctx context.Context, msg Message) error { return h.Fn(ctx, msg) }
func (h HandlerFunc) DedupeEnabled() bool                           { return false }

// Dispatcher routes incoming messages to registered handlers.
type Dispatcher struct {
	mu        sync.RWMutex
	handlers  map[string]Handler
	producer  *Producer // used to publish to DLQ on retry exhaustion
	dedupe    *Dedupe
	recordDLQ func(ctx context.Context, topic string, msg Message, errMsg string)
}

func NewDispatcher(producer *Producer, dedupe *Dedupe) *Dispatcher {
	return &Dispatcher{
		handlers: make(map[string]Handler),
		producer: producer,
		dedupe:   dedupe,
	}
}

// SetDLQRecorder sets a callback for persisting DLQ records to the database.
func (d *Dispatcher) SetDLQRecorder(fn func(ctx context.Context, topic string, msg Message, errMsg string)) {
	d.recordDLQ = fn
}

// Register adds a handler. Panics if a handler for the same type is already registered.
func (d *Dispatcher) Register(h Handler) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, exists := d.handlers[h.Type()]; exists {
		logging.L().Panic("dispatcher handler already registered",
			zap.String("type", h.Type()),
		)
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

	// Dedup check — skip if already processed
	// Note: we only check dedup BEFORE the handler. Marking happens
	// after successful execution (below) so retries work correctly.
	if d.dedupe != nil && h.DedupeEnabled() {
		alreadySeen, err := d.dedupe.Seen(ctx, msg.ID)
		if err != nil {
			logging.Ctx(ctx).Error("dispatcher dedup error",
				zap.String("msg_type", msg.Type),
				zap.String("msg_id", msg.ID),
				zap.Error(err),
			)
			// fall through
		} else if alreadySeen {
			logging.Ctx(ctx).Info("dispatcher skipping duplicate",
				zap.String("msg_type", msg.Type),
				zap.String("msg_id", msg.ID),
			)
			return nil
		}
	}

	err := h.Handle(ctx, msg)
	if err == nil {
		// Mark as seen only after successful execution — ensures retries
		// are not blocked by the dedup key for the same message ID.
		if d.dedupe != nil && h.DedupeEnabled() {
			if markErr := d.dedupe.Mark(ctx, msg.ID); markErr != nil {
				logging.Ctx(ctx).Error("dispatcher dedup mark error",
					zap.String("msg_type", msg.Type),
					zap.String("msg_id", msg.ID),
					zap.Error(markErr),
				)
			}
		}
		return nil
	}

	// Handler failed — decide: retry or DLQ
	msg.RetryCount++
	if msg.RetryCount < MaxRetries {
		// Re-publish to original topic for retry with delay
		time.Sleep(1 * time.Second)
		if d.producer == nil {
			logging.Ctx(ctx).Warn("dispatcher retry skipped (no producer configured)",
				zap.String("msg_type", msg.Type),
				zap.String("msg_id", msg.ID),
			)
		} else if pubErr := d.producer.Publish(ctx, topicForType(msg.Type), msg); pubErr != nil {
			logging.Ctx(ctx).Error("dispatcher retry publish failed",
				zap.String("msg_type", msg.Type),
				zap.String("msg_id", msg.ID),
				zap.Error(pubErr),
			)
		}
		return err
	}

	// Retries exhausted — publish to DLQ
	dlqTopic := topicForDLQ(msg.Type)
	logging.Ctx(ctx).Warn("dispatcher routing to DLQ",
		zap.String("msg_type", msg.Type),
		zap.String("msg_id", msg.ID),
		zap.String("dlq_topic", dlqTopic),
		zap.Int("retries", msg.RetryCount),
		zap.Error(err),
	)

	if d.producer != nil {
		if pubErr := d.producer.Publish(ctx, dlqTopic, msg); pubErr != nil {
			logging.Ctx(ctx).Error("dispatcher DLQ publish failed",
				zap.String("msg_type", msg.Type),
				zap.String("msg_id", msg.ID),
				zap.Error(pubErr),
			)
		}
	} else {
		logging.Ctx(ctx).Warn("dispatcher DLQ publish skipped (no producer configured)",
			zap.String("msg_type", msg.Type),
			zap.String("msg_id", msg.ID),
		)
	}

	// Persist DLQ record to database
	if d.recordDLQ != nil {
		d.recordDLQ(ctx, dlqTopic, msg, err.Error())
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
