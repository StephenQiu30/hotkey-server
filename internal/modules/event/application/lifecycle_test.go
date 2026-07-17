package application

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/modules/event/domain"
)

type lifecycleStoreFake struct {
	event domain.Event
	audit domain.GovernanceAudit
}

type evidenceStoreFake struct {
	command EvidenceRecomputeCommand
	event   domain.Event
}

func (store *evidenceStoreFake) RecomputeEventEvidence(_ context.Context, command EvidenceRecomputeCommand) (domain.Event, error) {
	store.command = command
	return store.event, nil
}

func (store *lifecycleStoreFake) Get(context.Context, int64) (domain.Event, error) {
	return store.event, nil
}
func (store *lifecycleStoreFake) Save(_ context.Context, event domain.Event, expected int64, audit domain.GovernanceAudit) error {
	if store.event.Version != expected {
		return fmt.Errorf("stale")
	}
	event.Version++
	store.event, store.audit = event, audit
	return nil
}

func lifecycleEvent() domain.Event {
	now := time.Now().UTC()
	return domain.Event{ID: 1, Version: 2, EventKey: "evt_1", TitleZH: "事件", LifecycleStatus: domain.LifecycleDetected, FirstSeenAt: now, LastSeenAt: now}
}

func TestLifecycleTransitionHonorsVersionAndStateMachine(t *testing.T) {
	store := &lifecycleStoreFake{event: lifecycleEvent()}
	service := NewLifecycleService(store)
	event, err := service.Transition(context.Background(), LifecycleInput{EventID: 1, ExpectedVersion: 2, To: domain.LifecycleActive, ReasonCode: "two_sources"})
	if err != nil || event.LifecycleStatus != domain.LifecycleActive || store.audit.Action != domain.AuditLifecycleTransition {
		t.Fatalf("Transition() = %#v/%v audit=%#v", event, err, store.audit)
	}
	if _, err := service.Transition(context.Background(), LifecycleInput{EventID: 1, ExpectedVersion: 2, To: domain.LifecycleClosed, ReasonCode: "stale"}); err == nil {
		t.Fatal("Transition() accepted stale event version")
	}
}

func TestRecomputeEvidenceKeepsManualLockAndSelectsStableRepresentative(t *testing.T) {
	event := lifecycleEvent()
	representative := int64(99)
	event.ManualLocked = true
	event.RepresentativeContentID = &representative
	status, contentID, err := RecomputeEvidence(EvidenceInput{Event: event})
	if err != nil || status != event.LifecycleStatus || contentID == nil || *contentID != representative {
		t.Fatalf("locked RecomputeEvidence() = %q/%v/%v", status, contentID, err)
	}
	event.ManualLocked = false
	members := []domain.EventMember{{ID: 2, Version: 1, EventID: 1, ContentID: 20, MembershipScore: 90, EvidenceRole: domain.EvidenceSupporting, Origin: domain.MemberOriginRule}, {ID: 1, Version: 1, EventID: 1, ContentID: 10, MembershipScore: 90, EvidenceRole: domain.EvidencePrimary, Origin: domain.MemberOriginRule}}
	status, contentID, err = RecomputeEvidence(EvidenceInput{Event: event, ValidMembers: members, IndependentSourceIDs: []int64{2, 1}})
	if err != nil || status != domain.LifecycleActive || contentID == nil || *contentID != 10 {
		t.Fatalf("RecomputeEvidence() = %q/%v/%v", status, contentID, err)
	}
	status, contentID, err = RecomputeEvidence(EvidenceInput{Event: event})
	if err != nil || status != domain.LifecycleRejected || contentID != nil {
		t.Fatalf("empty RecomputeEvidence() = %q/%v/%v", status, contentID, err)
	}
	event.LifecycleStatus = domain.LifecycleActive
	status, contentID, err = RecomputeEvidence(EvidenceInput{Event: event, ValidMembers: members, IndependentSourceIDs: []int64{1}})
	if err != nil || status != domain.LifecycleCooling || contentID == nil || *contentID != 10 {
		t.Fatalf("single-source active RecomputeEvidence() = %q/%v/%v", status, contentID, err)
	}
	members[0].ManualLocked = true
	status, contentID, err = RecomputeEvidence(EvidenceInput{Event: event, ValidMembers: members, IndependentSourceIDs: []int64{1, 2}})
	if err != nil || status != domain.LifecycleActive || contentID == nil || *contentID != 20 {
		t.Fatalf("manual member RecomputeEvidence() = %q/%v/%v", status, contentID, err)
	}
}

func TestEvidenceServiceDelegatesValidatedCommands(t *testing.T) {
	store := &evidenceStoreFake{event: lifecycleEvent()}
	service := NewEvidenceService(store)
	result, err := service.Recompute(context.Background(), EvidenceRecomputeCommand{EventID: 1, ReasonCode: "content_deleted"})
	if err != nil || result.ID != 1 || store.command.EventID != 1 {
		t.Fatalf("Recompute() = %#v/%v command=%#v", result, err, store.command)
	}
	if _, err := service.Recompute(context.Background(), EvidenceRecomputeCommand{}); err == nil {
		t.Fatal("Recompute() accepted invalid command")
	}
}
