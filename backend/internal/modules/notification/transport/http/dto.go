package http

import (
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/notification/application"
)

type NotificationResult[T any] struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}

type EmptyResponseDTO struct{}

type UserNotificationResponseDTO struct {
	ID              int64     `json:"id"`
	Version         int64     `json:"version"`
	MonitorID       int64     `json:"monitor_id"`
	EventType       string    `json:"event_type"`
	ResourceType    string    `json:"resource_type"`
	ResourceID      int64     `json:"resource_id"`
	ResourceVersion int64     `json:"resource_version"`
	OccurredAt      time.Time `json:"occurred_at"`
	Title           string    `json:"title"`
	Summary         string    `json:"summary,omitempty"`
	ResourceStatus  string    `json:"resource_status"`
	DeepLink        string    `json:"deep_link"`
	CreatedAt       time.Time `json:"created_at"`
}

type UserNotificationPageResponseDTO struct {
	Items       []UserNotificationResponseDTO `json:"items"`
	NextAfterID int64                         `json:"next_after_id"`
}

func userNotificationResponse(item application.UserNotificationDTO) UserNotificationResponseDTO {
	return UserNotificationResponseDTO{
		ID: item.ID, Version: item.Version, MonitorID: item.MonitorID, EventType: item.EventType,
		ResourceType: item.ResourceType, ResourceID: item.ResourceID, ResourceVersion: item.ResourceVersion,
		OccurredAt: item.OccurredAt, Title: item.Title, Summary: item.Summary,
		ResourceStatus: item.ResourceStatus, DeepLink: item.DeepLink, CreatedAt: item.CreatedAt,
	}
}

func userNotificationPageResponse(page application.ListUserNotificationsResult) UserNotificationPageResponseDTO {
	response := UserNotificationPageResponseDTO{
		Items: make([]UserNotificationResponseDTO, 0, len(page.Items)), NextAfterID: page.NextAfterID,
	}
	for _, item := range page.Items {
		response.Items = append(response.Items, userNotificationResponse(item))
	}
	return response
}
