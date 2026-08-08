package bootstrap

import (
	"context"
	"fmt"
	"html"
	"strings"
	"time"

	alertapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/alert/application"
	deliverydomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/delivery/domain"
	deliverypostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/delivery/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/queue"
)

type alertTransactionRunner struct{ runtime *database.Runtime }

func (runner alertTransactionRunner) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	return runner.runtime.WithinTransaction(ctx, func(transactionContext context.Context, _ database.Transaction) error { return fn(transactionContext) })
}

type alertEmailPlanner struct {
	repository *deliverypostgres.Repository
	jobs       *queue.Store
}

func (planner alertEmailPlanner) PlanAlertEmail(ctx context.Context, plan alertapplication.AlertEmailPlan) error {
	text := strings.TrimSpace(plan.Reason)
	if text == "" {
		text = plan.Title
	}
	htmlBody := "<html><body><h1>" + html.EscapeString(plan.Title) + "</h1><p>" + strings.ReplaceAll(html.EscapeString(text), "\n", "<br>\n") + "</p></body></html>"
	delivery, _, err := planner.repository.CreateAlertDelivery(ctx, deliverydomain.AlertDelivery{
		OccurrenceID: plan.OccurrenceID, IdempotencyKey: plan.IdempotencyKey, Recipient: plan.Recipient,
		Subject:  "[HotKey " + strings.ToUpper(string(plan.Severity)) + "] " + plan.Title,
		TextBody: plan.Title + "\n\n" + text, HTMLBody: htmlBody, Severity: string(plan.Severity), Status: deliverydomain.DeliveryQueued,
	})
	if err != nil {
		return err
	}
	inputHash := queue.StableJobHash(queue.KindDeliverAlertEmail, fmt.Sprint(plan.OccurrenceID), plan.IdempotencyKey)
	_, _, err = planner.jobs.Enqueue(ctx, queue.Job{
		Kind: queue.KindDeliverAlertEmail, UniqueKey: queue.StableJobKey(queue.KindDeliverAlertEmail, delivery.ID, 1, inputHash),
		Payload: queue.Payload{EntityID: delivery.ID, EntityVersion: 1, InputHash: inputHash}, ScheduledAt: time.Now().UTC(), MaxAttempts: 5, Priority: 8,
	})
	return err
}
