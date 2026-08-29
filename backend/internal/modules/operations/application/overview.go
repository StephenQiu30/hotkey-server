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
	overview.AlertPolicyVersion = operationsdomain.RuntimeAlertPolicyVersion
	if overview.Alerts == nil {
		overview.Alerts = make([]operationsdomain.RuntimeAlert, 0, 1)
	}
	for index := range overview.Alerts {
		alert, found := operationsdomain.ApplyRuntimeAlertPolicy(overview.Alerts[index])
		if !found {
			return operationsdomain.RuntimeOverview{}, sharedrepository.ErrConstraint
		}
		overview.Alerts[index] = alert
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
	alert, found := operationsdomain.ApplyRuntimeAlertPolicy(operationsdomain.RuntimeAlert{
		AlertID: "ALERT-DELIVERY-UNKNOWN", AttemptID: delivery.AttemptID,
		NotificationID: delivery.NotificationID, ResourceType: delivery.ResourceType,
		ResourceID: delivery.ResourceID, AffectedCount: delivery.AffectedCount,
		TriggeredAt: delivery.TriggeredAt.UTC(),
	})
	if !found {
		return operationsdomain.RuntimeOverview{}, sharedrepository.ErrConstraint
	}
	overview.Alerts = append(overview.Alerts, alert)
	return overview, nil
}
