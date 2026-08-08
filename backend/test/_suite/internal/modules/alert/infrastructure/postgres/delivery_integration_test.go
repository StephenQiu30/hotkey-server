//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/alert/domain"
	deliverydomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/delivery/domain"
	deliverypostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/delivery/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/queue"
)

func TestOccurrenceDeliveryAndJobAreAtomicAndUniqueUnderConcurrency(t *testing.T) {
	fixture := newAlertRepositoryFixture(t)
	deliveries := deliverypostgres.NewRepository(fixture.runtime)
	jobs := queue.NewStore(fixture.runtime)
	command := fixture.command(fixture.updateID, time.Date(2026, 8, 4, 7, 0, 0, 0, time.UTC))

	var wait sync.WaitGroup
	errorsChannel := make(chan error, 2)
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			errorsChannel <- writeAlertDeliveryTransaction(context.Background(), fixture.runtime, fixture.repository, deliveries, jobs, command, nil)
		}()
	}
	wait.Wait()
	close(errorsChannel)
	for err := range errorsChannel {
		if err != nil {
			t.Fatal(err)
		}
	}

	for table, want := range map[string]int{"alert_occurrences": 1, "alert_email_deliveries": 1, "river_job": 1} {
		var got int
		if err := fixture.runtime.SQL.QueryRowContext(context.Background(), `SELECT count(*) FROM `+table).Scan(&got); err != nil || got != want {
			t.Fatalf("%s count = %d/%v, want %d", table, got, err, want)
		}
	}
}

func TestAlertDeliveryPlannerFailureRollsBackOccurrenceAndDelivery(t *testing.T) {
	fixture := newAlertRepositoryFixture(t)
	deliveries := deliverypostgres.NewRepository(fixture.runtime)
	jobs := queue.NewStore(fixture.runtime)
	injected := errors.New("injected planner failure")
	err := writeAlertDeliveryTransaction(context.Background(), fixture.runtime, fixture.repository, deliveries, jobs, fixture.command(fixture.updateID, time.Now().UTC()), injected)
	if !errors.Is(err, injected) {
		t.Fatalf("transaction error = %v", err)
	}
	for _, table := range []string{"alert_threads", "alert_occurrences", "alert_email_deliveries", "river_job"} {
		var count int
		if err := fixture.runtime.SQL.QueryRowContext(context.Background(), `SELECT count(*) FROM `+table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("%s count after rollback = %d/%v", table, count, err)
		}
	}
}

func writeAlertDeliveryTransaction(ctx context.Context, runtime *database.Runtime, alerts interface {
	RecordOccurrence(context.Context, domain.RecordOccurrenceCommand) (domain.RecordOccurrenceResult, error)
}, deliveries *deliverypostgres.Repository, jobs *queue.Store, command domain.RecordOccurrenceCommand, fail error) error {
	return runtime.WithinTransaction(ctx, func(transactionContext context.Context, _ database.Transaction) error {
		recorded, err := alerts.RecordOccurrence(transactionContext, command)
		if err != nil || !recorded.Created || !recorded.Disturb {
			return err
		}
		delivery, _, err := deliveries.CreateAlertDelivery(transactionContext, deliverydomain.AlertDelivery{
			OccurrenceID: recorded.Occurrence.ID, IdempotencyKey: recorded.Occurrence.Fingerprint, Recipient: "owner@example.test",
			Subject: "HotKey warning", TextBody: "body", HTMLBody: "<p>body</p>", Severity: "warning", Status: deliverydomain.DeliveryQueued,
		})
		if err != nil {
			return err
		}
		if fail != nil {
			return fail
		}
		inputHash := queue.StableJobHash(queue.KindDeliverAlertEmail, fmt.Sprint(recorded.Occurrence.ID), strings.Repeat("a", 64))
		_, _, err = jobs.Enqueue(transactionContext, queue.Job{Kind: queue.KindDeliverAlertEmail, UniqueKey: queue.StableJobKey(queue.KindDeliverAlertEmail, delivery.ID, 1, inputHash), Payload: queue.Payload{EntityID: delivery.ID, EntityVersion: 1, InputHash: inputHash}, ScheduledAt: command.TriggeredAt, MaxAttempts: 5, Priority: 8})
		return err
	})
}
