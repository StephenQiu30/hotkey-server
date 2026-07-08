package queue_test

import (
	"context"
	"os"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/queue"
	"github.com/StephenQiu30/hotkey-server/tests/testutil"
)

func TestProducerPublish(t *testing.T) {
	testutil.SkipIfNoKafka(t)

	brokers := []string{"localhost:9092"}
	if env := os.Getenv("TEST_KAFKA_BROKERS"); env != "" {
		brokers = []string{env}
	}

	p := queue.NewProducer(brokers)
	defer p.Close()

	msg := queue.NewMessage("test.type", nil)
	err := p.Publish(context.Background(), "hotkey.test", msg)
	if err != nil {
		t.Fatalf("Publish returned error: %v", err)
	}
}
