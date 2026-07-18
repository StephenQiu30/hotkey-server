package http

import (
	deliveryapplication "github.com/StephenQiu30/hotkey-server/internal/modules/delivery/application"
	"github.com/StephenQiu30/hotkey-server/internal/modules/delivery/domain"
)

// DeliveryResult mirrors the shared Result envelope for Swagger only.
type DeliveryResult[T any] struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}

type DeliveryEmptyResponse struct{}

type CreateSubscriptionRequest struct {
	MonitorID  *int64 `json:"monitor_id,omitempty"`
	ReportType string `json:"report_type,omitempty" binding:"omitempty,oneof=daily weekly"`
	Channel    string `json:"channel,omitempty" binding:"omitempty,oneof=email rss"`
	Recipient  string `json:"recipient,omitempty"`
	Timezone   string `json:"timezone,omitempty"`
	Schedule   string `json:"schedule,omitempty"`
	Enabled    *bool  `json:"enabled,omitempty"`
}

type UpdateSubscriptionRequest struct {
	ExpectedVersion int64   `json:"expected_version" binding:"required,gt=0"`
	Recipient       *string `json:"recipient,omitempty"`
	Timezone        *string `json:"timezone,omitempty"`
	Schedule        *string `json:"schedule,omitempty"`
	Enabled         *bool   `json:"enabled,omitempty"`
}

type RotateRSSTokenRequest struct {
	ExpectedVersion int64 `json:"expected_version" binding:"required,gt=0"`
}

type DeleteSubscriptionRequest struct {
	ExpectedVersion int64 `json:"expected_version" binding:"required,gt=0"`
}

// SubscriptionResponse deliberately excludes TokenHash. Token hashes are
// implementation facts and must not become a public correlation surface.
type SubscriptionResponse struct {
	ID         int64  `json:"id"`
	Version    int64  `json:"version"`
	MonitorID  *int64 `json:"monitor_id,omitempty"`
	ReportType string `json:"report_type"`
	Channel    string `json:"channel"`
	Recipient  string `json:"recipient,omitempty"`
	Timezone   string `json:"timezone"`
	Schedule   string `json:"schedule"`
	Enabled    bool   `json:"enabled"`
}

type SubscriptionPageResponse struct {
	Items      []SubscriptionResponse `json:"items"`
	NextCursor string                 `json:"next_cursor,omitempty"`
}

// SubscriptionSecretResponse returns the opaque RSS token exactly once. It
// is never present in ordinary list/detail/update responses.
type SubscriptionSecretResponse struct {
	Subscription SubscriptionResponse `json:"subscription"`
	RSSToken     string               `json:"rss_token,omitempty"`
}

func subscriptionResponse(subscription domain.Subscription) SubscriptionResponse {
	return SubscriptionResponse{ID: subscription.ID, Version: subscription.Version, MonitorID: subscription.MonitorID, ReportType: subscription.ReportType, Channel: string(subscription.Channel), Recipient: subscription.Recipient, Timezone: subscription.Timezone, Schedule: subscription.Schedule, Enabled: subscription.Enabled}
}

func subscriptionSecretResponse(result deliveryapplication.SubscriptionSecret) SubscriptionSecretResponse {
	return SubscriptionSecretResponse{Subscription: subscriptionResponse(result.Subscription), RSSToken: result.RSSToken}
}
