package jobs

import (
	"context"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/platform/queue"
)

func TestEventHandlersRejectWrongKinds(t *testing.T) {
	job := queue.Job{Kind: queue.KindEvaluateRelevance, UniqueKey: "x", ScheduledAt: time.Now().UTC(), MaxAttempts: 1, Priority: 1, Payload: queue.Payload{EntityID: 1, EntityVersion: 1}}
	if err := (&ClusterHandler{}).Handle(context.Background(), job); !queue.IsPermanent(err) {
		t.Fatalf("cluster wrong kind = %v", err)
	}
	if err := (&HeatHandler{}).Handle(context.Background(), job); !queue.IsPermanent(err) {
		t.Fatalf("heat wrong kind = %v", err)
	}
}
