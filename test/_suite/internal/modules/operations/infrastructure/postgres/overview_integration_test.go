package postgres_test

import (
	"context"
	"testing"
	"time"

	operationspostgres "github.com/StephenQiu30/hotkey-server/internal/modules/operations/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/internal/platform/queue"
	"github.com/StephenQiu30/hotkey-server/test/postgresfixture"
)

func TestJobRepositoryRuntimeOverviewCountsSafeQueueStates(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}
	store := queue.NewStore(runtime)
	if _, _, err := store.Enqueue(ctx, queue.Job{Kind: queue.KindCollectSource, UniqueKey: "overview-a", Payload: queue.Payload{EntityID: 1, EntityVersion: 1}, ScheduledAt: time.Now().UTC(), MaxAttempts: 2, Priority: 1}); err != nil {
		t.Fatal(err)
	}
	overview, err := operationspostgres.NewJobRepository(runtime).RuntimeOverview(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if overview.AvailableJobs != 1 || overview.RunningJobs != 0 || overview.OldestAvailableAt == nil || overview.GeneratedAt.IsZero() {
		t.Fatalf("overview = %#v", overview)
	}
}
