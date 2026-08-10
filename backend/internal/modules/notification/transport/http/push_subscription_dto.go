package http

import (
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/notification/application"
)

type PushCapabilityResponseDTO struct {
	Available      bool   `json:"available"`
	VAPIDPublicKey string `json:"vapid_public_key,omitempty"`
}

type PushSubscriptionKeysRequestDTO struct {
	P256DH string `json:"p256dh" binding:"required,max=256"`
	Auth   string `json:"auth" binding:"required,max=128"`
}

type RegisterPushSubscriptionRequestDTO struct {
	Endpoint    string                         `json:"endpoint" binding:"required,max=4096"`
	Keys        PushSubscriptionKeysRequestDTO `json:"keys" binding:"required"`
	DeviceLabel string                         `json:"device_label" binding:"required,max=80"`
	Timezone    string                         `json:"timezone" binding:"required,max=64"`
	QuietStart  *string                        `json:"quiet_start" extensions:"x-nullable"`
	QuietEnd    *string                        `json:"quiet_end" extensions:"x-nullable"`
	TTLSeconds  int                            `json:"ttl_seconds" binding:"required,min=60,max=86400" minimum:"60" maximum:"86400"`
	MonitorIDs  []int64                        `json:"monitor_ids" binding:"required,min=1,max=100,dive,gt=0"`
}

type UpdatePushSubscriptionRequestDTO struct {
	DeviceLabel string  `json:"device_label" binding:"required,max=80"`
	Timezone    string  `json:"timezone" binding:"required,max=64"`
	QuietStart  *string `json:"quiet_start" extensions:"x-nullable"`
	QuietEnd    *string `json:"quiet_end" extensions:"x-nullable"`
	TTLSeconds  int     `json:"ttl_seconds" binding:"required,min=60,max=86400" minimum:"60" maximum:"86400"`
	MonitorIDs  []int64 `json:"monitor_ids" binding:"required,min=1,max=100,dive,gt=0"`
}

type PushSubscriptionResponseDTO struct {
	ID               int64      `json:"id"`
	Version          int64      `json:"version"`
	DeviceLabel      string     `json:"device_label"`
	Timezone         string     `json:"timezone"`
	QuietStart       *string    `json:"quiet_start" extensions:"x-nullable"`
	QuietEnd         *string    `json:"quiet_end" extensions:"x-nullable"`
	TTLSeconds       int        `json:"ttl_seconds"`
	Status           string     `json:"status" enums:"active,disabled,expired"`
	ExpirationReason string     `json:"expiration_reason,omitempty"`
	MonitorIDs       []int64    `json:"monitor_ids"`
	LastSuccessAt    *time.Time `json:"last_success_at" extensions:"x-nullable"`
	LastFailureAt    *time.Time `json:"last_failure_at" extensions:"x-nullable"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

type PushSubscriptionListResponseDTO struct {
	Items []PushSubscriptionResponseDTO `json:"items"`
}

func pushCapabilityResponse(capability application.PushCapabilityDTO) PushCapabilityResponseDTO {
	return PushCapabilityResponseDTO{Available: capability.Available, VAPIDPublicKey: capability.VAPIDPublicKey}
}

func pushSubscriptionResponse(subscription application.PushSubscriptionDTO) PushSubscriptionResponseDTO {
	return PushSubscriptionResponseDTO{
		ID: subscription.ID, Version: subscription.Version, DeviceLabel: subscription.DeviceLabel,
		Timezone: subscription.Timezone, QuietStart: subscription.QuietStart, QuietEnd: subscription.QuietEnd,
		TTLSeconds: subscription.TTLSeconds, Status: subscription.Status, ExpirationReason: subscription.ExpirationReason,
		MonitorIDs: append([]int64(nil), subscription.MonitorIDs...), LastSuccessAt: subscription.LastSuccessAt,
		LastFailureAt: subscription.LastFailureAt, CreatedAt: subscription.CreatedAt, UpdatedAt: subscription.UpdatedAt,
	}
}

func pushSubscriptionListResponse(result application.ListPushSubscriptionsResult) PushSubscriptionListResponseDTO {
	response := PushSubscriptionListResponseDTO{Items: make([]PushSubscriptionResponseDTO, 0, len(result.Items))}
	for _, item := range result.Items {
		response.Items = append(response.Items, pushSubscriptionResponse(item))
	}
	return response
}
