package redis_test

import (
	"context"
	"testing"

	redisutil "github.com/StephenQiu30/hotkey-server/internal/platform/redis"
)

func TestNewClient(t *testing.T) {
	client := redisutil.NewClient("localhost:6379")
	if client == nil {
		t.Fatal("NewClient returned nil")
	}
	defer client.Close()

	if client.Options().Addr != "localhost:6379" {
		t.Errorf("expected addr localhost:6379, got %s", client.Options().Addr)
	}
}

func TestHealthCheck_Unreachable(t *testing.T) {
	client := redisutil.NewClient("localhost:1")
	defer client.Close()

	err := redisutil.HealthCheck(context.Background(), client)
	if err == nil {
		t.Fatal("expected error for unreachable redis, got nil")
	}
}
