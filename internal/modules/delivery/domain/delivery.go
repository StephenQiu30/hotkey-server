package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

type Channel string

const (
	ChannelEmail Channel = "email"
	ChannelRSS   Channel = "rss"
)

type DeliveryStatus string

const (
	DeliveryQueued    DeliveryStatus = "queued"
	DeliveryClaimed   DeliveryStatus = "claimed"
	DeliverySucceeded DeliveryStatus = "succeeded"
	DeliveryRetrying  DeliveryStatus = "retrying"
	DeliveryFailed    DeliveryStatus = "failed"
	DeliveryCancelled DeliveryStatus = "cancelled"
)

type Subscription struct {
	ID, Version, UserID                       int64
	ReportType, Recipient, Timezone, Schedule string
	Channel                                   Channel
	TokenHash                                 string
	Enabled                                   bool
}

func (subscription Subscription) Validate() error {
	if subscription.ID <= 0 || subscription.Version <= 0 || subscription.UserID <= 0 || (subscription.ReportType != "daily" && subscription.ReportType != "weekly") || strings.TrimSpace(subscription.Timezone) == "" || strings.TrimSpace(subscription.Schedule) == "" {
		return fmt.Errorf("invalid subscription")
	}
	if subscription.Channel == ChannelEmail && (strings.TrimSpace(subscription.Recipient) == "" || subscription.TokenHash != "") {
		return fmt.Errorf("invalid email subscription")
	}
	if subscription.Channel == ChannelRSS && (subscription.TokenHash == "" || subscription.Recipient != "") {
		return fmt.Errorf("invalid rss subscription")
	}
	if subscription.Channel != ChannelEmail && subscription.Channel != ChannelRSS {
		return fmt.Errorf("invalid delivery channel")
	}
	return nil
}

type Delivery struct {
	ID, ReportID, SubscriptionID int64
	IdempotencyKey               string
	Status                       DeliveryStatus
	NextAttemptAt                *time.Time
	SucceededAt                  *time.Time
}

func (delivery Delivery) Validate() error {
	if delivery.ID <= 0 || delivery.ReportID <= 0 || delivery.SubscriptionID <= 0 || strings.TrimSpace(delivery.IdempotencyKey) == "" {
		return fmt.Errorf("invalid delivery")
	}
	switch delivery.Status {
	case DeliveryQueued, DeliveryClaimed, DeliverySucceeded, DeliveryRetrying, DeliveryFailed, DeliveryCancelled:
	default:
		return fmt.Errorf("invalid delivery status")
	}
	return nil
}

func TokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func RetryableSMTP(code int) bool {
	return code == 0 || code == 421 || code == 450 || code == 451 || code == 452
}
