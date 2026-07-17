package application

import (
	"context"

	"github.com/StephenQiu30/hotkey-server/internal/modules/delivery/domain"
)

type SubscriptionStore interface {
	SaveSubscription(context.Context, domain.Subscription) error
	CreateDelivery(context.Context, domain.Delivery) (bool, error)
	UpdateDelivery(context.Context, domain.Delivery) error
	AppendAttempt(context.Context, int64, int, string, int, string) error
}
