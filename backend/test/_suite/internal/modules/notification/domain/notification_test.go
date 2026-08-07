package domain

import (
	"strings"
	"testing"
	"time"
)

func TestNotificationEventValidatesSafePersistentProjection(t *testing.T) {
	now := time.Date(2026, 8, 8, 2, 0, 0, 0, time.UTC)
	event := NotificationEvent{
		ID: 1, EventType: EventUpdated, ResourceType: ResourceEvent,
		ResourceID: 9, Audience: AudienceViewer, OccurredAt: now,
		Payload: NotificationPayload{Title: "事件更新", Summary: "出现新的独立来源", Status: "rising"},
	}
	if err := event.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	event.Payload.Summary = strings.Repeat("x", 2001)
	if err := event.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want oversized summary rejection")
	}
}

func TestAudienceRoleHierarchyDoesNotExposeAdminDiagnostics(t *testing.T) {
	tests := []struct {
		viewer, audience AudienceRole
		want             bool
	}{
		{AudienceViewer, AudienceViewer, true},
		{AudienceViewer, AudienceEditor, false},
		{AudienceViewer, AudienceAdmin, false},
		{AudienceEditor, AudienceViewer, true},
		{AudienceEditor, AudienceEditor, true},
		{AudienceEditor, AudienceAdmin, false},
		{AudienceAdmin, AudienceViewer, true},
		{AudienceAdmin, AudienceEditor, true},
		{AudienceAdmin, AudienceAdmin, true},
	}
	for _, test := range tests {
		if got := test.viewer.Allows(test.audience); got != test.want {
			t.Fatalf("%q.Allows(%q) = %t, want %t", test.viewer, test.audience, got, test.want)
		}
	}
}

func TestNotificationQueryNormalizesAndRejectsInvalidBounds(t *testing.T) {
	query := (NotificationQuery{Role: AudienceViewer}).Normalized()
	if query.Limit != DefaultListLimit {
		t.Fatalf("Normalized().Limit = %d, want %d", query.Limit, DefaultListLimit)
	}
	if err := query.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	for _, invalid := range []NotificationQuery{
		{Role: "owner", Limit: 10},
		{Role: AudienceViewer, AfterID: -1, Limit: 10},
		{Role: AudienceViewer, Limit: MaximumListLimit + 1},
	} {
		if err := invalid.Validate(); err == nil {
			t.Fatalf("Validate(%#v) error = nil", invalid)
		}
	}
}
