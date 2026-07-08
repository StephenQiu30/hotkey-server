package queue_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/queue"
)

var errTestFail = errors.New("handler failed as expected")

type testHandler struct {
	msgType    string
	shouldFail bool
	callCount  atomic.Int64
}

func (h *testHandler) Type() string { return h.msgType }
func (h *testHandler) Handle(ctx context.Context, msg queue.Message) error {
	h.callCount.Add(1)
	if h.shouldFail {
		return errTestFail
	}
	return nil
}
func (h *testHandler) DedupeEnabled() bool { return false }

func TestDispatcherRoutesToCorrectHandler(t *testing.T) {
	t.Parallel()

	d := queue.NewDispatcher(nil)
	h := &testHandler{msgType: "test.type"}
	d.Register(h)

	msg := queue.NewMessage("test.type", json.RawMessage(`{}`))
	err := d.Dispatch(context.Background(), msg)
	if err != nil {
		t.Fatalf("Dispatch returned error: %v", err)
	}
	if h.callCount.Load() != 1 {
		t.Fatalf("handler called %d times, want 1", h.callCount.Load())
	}
}

func TestDispatcherReturnsErrorForUnknownType(t *testing.T) {
	t.Parallel()

	d := queue.NewDispatcher(nil)
	msg := queue.NewMessage("unknown.type", json.RawMessage(`{}`))
	err := d.Dispatch(context.Background(), msg)
	if err != queue.ErrHandlerNotFound {
		t.Fatalf("error = %v, want ErrHandlerNotFound", err)
	}
}

func TestDispatcherRejectsInvalidMessage(t *testing.T) {
	t.Parallel()

	d := queue.NewDispatcher(nil)
	err := d.Dispatch(context.Background(), queue.Message{})
	if err != queue.ErrInvalidMessage {
		t.Fatalf("error = %v, want ErrInvalidMessage", err)
	}
}
