//go:build e2e

package e2e_test

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"
)

func e2ePostgresAddr(t *testing.T) string {
	t.Helper()
	if addr := os.Getenv("HOTKEY_E2E_POSTGRES_ADDR"); addr != "" {
		return addr
	}
	return "127.0.0.1:15432"
}

func e2eRedisURL(t *testing.T) string {
	t.Helper()
	if addr := os.Getenv("HOTKEY_E2E_REDIS_URL"); addr != "" {
		return addr
	}
	return "redis://127.0.0.1:16379/0"
}

func e2eServerURL(t *testing.T) string {
	t.Helper()
	if addr := os.Getenv("HOTKEY_E2E_SERVER_URL"); addr != "" {
		return addr
	}
	return "http://127.0.0.1:18080"
}

// TestHealthCheck_PostgreSQL verifies the E2E PostgreSQL port is accepting connections.
func TestHealthCheck_PostgreSQL(t *testing.T) {
	addr := e2ePostgresAddr(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := pingTCP(ctx, addr); err != nil {
		t.Fatalf("E2E PostgreSQL not reachable at %s: %v", addr, err)
	}
	t.Logf("E2E PostgreSQL healthy at %s", addr)
}

// TestHealthCheck_Redis verifies the E2E Redis responds to PING.
func TestHealthCheck_Redis(t *testing.T) {
	rawURL := e2eRedisURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := redisPing(ctx, rawURL); err != nil {
		t.Fatalf("E2E Redis not reachable: %v", err)
	}
	t.Logf("E2E Redis healthy at %s", rawURL)
}

// TestHealthCheck_ServerHTTP verifies the E2E server responds to /healthz.
// Skipped when HOTKEY_E2E_SERVER_URL is not set (e.g. CI without a running server).
func TestHealthCheck_ServerHTTP(t *testing.T) {
	if os.Getenv("HOTKEY_E2E_SERVER_URL") == "" {
		t.Skip("HOTKEY_E2E_SERVER_URL not set, skipping server health check")
	}
	baseURL := e2eServerURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/healthz", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("E2E server not reachable at %s/healthz: %v", baseURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("E2E server /healthz returned status %d, want 200", resp.StatusCode)
	}
	t.Logf("E2E server healthy at %s/healthz", baseURL)
}
