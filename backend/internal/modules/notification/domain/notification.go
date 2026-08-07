package domain

import (
	"fmt"
	"strings"
	"time"
)

const (
	DefaultListLimit = 50
	MaximumListLimit = 100
)

type AudienceRole string

const (
	AudienceViewer AudienceRole = "viewer"
	AudienceEditor AudienceRole = "editor"
	AudienceAdmin  AudienceRole = "admin"
)

func (role AudienceRole) Valid() bool {
	return role == AudienceViewer || role == AudienceEditor || role == AudienceAdmin
}

func (role AudienceRole) Allows(audience AudienceRole) bool {
	rank := map[AudienceRole]int{AudienceViewer: 1, AudienceEditor: 2, AudienceAdmin: 3}
	viewerRank, viewerOK := rank[role]
	audienceRank, audienceOK := rank[audience]
	return viewerOK && audienceOK && viewerRank >= audienceRank
}

type EventType string

const (
	EventUpdated        EventType = "event.updated"
	AlertTriggered      EventType = "alert.triggered"
	ReportPublished     EventType = "report.published"
	ReportFailed        EventType = "report.failed"
	CollectionSucceeded EventType = "collection.succeeded"
	CollectionFailed    EventType = "collection.failed"
)

type ResourceType string

const (
	ResourceEvent         ResourceType = "event"
	ResourceAlert         ResourceType = "alert"
	ResourceReport        ResourceType = "report"
	ResourceCollectionRun ResourceType = "collection_run"
)

type NotificationPayload struct {
	Title    string `json:"title"`
	Summary  string `json:"summary,omitempty"`
	Status   string `json:"status,omitempty"`
	Severity string `json:"severity,omitempty"`
}

func (payload NotificationPayload) Validate() error {
	if strings.TrimSpace(payload.Title) == "" || len([]byte(payload.Title)) > 1000 {
		return fmt.Errorf("notification title must contain 1-1000 bytes")
	}
	if len([]byte(payload.Summary)) > 2000 {
		return fmt.Errorf("notification summary must not exceed 2000 bytes")
	}
	if len([]byte(payload.Status)) > 64 || len([]byte(payload.Severity)) > 16 {
		return fmt.Errorf("notification status or severity is too long")
	}
	return nil
}

type NotificationEvent struct {
	ID           int64
	EventType    EventType
	ResourceType ResourceType
	ResourceID   int64
	Audience     AudienceRole
	OccurredAt   time.Time
	Payload      NotificationPayload
}

func (event NotificationEvent) Validate() error {
	if event.ID <= 0 || event.ResourceID <= 0 || event.OccurredAt.IsZero() {
		return fmt.Errorf("notification identity, resource and occurrence time are required")
	}
	if !event.Audience.Valid() {
		return fmt.Errorf("notification audience is invalid")
	}
	wantedResource := map[EventType]ResourceType{
		EventUpdated: ResourceEvent, AlertTriggered: ResourceAlert,
		ReportPublished: ResourceReport, ReportFailed: ResourceReport,
		CollectionSucceeded: ResourceCollectionRun, CollectionFailed: ResourceCollectionRun,
	}[event.EventType]
	if wantedResource == "" || event.ResourceType != wantedResource {
		return fmt.Errorf("notification type and resource do not match")
	}
	return event.Payload.Validate()
}

type NotificationQuery struct {
	Role    AudienceRole
	AfterID int64
	Limit   int
}

func (query NotificationQuery) Normalized() NotificationQuery {
	if query.Limit == 0 {
		query.Limit = DefaultListLimit
	}
	return query
}

func (query NotificationQuery) Validate() error {
	if !query.Role.Valid() || query.AfterID < 0 || query.Limit <= 0 || query.Limit > MaximumListLimit {
		return fmt.Errorf("invalid notification query")
	}
	return nil
}

type NotificationPage struct {
	Items       []NotificationEvent
	NextAfterID int64
}
