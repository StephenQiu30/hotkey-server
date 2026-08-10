package domain

import "testing"

func TestMicroEventGovernanceRequiresExactVersionsAndOriginalDecision(t *testing.T) {
	if err := ValidateMicroEventGovernance(MicroEventGovernanceInput{Action: "move_member", MicroEventID: 1,
		ExpectedEventVersion: 2, MembershipDecisionID: 3, ContentFamilyID: 4, ExpectedMemberVersion: 1,
		TargetMicroEventID: 5, ExpectedTargetEventVersion: 6}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateMicroEventGovernance(MicroEventGovernanceInput{Action: "move_member", MicroEventID: 1,
		ExpectedEventVersion: 2, ContentFamilyID: 4, ExpectedMemberVersion: 1, TargetMicroEventID: 5,
		ExpectedTargetEventVersion: 6}); err == nil {
		t.Fatal("member move without original decision was accepted")
	}
}

func TestMicroEventReviewFeedbackDoesNotInventExistingMember(t *testing.T) {
	if err := ValidateMicroEventGovernance(MicroEventGovernanceInput{Action: "different_event", MicroEventID: 1,
		ExpectedEventVersion: 2, MembershipDecisionID: 3, ContentFamilyID: 4}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateMicroEventGovernance(MicroEventGovernanceInput{Action: "same_event", MicroEventID: 1,
		ExpectedEventVersion: 2, MembershipDecisionID: 3, ContentFamilyID: 4, ExpectedMemberVersion: 1}); err == nil {
		t.Fatal("review feedback claimed an active member")
	}
}
