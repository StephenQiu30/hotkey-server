package application

import (
	"context"
	"testing"
	"time"
)

type microEventRepositoryFake struct {
	target  MicroEventAssignmentTargetDTO
	query   ReadMicroEventAssignmentTargetQuery
	command CommitMicroEventMembershipCommand
}

type microEventQualityReaderFake struct{ active bool }

func (fake microEventQualityReaderFake) IsDecisionQualityProfileActive(context.Context, string, string) (bool, error) {
	return fake.active, nil
}

func (repository *microEventRepositoryFake) ReadMicroEventAssignmentTarget(_ context.Context, query ReadMicroEventAssignmentTargetQuery) (MicroEventAssignmentTargetDTO, error) {
	repository.query = query
	return repository.target, nil
}

func (repository *microEventRepositoryFake) CommitMicroEventMembership(_ context.Context, command CommitMicroEventMembershipCommand) (CommitMicroEventMembershipResult, error) {
	repository.command = command
	eventID, version, status := command.CandidateMicroEventID, command.ExpectedEventVersion+1, "active"
	if command.Action == "create" {
		eventID, version = 90, 1
	}
	if command.Action == "review" {
		status = "review_pending"
	}
	return CommitMicroEventMembershipResult{Event: MicroEventDTO{ID: eventID, Version: version, EventKey: command.EventKey,
		Status: status, PrimarySubjectKey: command.PrimarySubjectKey, PrimaryActionKey: command.PrimaryActionKey,
		LocationKeys: command.LocationKeys, IdentifierKeys: command.IdentifierKeys, EventStartedAt: command.OccurredAt,
		ClusteringProfileVersion: command.ClusteringProfileVersion}, Decision: MicroEventMembershipDecisionDTO{
		ID: 99, ContentFamilyID: command.ContentFamilyID, DocumentMatchDecisionID: command.DocumentMatchDecisionID,
		MicroEventID: eventID, EventVersion: version, Action: command.Action, SameEventScore: command.SameEventScore,
		LeadingMargin: command.LeadingMargin, Features: command.Features, ClusteringProfileVersion: command.ClusteringProfileVersion,
		ReasonCodes: command.ReasonCodes,
	}}, nil
}

func TestMicroEventServiceCreatesOnlyFromAcceptedStructuredMatch(t *testing.T) {
	now := time.Now().UTC()
	repository := &microEventRepositoryFake{target: MicroEventAssignmentTargetDTO{ContentFamilyID: 7, DocumentVersionID: 70,
		DocumentMatchDecisionID: 8, MonitorID: 9, MonitorVersionID: 10, EffectiveMatchDecision: "accepted",
		PrimarySubjectKey: "acme", PrimaryActionKey: "launch", LocationKeys: []string{"shanghai"}, OccurredAt: now}}
	service, err := NewMicroEventService(repository)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Assign(context.Background(), AssignContentFamilyToMicroEventCommand{ContentFamilyID: 7,
		DocumentMatchDecisionID: 8, ClusteringProfileVersion: CanonicalMicroEventClusteringProfileVersion})
	if err != nil || result.Decision.Action != "create" || result.Event.ID != 90 || repository.command.EventKey == "" ||
		repository.command.IdempotencyKey == "" || repository.command.CommandFingerprint == "" {
		t.Fatalf("create result/command = %#v / %#v / %v", result, repository.command, err)
	}
	repository.target.EffectiveMatchDecision = "review"
	if _, err := service.Assign(context.Background(), AssignContentFamilyToMicroEventCommand{ContentFamilyID: 7,
		DocumentMatchDecisionID: 8, ClusteringProfileVersion: CanonicalMicroEventClusteringProfileVersion}); err == nil {
		t.Fatal("non-accepted match entered micro-event clustering")
	}
}

func TestMicroEventServiceJoinsHighMarginCandidate(t *testing.T) {
	now := time.Now().UTC()
	repository := &microEventRepositoryFake{target: MicroEventAssignmentTargetDTO{ContentFamilyID: 17, DocumentVersionID: 170,
		DocumentMatchDecisionID: 18, MonitorID: 19, MonitorVersionID: 20, EffectiveMatchDecision: "accepted",
		PrimarySubjectKey: "acme", PrimaryActionKey: "launch", OccurredAt: now,
		Candidates: []MicroEventCandidateDTO{{MicroEventID: 21, EventVersion: 3, Features: MicroEventFeaturesDTO{
			SparseSimilarity: .96, DenseSimilarity: .95, EntityOverlap: 1, ActionOverlap: 1,
			LocationConsistency: .95, IdentifierConsistency: .95, TimeSimilarity: .95, LineageRelation: .9,
		}}}}}
	service, _ := NewMicroEventService(repository)
	result, err := service.Assign(context.Background(), AssignContentFamilyToMicroEventCommand{ContentFamilyID: 17,
		DocumentMatchDecisionID: 18, ClusteringProfileVersion: CanonicalMicroEventClusteringProfileVersion})
	if err != nil || result.Event.ID != 21 || result.Event.Version != 4 || result.Decision.Action != "join" ||
		repository.command.ExpectedEventVersion != 3 {
		t.Fatalf("join result/command = %#v / %#v / %v", result, repository.command, err)
	}
}

func TestMicroEventServiceDowngradesAutomaticJoinWithoutActiveQualityProfile(t *testing.T) {
	now := time.Now().UTC()
	repository := &microEventRepositoryFake{target: MicroEventAssignmentTargetDTO{ContentFamilyID: 17, DocumentVersionID: 170,
		DocumentMatchDecisionID: 18, MonitorID: 19, MonitorVersionID: 20, EffectiveMatchDecision: "accepted",
		PrimarySubjectKey: "acme", PrimaryActionKey: "launch", OccurredAt: now,
		Candidates: []MicroEventCandidateDTO{{MicroEventID: 21, EventVersion: 3, Features: MicroEventFeaturesDTO{
			SparseSimilarity: .96, DenseSimilarity: .95, EntityOverlap: 1, ActionOverlap: 1,
			LocationConsistency: .95, IdentifierConsistency: .95, TimeSimilarity: .95, LineageRelation: .9,
		}}}}}
	service, err := NewMicroEventServiceWithQualityProfiles(repository, microEventQualityReaderFake{active: false})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Assign(t.Context(), AssignContentFamilyToMicroEventCommand{ContentFamilyID: 17,
		DocumentMatchDecisionID: 18, ClusteringProfileVersion: CanonicalMicroEventClusteringProfileVersion})
	if err != nil {
		t.Fatal(err)
	}
	if result.Decision.Action != "review" || result.Event.Status != "review_pending" {
		t.Fatalf("quality-gated micro-event decision = %#v", result)
	}
}
