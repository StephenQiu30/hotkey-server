package queue

import (
	"context"
	"encoding/json"
	"sync/atomic"

	"github.com/segmentio/kafka-go"
)

// Producer publishes messages to Kafka topics.
type Producer struct {
	writer *kafka.Writer
	closed atomic.Bool
}

// NewProducer creates a Producer connected to the given brokers.
func NewProducer(brokers []string) *Producer {
	w := &kafka.Writer{
		Addr:         kafka.TCP(brokers...),
		Balancer:     &kafka.LeastBytes{},
		RequiredAcks: kafka.RequireAll, // wait for all ISR replicas
		Async:        false,             // synchronous for error propagation
	}
	return &Producer{writer: w}
}

// Publish sends a message to the given topic. Returns error if closed or write fails.
func (p *Producer) Publish(ctx context.Context, topic string, msg Message) error {
	if p.closed.Load() {
		return ErrProducerClosed
	}
	bytes, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return p.writer.WriteMessages(ctx, kafka.Message{
		Topic: topic,
		Key:   []byte(msg.Type + ":" + msg.ID),
		Value: bytes,
	})
}

// Close shuts down the underlying writer.
func (p *Producer) Close() error {
	p.closed.Store(true)
	return p.writer.Close()
}
