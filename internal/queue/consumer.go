package queue

import (
	"context"
	"encoding/json"
	"log"
	"sync/atomic"

	"github.com/segmentio/kafka-go"
)

// Consumer reads messages from a Kafka topic and dispatches them.
type Consumer struct {
	reader     *kafka.Reader
	dispatcher *Dispatcher
	closed     atomic.Bool
}

// NewConsumer creates a Consumer for one topic with consumer-group coordination.
func NewConsumer(brokers []string, topic string, groupID string, dispatcher *Dispatcher) *Consumer {
	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:     brokers,
		Topic:       topic,
		GroupID:     groupID,
		StartOffset: kafka.LastOffset,
		MinBytes:    1,
		MaxBytes:    1e6,
	})
	return &Consumer{reader: r, dispatcher: dispatcher}
}

// Run starts the poll-dispatch loop. Blocks until ctx is cancelled or a fatal error.
func (c *Consumer) Run(ctx context.Context) error {
	for {
		if c.closed.Load() {
			return ErrConsumerClosed
		}
		km, err := c.reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil // normal shutdown
			}
			log.Printf("consumer: fetch error: %v", err)
			continue
		}

		var msg Message
		if err := json.Unmarshal(km.Value, &msg); err != nil {
			log.Printf("consumer: unmarshal error (committing offset): %v", err)
			if commitErr := c.reader.CommitMessages(ctx, km); commitErr != nil {
				log.Printf("consumer: commit offset error after unmarshal failure: %v", commitErr)
			}
			continue
		}

		// Dispatch
		if err := c.dispatcher.Dispatch(ctx, msg); err != nil {
			log.Printf("consumer: dispatch error for %s/%s: %v", msg.Type, msg.ID, err)
			// Still commit offset — already retried/DLQ'd inside dispatcher
		}

		if commitErr := c.reader.CommitMessages(ctx, km); commitErr != nil {
			log.Printf("consumer: commit offset error: %v", commitErr)
		}
	}
}

// Close shuts down the consumer reader.
func (c *Consumer) Close() error {
	c.closed.Store(true)
	return c.reader.Close()
}
