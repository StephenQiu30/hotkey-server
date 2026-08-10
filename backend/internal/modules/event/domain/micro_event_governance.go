package domain

import (
	"errors"
	"strings"
)

type MicroEventGovernanceInput struct {
	Action                     string
	MicroEventID               int64
	ExpectedEventVersion       int64
	MembershipDecisionID       int64
	ContentFamilyID            int64
	ExpectedMemberVersion      int64
	TargetMicroEventID         int64
	ExpectedTargetEventVersion int64
}

func ValidateMicroEventGovernance(input MicroEventGovernanceInput) error {
	if input.MicroEventID <= 0 || input.ExpectedEventVersion <= 0 || !validMicroEventGovernanceAction(input.Action) {
		return errors.New("invalid micro-event governance identity")
	}
	eventOnly := input.Action == "close_event" || input.Action == "reopen_event" || input.Action == "merge_events"
	if eventOnly && (input.MembershipDecisionID != 0 || input.ContentFamilyID != 0 || input.ExpectedMemberVersion != 0) {
		return errors.New("event governance must not carry a member identity")
	}
	if !eventOnly && (input.MembershipDecisionID <= 0 || input.ContentFamilyID <= 0) {
		return errors.New("member governance requires the original decision")
	}
	reviewFeedback := input.Action == "same_event" || input.Action == "different_event"
	if reviewFeedback && input.ExpectedMemberVersion != 0 {
		return errors.New("review feedback cannot claim an existing member")
	}
	if !eventOnly && !reviewFeedback && input.ExpectedMemberVersion <= 0 {
		return errors.New("member mutation requires the exact active member version")
	}
	targetRequired := input.Action == "move_member" || input.Action == "merge_events"
	if targetRequired && (input.TargetMicroEventID <= 0 || input.ExpectedTargetEventVersion <= 0 ||
		input.TargetMicroEventID == input.MicroEventID) {
		return errors.New("micro-event governance target is invalid")
	}
	if !targetRequired && (input.TargetMicroEventID != 0 || input.ExpectedTargetEventVersion != 0) {
		return errors.New("micro-event governance target is not allowed")
	}
	return nil
}

func validMicroEventGovernanceAction(value string) bool {
	switch strings.TrimSpace(value) {
	case "same_event", "different_event", "move_member", "merge_events", "split_event", "withdraw", "close_event", "reopen_event":
		return true
	default:
		return false
	}
}
