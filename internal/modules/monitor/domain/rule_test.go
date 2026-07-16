package domain

import "testing"

func TestNormalizeRuleEnforcesRuleMatrixAndApproval(t *testing.T) {
	t.Parallel()

	rule, err := NormalizeRule(MonitorRule{
		RuleType:       RuleTypeKeyword,
		Operator:       RuleOperatorContains,
		Value:          "  HotKey  ",
		Weight:         70,
		Priority:       10,
		Origin:         RuleOriginUser,
		ApprovalStatus: RuleApprovalApproved,
		Enabled:        true,
	})
	if err != nil {
		t.Fatalf("NormalizeRule() error = %v", err)
	}
	if rule.Value != "HotKey" {
		t.Errorf("normalized value = %q, want HotKey", rule.Value)
	}
	if _, err := NormalizeRule(MonitorRule{RuleType: RuleTypeDomain, Operator: RuleOperatorContains, Value: "example.test", Origin: RuleOriginUser, ApprovalStatus: RuleApprovalApproved}); err == nil {
		t.Fatal("domain contains rule = nil error, want matrix rejection")
	}
	if _, err := NormalizeRule(MonitorRule{RuleType: RuleTypeRegex, Operator: RuleOperatorMatches, Value: "(", Origin: RuleOriginUser, ApprovalStatus: RuleApprovalApproved}); err == nil {
		t.Fatal("invalid regular expression = nil error, want rejection")
	}
	if _, err := NewRule(RuleTypeKeyword, RuleOperatorContains, "candidate", 10, 1, RuleOriginAI); err != nil {
		t.Fatalf("NewRule(ai): %v", err)
	}
	if _, err := NewRule(RuleTypeKeyword, RuleOperatorContains, "candidate", 10, 1, RuleOriginUser); err != nil {
		t.Fatalf("NewRule(user): %v", err)
	}
}

func TestRulesRequireApprovedEnabledHumanCoreRule(t *testing.T) {
	t.Parallel()

	rules := []MonitorRule{{RuleType: RuleTypeKeyword, Operator: RuleOperatorContains, Value: "hotkey", Origin: RuleOriginAI, ApprovalStatus: RuleApprovalApproved, Enabled: true}}
	if HasApprovedHumanCoreRule(rules) {
		t.Fatal("AI rule counted as core rule")
	}
	rules[0].Origin = RuleOriginSystem
	if !HasApprovedHumanCoreRule(rules) {
		t.Fatal("approved enabled system keyword did not count as core rule")
	}
}

func TestNormalizeQueryOverrideRejectsNULAndCanonicalizesUnicode(t *testing.T) {
	t.Parallel()

	got, err := NormalizeQueryOverride("  cafe\u0301 ")
	if err != nil {
		t.Fatalf("NormalizeQueryOverride() error = %v", err)
	}
	if got != "café" {
		t.Errorf("normalized query override = %q, want café", got)
	}
	if _, err := NormalizeQueryOverride("bad\x00query"); err == nil {
		t.Fatal("NUL query override = nil error, want rejection")
	}
}
