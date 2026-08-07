package http

import (
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/notification/domain"
)

type NotificationResult[T any] struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}

type EmptyResponse struct{}

type NotificationPayloadResponse struct {
	Title    string `json:"title"`
	Summary  string `json:"summary,omitempty"`
	Status   string `json:"status,omitempty"`
	Severity string `json:"severity,omitempty"`
}

type NotificationResponse struct {
	ID           int64                       `json:"id"`
	EventType    string                      `json:"event_type"`
	ResourceType string                      `json:"resource_type"`
	ResourceID   int64                       `json:"resource_id"`
	Audience     string                      `json:"audience"`
	OccurredAt   time.Time                   `json:"occurred_at"`
	Payload      NotificationPayloadResponse `json:"payload"`
}

type NotificationPageResponse struct {
	Items       []NotificationResponse `json:"items"`
	NextAfterID int64                  `json:"next_after_id"`
}

func notificationResponse(event domain.NotificationEvent) NotificationResponse {
	return NotificationResponse{
		ID: event.ID, EventType: string(event.EventType), ResourceType: string(event.ResourceType),
		ResourceID: event.ResourceID, Audience: string(event.Audience), OccurredAt: event.OccurredAt,
		Payload: NotificationPayloadResponse{
			Title: event.Payload.Title, Summary: event.Payload.Summary,
			Status: event.Payload.Status, Severity: event.Payload.Severity,
		},
	}
}

func notificationPageResponse(page domain.NotificationPage) NotificationPageResponse {
	response := NotificationPageResponse{Items: make([]NotificationResponse, 0, len(page.Items)), NextAfterID: page.NextAfterID}
	for _, event := range page.Items {
		response.Items = append(response.Items, notificationResponse(event))
	}
	return response
}
