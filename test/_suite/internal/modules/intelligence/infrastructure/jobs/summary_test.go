package jobs

import (
	"context"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/platform/queue"
)

func TestSummaryHandlerRejectsWrongKind(t *testing.T) {
	handler := &SummaryHandler{}
	job := queue.Job{Kind: queue.KindClusterContent, UniqueKey: "x", ScheduledAt: time.Now().UTC(), MaxAttempts: 1, Priority: 1, Payload: queue.Payload{EntityID: 1, EntityVersion: 1}}
	if err := handler.Handle(context.Background(), job); !queue.IsPermanent(err) {
		t.Fatalf("summary wrong kind = %v", err)
	}
}
