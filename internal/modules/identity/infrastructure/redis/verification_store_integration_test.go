package redis

import (
	"net"
	"net/url"
	"os"
	"testing"
	"time"
)

func testRedisURL(t *testing.T) string {
	t.Helper()
	rawURL := os.Getenv("HOTKEY_TEST_REDIS_URL")
	if rawURL == "" {
		t.Fatal("HOTKEY_TEST_REDIS_URL is required for Redis integration tests")
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse HOTKEY_TEST_REDIS_URL: %v", err)
	}
	if parsed.Scheme != "redis" || parsed.Host == "" {
		t.Fatalf("HOTKEY_TEST_REDIS_URL = %q, want redis URL", rawURL)
	}
	connection, err := net.DialTimeout("tcp", parsed.Host, time.Second)
	if err != nil {
		t.Fatalf("connect real Redis at %s: %v", parsed.Host, err)
	}
	if err := connection.Close(); err != nil {
		t.Fatalf("close Redis probe: %v", err)
	}
	return rawURL
}
