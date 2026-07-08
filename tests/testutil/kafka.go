package testutil

import (
	"os"
	"testing"
)

// SkipIfNoKafka skips the current test when no Kafka broker URL is available.
// Tests that require Kafka should call this at the start.
// Set TEST_KAFKA_BROKERS to enable Kafka-dependent tests.
func SkipIfNoKafka(t *testing.T) {
	t.Helper()

	if os.Getenv("TEST_KAFKA_BROKERS") != "" {
		return
	}

	t.Skip("skipping: TEST_KAFKA_BROKERS is not set (no Kafka broker available)")
}
