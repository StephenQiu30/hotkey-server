package application

import (
	"context"
	"strings"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/modules/event/domain"
)

type governanceFake struct {
	merge MergeCommand
	split SplitCommand
	lock  MemberLockCommand
}

func (fake *governanceFake) Merge(_ context.Context, command MergeCommand) (domain.Event, error) {
	fake.merge = command
	return domain.Event{ID: command.TargetEventID, Version: command.TargetExpectedVersion + 1, EventKey: "evt_target", LifecycleStatus: domain.LifecycleActive}, nil
}
func (fake *governanceFake) Split(_ context.Context, command SplitCommand) (domain.Event, error) {
	fake.split = command
	return domain.Event{ID: 99, Version: 1, EventKey: "evt_split", LifecycleStatus: domain.LifecycleDetected}, nil
}
func (fake *governanceFake) SetMemberLock(_ context.Context, command MemberLockCommand) (domain.EventMember, error) {
	fake.lock = command
	return domain.EventMember{ID: 2, Version: command.ExpectedVersion + 1, EventID: command.EventID, ContentID: command.ContentID, EvidenceRole: domain.EvidenceSupporting, Origin: domain.MemberOriginUser}, nil
}

func TestGovernanceCommandsCarryAllOptimisticVersions(t *testing.T) {
	fake := &governanceFake{}
	service := NewGovernanceService(fake)
	if _, err := service.Merge(context.Background(), MergeCommand{SourceEventID: 1, TargetEventID: 2, SourceExpectedVersion: 3, TargetExpectedVersion: 4, ReasonCode: "confirmed"}); err != nil {
		t.Fatalf("Merge() error = %v", err)
	}
	if fake.merge.SourceExpectedVersion != 3 || fake.merge.TargetExpectedVersion != 4 {
		t.Fatalf("Merge() lost versions: %#v", fake.merge)
	}
	if _, err := service.Split(context.Background(), SplitCommand{SourceEventID: 1, SourceExpectedVersion: 5, Members: []SplitMember{{ContentID: 10, ExpectedVersion: 2}, {ContentID: 11, ExpectedVersion: 3}}, ReasonCode: "separate"}); err != nil {
		t.Fatalf("Split() error = %v", err)
	}
	if len(fake.split.Members) != 2 || fake.split.Members[1].ExpectedVersion != 3 {
		t.Fatalf("Split() lost member versions: %#v", fake.split)
	}
	if _, err := service.SetMemberLock(context.Background(), MemberLockCommand{EventID: 1, ContentID: 10, ExpectedVersion: 2, Locked: false, ReasonCode: "reviewed"}); err != nil {
		t.Fatalf("SetMemberLock() error = %v", err)
	}
	if fake.lock.Locked {
		t.Fatal("SetMemberLock() did not preserve unlock")
	}
}

func TestGovernanceRejectsDuplicateSplitMembers(t *testing.T) {
	_, err := NewGovernanceService(&governanceFake{}).Split(context.Background(), SplitCommand{SourceEventID: 1, SourceExpectedVersion: 1, Members: []SplitMember{{ContentID: 10, ExpectedVersion: 1}, {ContentID: 10, ExpectedVersion: 2}}, ReasonCode: "bad"})
	if err == nil {
		t.Fatal("Split() accepted duplicate content IDs")
	}
}

func TestGovernanceRejectsReasonCodesOutsideAuditContract(t *testing.T) {
	service := NewGovernanceService(&governanceFake{})
	longReason := strings.Repeat("a", domain.MaxReasonCodeLength+1)
	if _, err := service.Merge(context.Background(), MergeCommand{SourceEventID: 1, TargetEventID: 2, SourceExpectedVersion: 1, TargetExpectedVersion: 1, ReasonCode: longReason}); err == nil {
		t.Fatal("Merge() accepted a reason that cannot be persisted in audit")
	}
	if _, err := service.SetMemberLock(context.Background(), MemberLockCommand{EventID: 1, ContentID: 2, ExpectedVersion: 1, ReasonCode: "   "}); err == nil {
		t.Fatal("SetMemberLock() accepted a blank reason")
	}
}
