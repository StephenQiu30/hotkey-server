package queue_test

import (
	"testing"

	queueutil "github.com/StephenQiu30/hotkey-server/internal/platform/queue"
)

func TestNewClient(t *testing.T) {
	client := queueutil.NewClient("localhost:6379")
	if client == nil {
		t.Fatal("NewClient returned nil")
	}
	defer client.Close()
}

func TestNewServer(t *testing.T) {
	srv := queueutil.NewServer("localhost:6379", 10)
	if srv == nil {
		t.Fatal("NewServer returned nil")
	}
}

func TestNewServeMux(t *testing.T) {
	mux := queueutil.NewServeMux()
	if mux == nil {
		t.Fatal("NewServeMux returned nil")
	}
}
