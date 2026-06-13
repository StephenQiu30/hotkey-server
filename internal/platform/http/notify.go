package http

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/StephenQiu30/hotkey-server/internal/notify"
)

// RegisterNotifyRoutes registers the notification endpoints.
func RegisterNotifyRoutes(api huma.API, svc *notify.Service) {
	huma.Register(api, huma.Operation{
		OperationID: "list-notifications",
		Method:      http.MethodGet,
		Path:        "/api/v1/notifications",
		Summary:     "List unread notifications",
		Description: "Returns all unread notifications for the authenticated user.",
		Tags:        []string{"notifications"},
		Security:    []map[string][]string{{"bearer": {}}},
		Errors:      []int{401, 500},
	}, func(ctx context.Context, input *struct{}) (*ListNotificationsOutput, error) {
		userID, ok := userIDFromCtx(ctx)
		if !ok {
			return nil, huma.Error401Unauthorized("unauthorized")
		}

		items, err := svc.ListUnread(ctx, userID)
		if err != nil {
			return nil, huma.Error500InternalServerError(err.Error())
		}

		result := make([]NotificationResponse, len(items))
		for i, n := range items {
			result[i] = toNotificationResponse(n)
		}

		return &ListNotificationsOutput{Body: result}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "mark-notification-read",
		Method:      http.MethodPost,
		Path:        "/api/v1/notifications/{id}/read",
		Summary:     "Mark notification as read",
		Description: "Marks a specific notification as read for the authenticated user.",
		Tags:        []string{"notifications"},
		Security:    []map[string][]string{{"bearer": {}}},
		Errors:      []int{401, 404, 500},
	}, func(ctx context.Context, input *MarkReadInput) (*struct{}, error) {
		userID, ok := userIDFromCtx(ctx)
		if !ok {
			return nil, huma.Error401Unauthorized("unauthorized")
		}

		if err := svc.MarkRead(ctx, userID, input.ID); err != nil {
			if err == notify.ErrNotFound || err == notify.ErrNotOwned {
				return nil, huma.Error404NotFound(err.Error())
			}
			return nil, huma.Error500InternalServerError(err.Error())
		}

		return nil, nil
	})
}

type MarkReadInput struct {
	ID int64 `path:"id" validate:"required" doc:"Notification ID"`
}

type ListNotificationsOutput struct {
	Body []NotificationResponse
}

type NotificationResponse struct {
	ID             int64   `json:"id" doc:"Notification ID"`
	UserID         int64   `json:"user_id" doc:"User ID"`
	AlertID        int64   `json:"alert_id" doc:"Alert ID"`
	Channel        string  `json:"channel" doc:"Notification channel"`
	DeliveryStatus string  `json:"delivery_status" doc:"Delivery status"`
	ReadAt         *string `json:"read_at,omitempty" doc:"Read timestamp (ISO 8601)"`
	CreatedAt      string  `json:"created_at" doc:"Creation timestamp (ISO 8601)"`
}

func toNotificationResponse(n notify.Notification) NotificationResponse {
	r := NotificationResponse{
		ID: n.ID, UserID: n.UserID, AlertID: n.AlertID,
		Channel: n.Channel, DeliveryStatus: n.DeliveryStatus,
		CreatedAt: n.CreatedAt.Format(time.RFC3339),
	}
	if n.ReadAt != nil {
		s := n.ReadAt.Format(time.RFC3339)
		r.ReadAt = &s
	}
	return r
}
