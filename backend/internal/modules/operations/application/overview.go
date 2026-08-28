package application

import (
	"context"
	"fmt"

	notificationapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/notification/application"
	operationsdomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/domain"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

type OverviewStore interface {
	RuntimeOverview(context.Context) (operationsdomain.RuntimeOverview, error)
}

const deliveryUnknownAlertRunbookURL = "https://github.com/StephenQiu30/hotkey-server/blob/main/docs/operations/004-%E5%8F%AF%E8%A7%82%E6%B5%8B%E6%80%A7SLO%E4%B8%8E%E4%BA%8B%E4%BB%B6%E5%93%8D%E5%BA%94.md#delivery-unknown-alert-response"

type OverviewService struct {
	store             OverviewStore
	unknownDeliveries notificationapplication.UnknownDeliveryAlertReader
}

func NewOverviewService(store OverviewStore, unknownDeliveries notificationapplication.UnknownDeliveryAlertReader) (*OverviewService, error) {
	if store == nil || unknownDeliveries == nil {
		return nil, fmt.Errorf("overview dependencies are required")
	}
	return &OverviewService{store: store, unknownDeliveries: unknownDeliveries}, nil
}

func (service *OverviewService) Get(ctx context.Context) (operationsdomain.RuntimeOverview, error) {
	if service == nil || service.store == nil || service.unknownDeliveries == nil {
		return operationsdomain.RuntimeOverview{}, sharedrepository.ErrUnavailable
	}
	overview, err := service.store.RuntimeOverview(ctx)
	if err != nil {
		return operationsdomain.RuntimeOverview{}, err
	}
	if overview.Alerts == nil {
		overview.Alerts = make([]operationsdomain.RuntimeAlert, 0, 1)
	}
	delivery, found, err := service.unknownDeliveries.UnknownDeliveryAlert(ctx)
	if err != nil {
		return operationsdomain.RuntimeOverview{}, err
	}
	if !found {
		return overview, nil
	}
	if delivery.AttemptID <= 0 || delivery.NotificationID <= 0 || delivery.ResourceID <= 0 ||
		(delivery.ResourceType != "micro_event" && delivery.ResourceType != "hotspot" && delivery.ResourceType != "report") ||
		delivery.AffectedCount <= 0 || delivery.TriggeredAt.IsZero() {
		return operationsdomain.RuntimeOverview{}, sharedrepository.ErrConstraint
	}
	overview.Alerts = append(overview.Alerts, operationsdomain.RuntimeAlert{
		AlertID: "ALERT-DELIVERY-UNKNOWN", Severity: "p1", ReasonCode: "notification_delivery_unknown",
		RunbookURL: deliveryUnknownAlertRunbookURL, AttemptID: delivery.AttemptID,
		NotificationID: delivery.NotificationID, ResourceType: delivery.ResourceType,
		ResourceID: delivery.ResourceID, AffectedCount: delivery.AffectedCount,
		TriggeredAt: delivery.TriggeredAt.UTC(),
	})
	return overview, nil
}
