package domain

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

const (
	DefaultUserNotificationLimit = 50
	MaximumUserNotificationLimit = 100
)

type UserNotificationEventType string

const (
	MicroEventCreated         UserNotificationEventType = "micro_event.created"
	MicroEventUpdated         UserNotificationEventType = "micro_event.updated"
	MicroEventReviewRequested UserNotificationEventType = "micro_event.review_requested"
	MicroEventEvidenceChanged UserNotificationEventType = "micro_event.evidence_changed"
)

func (eventType UserNotificationEventType) Valid() bool {
	switch eventType {
	case MicroEventCreated, MicroEventUpdated, MicroEventReviewRequested, MicroEventEvidenceChanged:
		return true
	default:
		return false
	}
}

var microEventDeepLink = regexp.MustCompile(`^/dashboard/events\?event=[1-9][0-9]{0,18}$`)
var deliveryTargetKey = regexp.MustCompile(`^[a-z][a-z0-9:_-]{0,127}$`)

type UserNotification struct {
	ID              int64
	Version         int64
	OutboxEventID   int64
	UserID          int64
	MonitorID       int64
	EventType       UserNotificationEventType
	ResourceType    string
	ResourceID      int64
	ResourceVersion int64
	OccurredAt      time.Time
	Title           string
	Summary         string
	ResourceStatus  string
	DeepLink        string
	CreatedAt       time.Time
}

func (notification UserNotification) Validate() error {
	if notification.ID <= 0 || notification.Version != 1 || notification.OutboxEventID <= 0 ||
		notification.UserID <= 0 || notification.MonitorID <= 0 || notification.ResourceID <= 0 ||
		notification.ResourceVersion <= 0 || notification.OccurredAt.IsZero() || notification.CreatedAt.IsZero() {
		return fmt.Errorf("user notification identity and version facts are required")
	}
	if !notification.EventType.Valid() || notification.ResourceType != "micro_event" {
		return fmt.Errorf("user notification event and resource type are invalid")
	}
	if strings.TrimSpace(notification.Title) == "" || len([]byte(notification.Title)) > 240 ||
		len([]byte(notification.Summary)) > 1000 || strings.TrimSpace(notification.ResourceStatus) == "" ||
		len([]byte(notification.ResourceStatus)) > 32 {
		return fmt.Errorf("user notification safe projection is invalid")
	}
	if len([]byte(notification.DeepLink)) > 256 || !microEventDeepLink.MatchString(notification.DeepLink) {
		return fmt.Errorf("user notification deep link is invalid")
	}
	return nil
}

type UserNotificationQuery struct {
	UserID    int64
	MonitorID *int64
	AfterID   int64
	Limit     int
}

func (query UserNotificationQuery) Normalized() UserNotificationQuery {
	if query.Limit == 0 {
		query.Limit = DefaultUserNotificationLimit
	}
	return query
}

func (query UserNotificationQuery) Validate() error {
	if query.UserID <= 0 || query.AfterID < 0 || query.Limit <= 0 || query.Limit > MaximumUserNotificationLimit {
		return fmt.Errorf("invalid user notification query")
	}
	if query.MonitorID != nil && *query.MonitorID <= 0 {
		return fmt.Errorf("invalid notification monitor filter")
	}
	return nil
}

type DeliveryChannel string

const (
	DeliveryChannelSSE       DeliveryChannel = "sse"
	DeliveryChannelWebSocket DeliveryChannel = "websocket"
	DeliveryChannelEmail     DeliveryChannel = "email"
	DeliveryChannelWebPush   DeliveryChannel = "web_push"
)

func (channel DeliveryChannel) Valid() bool {
	return channel == DeliveryChannelSSE || channel == DeliveryChannelWebSocket || channel == DeliveryChannelEmail || channel == DeliveryChannelWebPush
}

type DeliveryStatus string

const (
	DeliverySucceeded        DeliveryStatus = "succeeded"
	DeliveryFailed           DeliveryStatus = "failed"
	DeliveryPermanentFailure DeliveryStatus = "permanent_failure"
)

func (status DeliveryStatus) Valid() bool {
	return status == DeliverySucceeded || status == DeliveryFailed || status == DeliveryPermanentFailure
}

type NotificationDeliveryAttempt struct {
	UserNotificationID int64
	Channel            DeliveryChannel
	DeliveryTargetKey  string
	Status             DeliveryStatus
	ProviderMessageID  string
	ResponseCode       *int
	ErrorCode          string
	AttemptedAt        time.Time
}

func (attempt NotificationDeliveryAttempt) Validate() error {
	if attempt.UserNotificationID <= 0 || !attempt.Channel.Valid() || !deliveryTargetKey.MatchString(attempt.DeliveryTargetKey) ||
		!attempt.Status.Valid() || attempt.AttemptedAt.IsZero() {
		return fmt.Errorf("notification delivery attempt identity is invalid")
	}
	if len([]byte(attempt.ProviderMessageID)) > 256 || len([]byte(attempt.ErrorCode)) > 64 {
		return fmt.Errorf("notification delivery attempt projection is too long")
	}
	if attempt.ResponseCode != nil && (*attempt.ResponseCode < 100 || *attempt.ResponseCode > 599) {
		return fmt.Errorf("notification delivery response code is invalid")
	}
	if attempt.Status == DeliverySucceeded && attempt.ErrorCode != "" || attempt.Status != DeliverySucceeded && strings.TrimSpace(attempt.ErrorCode) == "" {
		return fmt.Errorf("notification delivery status and error code disagree")
	}
	return nil
}
