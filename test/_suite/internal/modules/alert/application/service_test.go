package application

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/modules/alert/domain"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
)

type candidateReaderFake struct {
	candidates []EventAlertCandidate
	err        error
	refs       []EventUpdateRef
}

func (fake *candidateReaderFake) ListAlertCandidates(_ context.Context, ref EventUpdateRef) ([]EventAlertCandidate, error) {
	fake.refs = append(fake.refs, ref)
	return fake.candidates, fake.err
}

type policyReaderFake struct {
	policies   []PublishedAlertPolicy
	err        error
	monitorIDs [][]int64
}

func (fake *policyReaderFake) ListPublishedAlertPolicies(_ context.Context, monitorIDs []int64) ([]PublishedAlertPolicy, error) {
	fake.monitorIDs = append(fake.monitorIDs, append([]int64(nil), monitorIDs...))
	return fake.policies, fake.err
}

type occurrenceWriterFake struct {
	commands []domain.RecordOccurrenceCommand
	err      error
}

func (fake *occurrenceWriterFake) RecordOccurrence(_ context.Context, command domain.RecordOccurrenceCommand) (domain.RecordOccurrenceResult, error) {
	fake.commands = append(fake.commands, command)
	if fake.err != nil {
		return domain.RecordOccurrenceResult{}, fake.err
	}
	return domain.RecordOccurrenceResult{Created: true}, nil
}

var _ EventCandidateReader = (*candidateReaderFake)(nil)
var _ MonitorPolicyReader = (*policyReaderFake)(nil)
var _ OccurrenceWriter = (*occurrenceWriterFake)(nil)

func TestServiceUsesNarrowPortsForEligibilityAndFreezesPublishedPolicy(t *testing.T) {
	t.Parallel()
	observedAt := time.Date(2026, 8, 4, 4, 0, 0, 0, time.UTC)
	candidates := &candidateReaderFake{candidates: []EventAlertCandidate{
		{MonitorID: 10, EventID: 20, UpdateKind: "rising", FinalScore: 75, TitleSnapshot: "Event 20", ReasonSnapshot: "heat entered rising", TriggeredAt: observedAt},
		{MonitorID: 11, EventID: 21, UpdateKind: "cooling", FinalScore: 99, TitleSnapshot: "ignored cooling", TriggeredAt: observedAt},
		{MonitorID: 12, EventID: 22, UpdateKind: "event_created", FinalScore: 74.99, TitleSnapshot: "below threshold", TriggeredAt: observedAt},
		{MonitorID: 13, EventID: 23, UpdateKind: "reactivated", FinalScore: 98, TitleSnapshot: "no active policy", TriggeredAt: observedAt},
	}}
	policies := &policyReaderFake{policies: []PublishedAlertPolicy{
		{MonitorID: 10, ConfigVersionID: 101, Revision: 7, ConfigHash: strings.Repeat("a", 64), EventThreshold: 75},
		{MonitorID: 12, ConfigVersionID: 102, Revision: 3, ConfigHash: strings.Repeat("b", 64), EventThreshold: 75},
	}}
	writer := &occurrenceWriterFake{}
	service, err := NewService(Dependencies{Candidates: candidates, Policies: policies, Occurrences: writer})
	if err != nil {
		t.Fatal(err)
	}
	ref := EventUpdateRef{ID: 41, Version: 2, EvidenceSetHash: strings.Repeat("e", 64)}
	result, err := service.Evaluate(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	if result.CandidateCount != 4 || result.EligibleCount != 1 || result.CreatedCount != 1 {
		t.Fatalf("evaluation result = %#v", result)
	}
	if len(candidates.refs) != 1 || candidates.refs[0] != ref {
		t.Fatalf("candidate refs = %#v", candidates.refs)
	}
	if len(policies.monitorIDs) != 1 || !reflect.DeepEqual(policies.monitorIDs[0], []int64{10, 12, 13}) {
		t.Fatalf("policy lookup monitor IDs = %#v, want actionable IDs only", policies.monitorIDs)
	}
	if len(writer.commands) != 1 {
		t.Fatalf("RecordOccurrence calls = %d, want 1", len(writer.commands))
	}
	command := writer.commands[0]
	if command.MonitorID != 10 || command.EventID != 20 || command.EventUpdateID != ref.ID || command.TriggerType != domain.TriggerRising {
		t.Fatalf("trigger identity = %#v", command)
	}
	if command.MonitorConfigVersionID != 101 || command.MonitorRevision != 7 || command.MonitorConfigHash != strings.Repeat("a", 64) || command.EventThresholdSnapshot != 75 {
		t.Fatalf("published policy was not frozen: %#v", command)
	}
	if command.FinalScoreSnapshot != 75 || command.Severity != domain.SeverityWarning || command.PolicyVersion != domain.PolicyVersionV1 || command.Fingerprint == "" {
		t.Fatalf("occurrence snapshot = %#v", command)
	}
}

func TestServiceKeepsPublishedRevisionsAsDistinctThreadIdentities(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 4, 4, 0, 0, 0, time.UTC)
	candidates := &candidateReaderFake{candidates: []EventAlertCandidate{{MonitorID: 10, EventID: 20, UpdateKind: "metric_changed", FinalScore: 91, TriggeredAt: now}}}
	policies := &policyReaderFake{policies: []PublishedAlertPolicy{{MonitorID: 10, ConfigVersionID: 101, Revision: 7, ConfigHash: strings.Repeat("a", 64), EventThreshold: 80}}}
	writer := &occurrenceWriterFake{}
	service, err := NewService(Dependencies{Candidates: candidates, Policies: policies, Occurrences: writer})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Evaluate(context.Background(), EventUpdateRef{ID: 41, Version: 1, EvidenceSetHash: strings.Repeat("e", 64)}); err != nil {
		t.Fatal(err)
	}
	policies.policies[0] = PublishedAlertPolicy{MonitorID: 10, ConfigVersionID: 202, Revision: 8, ConfigHash: strings.Repeat("b", 64), EventThreshold: 90}
	if _, err := service.Evaluate(context.Background(), EventUpdateRef{ID: 42, Version: 1, EvidenceSetHash: strings.Repeat("f", 64)}); err != nil {
		t.Fatal(err)
	}
	if len(writer.commands) != 2 || writer.commands[0].MonitorConfigVersionID == writer.commands[1].MonitorConfigVersionID || writer.commands[0].Fingerprint == writer.commands[1].Fingerprint {
		t.Fatalf("revision snapshots were conflated: %#v", writer.commands)
	}
}

func TestServiceRejectsInvalidRefsAndStopsAtPortFailures(t *testing.T) {
	t.Parallel()
	candidates := &candidateReaderFake{}
	policies := &policyReaderFake{}
	writer := &occurrenceWriterFake{}
	service, err := NewService(Dependencies{Candidates: candidates, Policies: policies, Occurrences: writer})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Evaluate(context.Background(), EventUpdateRef{}); !errors.Is(err, sharedrepository.ErrInvalidInput) {
		t.Fatalf("invalid ref error = %v, want ErrInvalidInput", err)
	}
	if len(candidates.refs) != 0 {
		t.Fatal("invalid ref reached the candidate port")
	}

	candidates.err = sharedrepository.ErrUnavailable
	if _, err := service.Evaluate(context.Background(), EventUpdateRef{ID: 1, Version: 1, EvidenceSetHash: strings.Repeat("a", 64)}); !errors.Is(err, sharedrepository.ErrUnavailable) {
		t.Fatalf("candidate error = %v", err)
	}
	if len(policies.monitorIDs) != 0 || len(writer.commands) != 0 {
		t.Fatal("candidate failure consulted downstream policy or repository ports")
	}
}
