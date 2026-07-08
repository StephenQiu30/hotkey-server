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

type testHandlerWithDedup struct {
	testHandler
}

func (h *testHandlerWithDedup) DedupeEnabled() bool { return true }

func TestDispatcherRoutesToCorrectHandler(t *testing.T) {
	t.Parallel()

	d := queue.NewDispatcher(nil, nil)
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

	d := queue.NewDispatcher(nil, nil)
	msg := queue.NewMessage("unknown.type", json.RawMessage(`{}`))
	err := d.Dispatch(context.Background(), msg)
	if err != queue.ErrHandlerNotFound {
		t.Fatalf("error = %v, want ErrHandlerNotFound", err)
	}
}

func TestDispatcherRejectsInvalidMessage(t *testing.T) {
	t.Parallel()

	d := queue.NewDispatcher(nil, nil)
	err := d.Dispatch(context.Background(), queue.Message{})
	if err != queue.ErrInvalidMessage {
		t.Fatalf("error = %v, want ErrInvalidMessage", err)
	}
}

func TestDispatcherDLQRecorderCalledOnRetryExhaustion(t *testing.T) {
	t.Parallel()

	d := queue.NewDispatcher(nil, nil)
	h := &testHandler{msgType: "test.fail", shouldFail: true}
	d.Register(h)

	var recordedTopic string
	var recordedMsg queue.Message
	var recordedErrMsg string
	recorded := make(chan struct{}, 1)

	d.SetDLQRecorder(func(ctx context.Context, topic string, msg queue.Message, errMsg string) {
		recordedTopic = topic
		recordedMsg = msg
		recordedErrMsg = errMsg
		recorded <- struct{}{}
	})

	msg := queue.NewMessage("test.fail", json.RawMessage(`{}`))
	msg.RetryCount = queue.MaxRetries // Start at max retries so it goes straight to DLQ

	err := d.Dispatch(context.Background(), msg)
	if err == nil {
		t.Fatal("Dispatch should return error when handler fails")
	}

	// Wait for the recorder to be called (it's synchronous in the same goroutine)
	<-recorded

	if recordedTopic != "test.fail.dlq" {
		t.Fatalf("recorded topic = %q, want %q", recordedTopic, "test.fail.dlq")
	}
	if recordedMsg.ID != msg.ID {
		t.Fatalf("recorded message ID = %q, want %q", recordedMsg.ID, msg.ID)
	}
	if recordedErrMsg != errTestFail.Error() {
		t.Fatalf("recorded error = %q, want %q", recordedErrMsg, errTestFail.Error())
	}
}
