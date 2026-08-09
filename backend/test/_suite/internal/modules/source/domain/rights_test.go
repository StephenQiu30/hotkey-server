package domain

import (
	"strings"
	"testing"
	"time"
)

func TestRightsActionVocabularyIsFrozen(t *testing.T) {
	t.Parallel()
	if RightsAction("embed_or_tdm").Valid() || RightsAction("external_model_disclosure").Valid() {
		t.Fatal("legacy vocabulary escaped as a persisted action value")
	}
}

func TestRightsPolicyIsAnImmutableSemanticEntity(t *testing.T) {
	t.Parallel()
	effective := time.Date(2026, time.August, 9, 3, 0, 0, 0, time.UTC)
	sourceID := int64(42)
	policy := RightsPolicy{
		ID: 3, Version: 1, SourceConnectionID: &sourceID,
		Scope:    RightsScope{Type: RightsScopeSourceEndpoint, SubjectID: "source-42"},
		Revision: 2, Priority: RightsPriorityEndpointContract,
		Basis: RightsBasis{Summary: "reviewed policy"}, PolicyHash: strings.Repeat("a", 64),
		EffectiveFrom: effective,
	}
	if err := policy.Validate(); err != nil {
		t.Fatalf("valid policy rejected: %v", err)
	}
	policy.SourceConnectionID = nil
	if err := policy.Validate(); err == nil {
		t.Fatal("source-scoped policy without source was accepted")
	}
}

func TestSingleActionRightsDecisionValidatesExactSubjectPolicyAndLifetime(t *testing.T) {
	t.Parallel()
	evaluatedAt := time.Date(2026, time.August, 9, 4, 0, 0, 0, time.UTC)
	expiresAt := evaluatedAt.Add(24 * time.Hour)
	valid := testRightsDecision(RightsScope{Type: RightsScopeSourceEndpoint, SubjectID: "source-42"}, RightsPriorityEndpointContract)
	valid.ExpiresAt = &expiresAt
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid decision rejected: %v", err)
	}
	if !valid.Allows(evaluatedAt.Add(time.Hour)) {
		t.Fatal("valid unexpired single-action allow was denied")
	}
	if valid.Allows(expiresAt) || valid.Allows(evaluatedAt.Add(-time.Nanosecond)) {
		t.Fatal("decision authorized outside its effective lifetime")
	}

	tests := []struct {
		name   string
		mutate func(*RightsDecision)
	}{
		{"missing entity id", func(decision *RightsDecision) { decision.ID = 0 }},
		{"missing source", func(decision *RightsDecision) { decision.SourceConnectionID = 0 }},
		{"missing scope subject", func(decision *RightsDecision) { decision.Scope.SubjectID = "" }},
		{"organization scope with subject", func(decision *RightsDecision) {
			decision.Scope = RightsScope{Type: RightsScopeOrganizationDefault, SubjectID: "must-be-empty"}
			decision.Priority = RightsPriorityOrganizationDefault
		}},
		{"unknown scope", func(decision *RightsDecision) { decision.Scope.Type = RightsScopeType("global") }},
		{"missing policy", func(decision *RightsDecision) { decision.PolicyID = 0 }},
		{"invalid policy revision", func(decision *RightsDecision) { decision.PolicyRevision = 0 }},
		{"invalid priority", func(decision *RightsDecision) { decision.Priority = RightsPriority(999) }},
		{"priority incompatible with scope", func(decision *RightsDecision) { decision.Priority = RightsPriorityOrganizationDefault }},
		{"missing basis", func(decision *RightsDecision) { decision.Basis.Summary = "" }},
		{"unsafe terms URI", func(decision *RightsDecision) {
			decision.Basis.TermsURL = "https://user:secret@publisher.example/terms"
		}},
		{"invalid subject type", func(decision *RightsDecision) { decision.SubjectType = RightsSubjectType("asset") }},
		{"missing subject", func(decision *RightsDecision) { decision.SubjectKey = "" }},
		{"invalid input digest", func(decision *RightsDecision) { decision.InputDigest = "not-a-digest" }},
		{"invalid action", func(decision *RightsDecision) { decision.Action = RightsAction("store") }},
		{"invalid result", func(decision *RightsDecision) { decision.Decision = RightsState("") }},
		{"missing evaluator", func(decision *RightsDecision) { decision.Evaluator = "" }},
		{"invalid reason code", func(decision *RightsDecision) { decision.ReasonCodes = []string{"Not Safe"} }},
		{"zero evaluation time", func(decision *RightsDecision) { decision.EvaluatedAt = time.Time{} }},
		{"zero effective time", func(decision *RightsDecision) { decision.EffectiveFrom = time.Time{} }},
		{"expiry not after effective time", func(decision *RightsDecision) { same := decision.EffectiveFrom; decision.ExpiresAt = &same }},
		{"non-retain duration", func(decision *RightsDecision) { decision.RetentionDays = intPointer(30) }},
		{"invalid supersession", func(decision *RightsDecision) { decision.SupersedesDecisionID = rightsInt64Pointer(decision.ID) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decision := valid
			test.mutate(&decision)
			if err := decision.Validate(); err == nil {
				t.Fatalf("Validate() accepted %#v", decision)
			}
			if decision.Allows(evaluatedAt.Add(time.Hour)) {
				t.Fatal("invalid decision authorized its action")
			}
		})
	}
}

func TestRetainDecisionAloneCarriesRetentionDays(t *testing.T) {
	t.Parallel()
	decision := testRightsDecision(RightsScope{Type: RightsScopeSourceEndpoint, SubjectID: "source-42"}, RightsPriorityEndpointContract)
	decision.Action = RightsActionRetain
	if err := decision.Validate(); err == nil {
		t.Fatal("retain decision without retention days was accepted")
	}
	decision.RetentionDays = intPointer(30)
	if err := decision.Validate(); err != nil {
		t.Fatalf("valid retain decision rejected: %v", err)
	}
	decision.Decision = RightsUnknown
	if decision.Allows(decision.EffectiveFrom.Add(time.Hour)) {
		t.Fatal("unknown retain decision authorized retention")
	}
}

func TestRightsDecisionSupportsOnlyFixedScopePriorityHierarchy(t *testing.T) {
	t.Parallel()
	tests := []struct {
		scope      RightsScope
		priorities []RightsPriority
	}{
		{RightsScope{Type: RightsScopeOrganizationDefault}, []RightsPriority{RightsPriorityOrganizationDefault}},
		{RightsScope{Type: RightsScopeSourceEndpoint, SubjectID: "endpoint-1"}, []RightsPriority{RightsPriorityConnectorRestriction, RightsPriorityEndpointContract}},
		{RightsScope{Type: RightsScopePublisher, SubjectID: "publisher-1"}, []RightsPriority{RightsPriorityConnectorRestriction, RightsPriorityEndpointContract}},
		{RightsScope{Type: RightsScopeFeedOrAccount, SubjectID: "feed-1"}, []RightsPriority{RightsPriorityConnectorRestriction, RightsPriorityEndpointContract}},
		{RightsScope{Type: RightsScopeObservation, SubjectID: "observation-1"}, []RightsPriority{RightsPriorityObservationExplicit}},
	}
	for _, test := range tests {
		for _, priority := range test.priorities {
			decision := testRightsDecision(test.scope, priority)
			if err := decision.Validate(); err != nil {
				t.Errorf("scope %q priority %d rejected: %v", test.scope.Type, priority, err)
			}
		}
	}
}

func testRightsDecision(scope RightsScope, priority RightsPriority) RightsDecision {
	evaluatedAt := time.Date(2026, time.August, 9, 4, 0, 0, 0, time.UTC)
	return RightsDecision{
		ID: 7, SourceConnectionID: 42,
		Scope: scope, PolicyID: 11, PolicyRevision: 1, Priority: priority,
		Basis:       RightsBasis{Summary: "reviewed policy"},
		SubjectType: RightsSubjectRawResponse, SubjectKey: strings.Repeat("b", 64), InputDigest: strings.Repeat("a", 64),
		Action: RightsActionStoreRaw, Decision: RightsAllow, ReasonCodes: []string{"contract_allow"}, Evaluator: "policy-v1",
		EvaluatedAt: evaluatedAt, EffectiveFrom: evaluatedAt,
	}
}

func TestRightsValidationErrorsDoNotEchoBasisOrPolicyInput(t *testing.T) {
	t.Parallel()
	secret := "sensitive-policy-material"
	decision := testRightsDecision(RightsScope{Type: RightsScopeSourceEndpoint, SubjectID: "source-1"}, RightsPriorityEndpointContract)
	decision.Basis.TermsURL = "https://user:" + secret + "@publisher.example/terms"
	err := decision.Validate()
	if err == nil {
		t.Fatal("Validate() error = nil")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("validation error leaked policy input: %q", err)
	}
}

func intPointer(value int) *int             { return &value }
func rightsInt64Pointer(value int64) *int64 { return &value }
