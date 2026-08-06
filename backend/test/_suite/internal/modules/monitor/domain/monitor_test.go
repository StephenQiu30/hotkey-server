package domain

import (
	"testing"
)

func TestMonitorStateTransitionsFollowPublishedStateMachine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		from MonitorStatus
		to   MonitorStatus
		want bool
	}{
		{MonitorStatusDraft, MonitorStatusActive, true},
		{MonitorStatusActive, MonitorStatusPaused, true},
		{MonitorStatusPaused, MonitorStatusActive, true},
		{MonitorStatusDraft, MonitorStatusArchived, true},
		{MonitorStatusActive, MonitorStatusArchived, true},
		{MonitorStatusPaused, MonitorStatusArchived, true},
		{MonitorStatusArchived, MonitorStatusPaused, true},
		{MonitorStatusDraft, MonitorStatusPaused, false},
		{MonitorStatusArchived, MonitorStatusActive, false},
	}
	for _, tt := range tests {
		if got := CanTransition(tt.from, tt.to); got != tt.want {
			t.Errorf("CanTransition(%q, %q) = %v, want %v", tt.from, tt.to, got, tt.want)
		}
	}
}

func TestExpectedDraftVersionDistinguishesExistingAndFirstDraft(t *testing.T) {
	t.Parallel()

	version := int64(8)
	if err := (ExpectedVersions{MonitorVersion: 4, DraftVersion: &version}).ValidateDraft(true); err != nil {
		t.Fatalf("existing draft expected versions: %v", err)
	}
	if err := (ExpectedVersions{MonitorVersion: 4}).ValidateDraft(false); err != nil {
		t.Fatalf("first draft expected versions: %v", err)
	}
	if err := (ExpectedVersions{MonitorVersion: 4}).ValidateDraft(true); err == nil {
		t.Fatal("existing draft with null draft version = nil error, want rejection")
	}
	if err := (ExpectedVersions{MonitorVersion: 4, DraftVersion: &version}).ValidateDraft(false); err == nil {
		t.Fatal("first draft with non-null draft version = nil error, want rejection")
	}
}

func TestNormalizeMonitorConfigValidatesAndCanonicalizesInput(t *testing.T) {
	t.Parallel()

	config, err := NormalizeMonitorConfig(MonitorConfig{
		Timezone:                  "Asia/Shanghai",
		Languages:                 []string{"zh-cn", "en"},
		Regions:                   []string{"cn", "US"},
		CollectionIntervalSeconds: 600,
		RelevanceThreshold:        60,
		EventThreshold:            25,
		RetentionDays:             30,
	})
	if err != nil {
		t.Fatalf("NormalizeMonitorConfig() error = %v", err)
	}
	if got, want := config.Languages[0], "en"; got != want {
		t.Errorf("first canonical language = %q, want %q", got, want)
	}
	if got, want := config.Regions[0], "CN"; got != want {
		t.Errorf("first canonical region = %q, want %q", got, want)
	}
	bad := config
	bad.CollectionIntervalSeconds = 301
	if _, err := NormalizeMonitorConfig(bad); err == nil {
		t.Fatal("invalid collection interval = nil error, want rejection")
	}
}

func TestCanonicalConfigHashIsStableAcrossInputOrdering(t *testing.T) {
	t.Parallel()

	config := MonitorConfig{
		Timezone: "UTC", Languages: []string{"zh-cn", "en"}, Regions: []string{"us", "cn"},
		CollectionIntervalSeconds: 600, RelevanceThreshold: 60, EventThreshold: 10, RetentionDays: 30,
	}
	first, err := CanonicalConfigHash(ConfigHashInput{
		MonitorID: 7, Revision: 2, Config: config,
		Rules: []MonitorRule{
			{ID: 2, RuleType: RuleTypeKeyword, Operator: RuleOperatorContains, Value: "beta", Weight: 10, Priority: 2, Origin: RuleOriginUser, ApprovalStatus: RuleApprovalApproved, Enabled: true},
			{ID: 1, RuleType: RuleTypeKeyword, Operator: RuleOperatorContains, Value: "alpha", Weight: 10, Priority: 1, Origin: RuleOriginAI, ApprovalStatus: RuleApprovalPending, Enabled: true},
		},
		Sources: []MonitorSource{{ID: 4, SourceConnectionID: 9, QueryOverride: "  café ", Priority: 2, Enabled: true}, {ID: 3, SourceConnectionID: 8, Priority: 1, Enabled: true}},
	})
	if err != nil {
		t.Fatalf("first CanonicalConfigHash() error = %v", err)
	}
	second, err := CanonicalConfigHash(ConfigHashInput{
		MonitorID: 7, Revision: 2, Config: MonitorConfig{Timezone: "UTC", Languages: []string{"en", "zh-CN"}, Regions: []string{"CN", "US"}, CollectionIntervalSeconds: 600, RelevanceThreshold: 60, EventThreshold: 10, RetentionDays: 30},
		Rules: []MonitorRule{
			{ID: 1, RuleType: RuleTypeKeyword, Operator: RuleOperatorContains, Value: "alpha", Weight: 10, Priority: 1, Origin: RuleOriginAI, ApprovalStatus: RuleApprovalPending, Enabled: true},
			{ID: 2, RuleType: RuleTypeKeyword, Operator: RuleOperatorContains, Value: "beta", Weight: 10, Priority: 2, Origin: RuleOriginUser, ApprovalStatus: RuleApprovalApproved, Enabled: true},
		},
		Sources: []MonitorSource{{ID: 3, SourceConnectionID: 8, Priority: 1, Enabled: true}, {ID: 4, SourceConnectionID: 9, QueryOverride: "cafe\u0301", Priority: 2, Enabled: true}},
	})
	if err != nil {
		t.Fatalf("second CanonicalConfigHash() error = %v", err)
	}
	if first != second {
		t.Errorf("canonical hashes differ for equivalent input: %s != %s", first, second)
	}
}
