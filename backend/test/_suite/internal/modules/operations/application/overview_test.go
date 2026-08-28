package application_test

import (
	"context"
	"testing"
	"time"

	notificationapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/notification/application"
	operationsapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/application"
	operationsdomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/domain"
)

type overviewStoreFake struct {
	overview operationsdomain.RuntimeOverview
}

func (fake overviewStoreFake) RuntimeOverview(context.Context) (operationsdomain.RuntimeOverview, error) {
	return fake.overview, nil
}

type unknownDeliveryAlertReaderFake struct {
	summary notificationapplication.UnknownDeliveryAlertSummary
	found   bool
}

func (fake unknownDeliveryAlertReaderFake) UnknownDeliveryAlert(context.Context) (notificationapplication.UnknownDeliveryAlertSummary, bool, error) {
	return fake.summary, fake.found, nil
}

func TestOverviewServiceAppendsBoundedUnknownDeliveryAlert(t *testing.T) {
	triggeredAt := time.Date(2026, time.August, 28, 8, 30, 0, 0, time.UTC)
	service, err := operationsapplication.NewOverviewService(
		overviewStoreFake{overview: operationsdomain.RuntimeOverview{Alerts: []operationsdomain.RuntimeAlert{{AlertID: "ALERT-RIVER-JOB-FAILED"}}}},
		unknownDeliveryAlertReaderFake{found: true, summary: notificationapplication.UnknownDeliveryAlertSummary{
			AttemptID: 91, NotificationID: 52, ResourceType: "micro_event", ResourceID: 43,
			AffectedCount: 2, TriggeredAt: triggeredAt,
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	overview, err := service.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(overview.Alerts) != 2 {
		t.Fatalf("alerts = %#v", overview.Alerts)
	}
	alert := overview.Alerts[1]
	if alert.AlertID != "ALERT-DELIVERY-UNKNOWN" || alert.Severity != "p1" ||
		alert.ReasonCode != "notification_delivery_unknown" || alert.AttemptID != 91 ||
		alert.NotificationID != 52 || alert.ResourceType != "micro_event" || alert.ResourceID != 43 ||
		alert.AffectedCount != 2 || !alert.TriggeredAt.Equal(triggeredAt) || alert.RunbookURL == "" {
		t.Fatalf("delivery alert = %#v", alert)
	}
}

func TestOverviewServiceKeepsAlertsNonNilWithoutUnknownDeliveries(t *testing.T) {
	service, err := operationsapplication.NewOverviewService(
		overviewStoreFake{overview: operationsdomain.RuntimeOverview{}},
		unknownDeliveryAlertReaderFake{},
	)
	if err != nil {
		t.Fatal(err)
	}
	overview, err := service.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if overview.Alerts == nil || len(overview.Alerts) != 0 {
		t.Fatalf("healthy alerts = %#v", overview.Alerts)
	}
}
