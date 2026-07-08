package queue_test

import (
	"context"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/queue"
)

// TestProducerPublish requires a running Kafka broker at localhost:9092.
func TestProducerPublish(t *testing.T) {
	t.Parallel()

	p := queue.NewProducer([]string{"localhost:9092"})
	defer p.Close()

	msg := queue.NewMessage("test.type", nil)
	err := p.Publish(context.Background(), "hotkey.test", msg)
	if err != nil {
		t.Fatalf("Publish returned error (is Kafka running?): %v", err)
	}
}
