// Package domain owns Delivery entities, value objects, and rules.
package domain

import (
	"fmt"
	"net/mail"
	"strings"
	"time"
)

type AlertDelivery struct {
	ID             int64
	OccurrenceID   int64
	IdempotencyKey string
	Recipient      string
	Subject        string
	TextBody       string
	HTMLBody       string
	Severity       string
	Status         DeliveryStatus
	NextAttemptAt  *time.Time
	SucceededAt    *time.Time
	LastError      string
	AttemptCount   int
}

func (delivery AlertDelivery) ValidateForCreate() error {
	parsed, err := mail.ParseAddress(strings.TrimSpace(delivery.Recipient))
	if delivery.OccurrenceID <= 0 || len(delivery.IdempotencyKey) != 64 || err != nil || parsed.Name != "" || parsed.Address != delivery.Recipient || strings.TrimSpace(delivery.Subject) == "" || strings.TrimSpace(delivery.TextBody) == "" || strings.TrimSpace(delivery.HTMLBody) == "" || delivery.Severity != "warning" && delivery.Severity != "critical" || delivery.Status != DeliveryQueued {
		return fmt.Errorf("invalid alert email delivery")
	}
	return nil
}

func (delivery AlertDelivery) Validate() error {
	copy := delivery
	copy.ID, copy.Status = 1, DeliveryQueued
	if err := copy.ValidateForCreate(); err != nil {
		return err
	}
	if delivery.ID <= 0 {
		return fmt.Errorf("invalid alert email delivery identity")
	}
	switch delivery.Status {
	case DeliveryQueued, DeliveryClaimed, DeliverySucceeded, DeliveryRetrying, DeliveryFailed:
		return nil
	default:
		return fmt.Errorf("invalid alert email delivery status")
	}
}
