package http

import (
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/alert/domain"
)

// AlertResult mirrors the shared Result envelope only for swag's source
// parser. Runtime output always uses platform HTTP helpers.
type AlertResult[T any] struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}

type EmptyResponse struct{}

type AlertActionRequest struct {
	ExpectedVersion int64  `json:"expected_version" binding:"required,gt=0" minimum:"1"`
	ReasonCode      string `json:"reason_code" binding:"required,max=64" maxLength:"64"`
}

type AlertThreadResponse struct {
	ID                   int64      `json:"id"`
	Version              int64      `json:"version"`
	MonitorID            int64      `json:"monitor_id"`
	EventID              int64      `json:"event_id"`
	TriggerType          string     `json:"trigger_type"`
	PolicyVersion        string     `json:"policy_version"`
	MonitorRevision      int64      `json:"monitor_revision"`
	Threshold            float64    `json:"threshold"`
	MinHeat              float64    `json:"min_heat"`
	MinMomentum          float64    `json:"min_momentum"`
	MinBreadth           float64    `json:"min_breadth"`
	WarningThreshold     float64    `json:"warning_threshold"`
	CriticalThreshold    float64    `json:"critical_threshold"`
	CooldownMinutes      int        `json:"cooldown_minutes"`
	State                string     `json:"state"`
	Severity             string     `json:"severity"`
	Title                string     `json:"title"`
	Reason               string     `json:"reason"`
	FirstTriggeredAt     time.Time  `json:"first_triggered_at"`
	LastTriggeredAt      time.Time  `json:"last_triggered_at"`
	OccurrenceCount      int64      `json:"occurrence_count"`
	CooldownUntil        time.Time  `json:"cooldown_until"`
	AcknowledgedAt       *time.Time `json:"acknowledged_at,omitempty"`
	AcknowledgedByUserID *int64     `json:"acknowledged_by_user_id,omitempty"`
	ResolvedAt           *time.Time `json:"resolved_at,omitempty"`
	ResolvedByUserID     *int64     `json:"resolved_by_user_id,omitempty"`
	SuppressedAt         *time.Time `json:"suppressed_at,omitempty"`
	SuppressedByUserID   *int64     `json:"suppressed_by_user_id,omitempty"`
}

type AlertOccurrenceResponse struct {
	ID            int64     `json:"id"`
	EventUpdateID int64     `json:"event_update_id"`
	Severity      string    `json:"severity"`
	FinalScore    float64   `json:"final_score"`
	Threshold     float64   `json:"threshold"`
	HeatScore     float64   `json:"heat_score"`
	MomentumScore float64   `json:"momentum_score"`
	BreadthScore  float64   `json:"breadth_score"`
	ReasonCodes   []string  `json:"reason_codes"`
	TriggeredAt   time.Time `json:"triggered_at"`
}

type AlertStateAuditResponse struct {
	ID              int64     `json:"id"`
	ActorType       string    `json:"actor_type"`
	ActorUserID     *int64    `json:"actor_user_id,omitempty"`
	FromState       string    `json:"from_state"`
	ToState         string    `json:"to_state"`
	ExpectedVersion int64     `json:"expected_version"`
	ReasonCode      string    `json:"reason_code"`
	CreatedAt       time.Time `json:"created_at"`
}

type AlertPageResponse struct {
	Items      []AlertThreadResponse `json:"items"`
	NextCursor string                `json:"next_cursor,omitempty"`
}

type AlertDetailResponse struct {
	Thread          AlertThreadResponse          `json:"thread"`
	Occurrences     []AlertOccurrenceResponse    `json:"occurrences"`
	Audits          []AlertStateAuditResponse    `json:"audits"`
	EmailDeliveries []AlertEmailDeliveryResponse `json:"email_deliveries"`
}

type AlertEmailDeliveryResponse struct {
	ID            int64      `json:"id"`
	OccurrenceID  int64      `json:"occurrence_id"`
	Severity      string     `json:"severity"`
	Status        string     `json:"status"`
	AttemptCount  int        `json:"attempt_count"`
	NextAttemptAt *time.Time `json:"next_attempt_at,omitempty"`
	SucceededAt   *time.Time `json:"succeeded_at,omitempty"`
	LastError     string     `json:"last_error,omitempty"`
}

func threadResponse(thread domain.Thread) AlertThreadResponse {
	return AlertThreadResponse{
		ID: thread.ID, Version: thread.Version, MonitorID: thread.MonitorID, EventID: thread.EventID,
		TriggerType: string(thread.TriggerType), PolicyVersion: thread.PolicyVersion,
		MonitorRevision: thread.MonitorRevision, Threshold: thread.EventThresholdSnapshot,
		MinHeat: thread.AlertMinHeatSnapshot, MinMomentum: thread.AlertMinMomentumSnapshot, MinBreadth: thread.AlertMinBreadthSnapshot,
		WarningThreshold: thread.AlertWarningThresholdSnapshot, CriticalThreshold: thread.AlertCriticalThresholdSnapshot, CooldownMinutes: thread.AlertCooldownMinutesSnapshot,
		State: string(thread.State), Severity: string(thread.Severity),
		Title: thread.TitleSnapshot, Reason: thread.ReasonSnapshot, FirstTriggeredAt: thread.FirstTriggeredAt, LastTriggeredAt: thread.LastTriggeredAt,
		OccurrenceCount: thread.OccurrenceCount, CooldownUntil: thread.CooldownUntil,
		AcknowledgedAt: thread.AcknowledgedAt, AcknowledgedByUserID: thread.AcknowledgedByUserID,
		ResolvedAt: thread.ResolvedAt, ResolvedByUserID: thread.ResolvedByUserID,
		SuppressedAt: thread.SuppressedAt, SuppressedByUserID: thread.SuppressedByUserID,
	}
}

func pageResponse(page domain.ThreadPage) AlertPageResponse {
	response := AlertPageResponse{Items: make([]AlertThreadResponse, 0, len(page.Items)), NextCursor: page.NextCursor}
	for _, thread := range page.Items {
		response.Items = append(response.Items, threadResponse(thread))
	}
	return response
}

func detailResponse(detail domain.ThreadDetail) AlertDetailResponse {
	response := AlertDetailResponse{
		Thread: threadResponse(detail.Thread), Occurrences: make([]AlertOccurrenceResponse, 0, len(detail.Occurrences)), Audits: make([]AlertStateAuditResponse, 0, len(detail.Audits)), EmailDeliveries: make([]AlertEmailDeliveryResponse, 0, len(detail.EmailDeliveries)),
	}
	for _, occurrence := range detail.Occurrences {
		response.Occurrences = append(response.Occurrences, AlertOccurrenceResponse{
			ID: occurrence.ID, EventUpdateID: occurrence.EventUpdateID, Severity: string(occurrence.Severity),
			FinalScore: occurrence.FinalScoreSnapshot, Threshold: occurrence.EventThresholdSnapshot,
			HeatScore: occurrence.HeatScoreSnapshot, MomentumScore: occurrence.MomentumScoreSnapshot, BreadthScore: occurrence.BreadthScoreSnapshot,
			ReasonCodes: append([]string(nil), occurrence.ReasonCodes...), TriggeredAt: occurrence.TriggeredAt,
		})
	}
	for _, delivery := range detail.EmailDeliveries {
		response.EmailDeliveries = append(response.EmailDeliveries, AlertEmailDeliveryResponse{ID: delivery.ID, OccurrenceID: delivery.OccurrenceID, Severity: string(delivery.Severity), Status: delivery.Status, AttemptCount: delivery.AttemptCount, NextAttemptAt: delivery.NextAttemptAt, SucceededAt: delivery.SucceededAt, LastError: delivery.LastError})
	}
	for _, audit := range detail.Audits {
		response.Audits = append(response.Audits, AlertStateAuditResponse{
			ID: audit.ID, ActorType: string(audit.ActorType), ActorUserID: audit.ActorUserID,
			FromState: string(audit.FromState), ToState: string(audit.ToState), ExpectedVersion: audit.ExpectedVersion,
			ReasonCode: audit.ReasonCode, CreatedAt: audit.CreatedAt,
		})
	}
	return response
}
