package bootstrap

import (
	"context"
	"fmt"
	"time"

	deliverydomain "github.com/StephenQiu30/hotkey-server/internal/modules/delivery/domain"
	deliverypostgres "github.com/StephenQiu30/hotkey-server/internal/modules/delivery/infrastructure/postgres"
	reportapplication "github.com/StephenQiu30/hotkey-server/internal/modules/report/application"
	"github.com/StephenQiu30/hotkey-server/internal/modules/report/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/queue"
	platformscheduler "github.com/StephenQiu30/hotkey-server/internal/platform/scheduler"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
)

type reportAutomationReader struct {
	repository *deliverypostgres.Repository
}

func (reader reportAutomationReader) GetEnabledSubscription(ctx context.Context, subscriptionID int64) (reportapplication.AutomationSubscription, error) {
	subscription, err := reader.repository.GetEnabledSubscription(ctx, subscriptionID)
	if err != nil {
		return reportapplication.AutomationSubscription{}, err
	}
	return reportapplication.AutomationSubscription{
		ID: subscription.ID, Version: subscription.Version, MonitorID: subscription.MonitorID,
		ReportType: domain.ReportType(subscription.ReportType), Channel: string(subscription.Channel),
		Timezone: subscription.Timezone, Enabled: subscription.Enabled,
	}, nil
}

type reportSubscriptionDueReader struct {
	repository *deliverypostgres.Repository
}

func (reader reportSubscriptionDueReader) ListEnabledReportSubscriptions(ctx context.Context) ([]platformscheduler.ReportSubscription, error) {
	subscriptions, err := reader.repository.ListEnabledSubscriptions(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]platformscheduler.ReportSubscription, 0, len(subscriptions))
	for _, subscription := range subscriptions {
		items = append(items, platformscheduler.ReportSubscription{
			ID: subscription.ID, Version: subscription.Version, ReportType: subscription.ReportType,
			Timezone: subscription.Timezone, Schedule: subscription.Schedule, Enabled: subscription.Enabled,
		})
	}
	return items, nil
}

type reportDeliveryPlanner struct {
	repository *deliverypostgres.Repository
	jobs       *queue.Store
}

func (planner *reportDeliveryPlanner) Schedule(ctx context.Context, report domain.Report) error {
	if planner == nil || planner.repository == nil || planner.jobs == nil || report.ID <= 0 || report.Status != domain.ReportPublished {
		return sharedrepository.ErrUnavailable
	}
	subscriptions, err := planner.repository.ListEnabledSubscriptions(ctx)
	if err != nil {
		return err
	}
	for _, subscription := range subscriptions {
		if !subscription.Enabled || subscription.Channel != deliverydomain.ChannelEmail || subscription.ReportType != string(report.Type) || !sameMonitor(subscription.MonitorID, report.MonitorID) {
			continue
		}
		idempotencyKey := queue.StableJobHash("report-delivery", fmt.Sprint(report.ID), fmt.Sprint(subscription.ID))
		_, err := planner.repository.CreateDelivery(ctx, deliverydomain.Delivery{
			ReportID: report.ID, SubscriptionID: subscription.ID, IdempotencyKey: idempotencyKey, Status: deliverydomain.DeliveryQueued,
		})
		if err != nil {
			return err
		}
		delivery, err := planner.repository.GetDeliveryForScope(ctx, report.ID, subscription.ID)
		if err != nil {
			return err
		}
		inputHash := queue.StableJobHash(queue.KindDeliverEmail, fmt.Sprint(report.ID), fmt.Sprint(subscription.ID), idempotencyKey)
		if _, _, err := planner.jobs.Enqueue(ctx, queue.Job{
			Kind: queue.KindDeliverEmail, UniqueKey: queue.StableJobKey(queue.KindDeliverEmail, delivery.ID, 1, inputHash),
			Payload: queue.Payload{EntityID: delivery.ID, EntityVersion: 1, InputHash: inputHash}, ScheduledAt: reportPublishedAt(report), MaxAttempts: 5, Priority: 8,
		}); err != nil {
			return err
		}
	}
	return nil
}

func reportPublishedAt(report domain.Report) (valueTime time.Time) {
	if report.PublishedAt != nil && !report.PublishedAt.IsZero() {
		return report.PublishedAt.UTC()
	}
	return time.Now().UTC()
}

func sameMonitor(left, right *int64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func newReportScheduler(repository *deliverypostgres.Repository, store *queue.Store) *platformscheduler.ReportScheduler {
	return platformscheduler.NewReportScheduler(reportSubscriptionDueReader{repository: repository}, store)
}
