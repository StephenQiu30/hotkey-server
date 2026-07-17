package jobs

import (
	"context"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/platform/queue"
)

func TestCollectHandlerRejectsWrongKindBeforeDependencies(t *testing.T) {
	handler := &CollectHandler{}
	err := handler.Handle(context.Background(), queue.Job{Kind: queue.KindNormalizeContent, UniqueKey: "x", ScheduledAt: time.Now().UTC(), MaxAttempts: 1, Priority: 1, Payload: queue.Payload{EntityID: 1, EntityVersion: 1}})
	if !queue.IsPermanent(err) {
		t.Fatalf("wrong kind error = %v, want permanent", err)
	}
}
