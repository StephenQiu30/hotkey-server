package application

import (
	"context"
	"testing"
	"time"
)

type storylineRepositoryStub struct {
	target   StorylineAssignmentTargetDTO
	mutation CommitStorylineAssignmentCommand
}

func (stub *storylineRepositoryStub) ReadStorylineAssignmentTarget(context.Context, ReadStorylineAssignmentTargetQuery) (StorylineAssignmentTargetDTO, error) {
	return stub.target, nil
}

func (stub *storylineRepositoryStub) CommitStorylineAssignment(_ context.Context, command CommitStorylineAssignmentCommand) (CommitStorylineAssignmentResult, error) {
	stub.mutation = command
	storylineID := int64(11)
	storylineVersion := int64(1)
	if !command.CreateNew {
		storylineID = command.CandidateStorylineID
		storylineVersion = command.ExpectedStorylineVersion + 1
	}
	return CommitStorylineAssignmentResult{Storyline: StorylineDTO{ID: storylineID, Version: storylineVersion,
		StorylineKey: command.StorylineKey, Title: command.Title, Status: "active",
		RelationProfileVersion: command.RelationProfileVersion}, Relation: StorylineEventDTO{ID: 12,
		StorylineID: storylineID, StorylineVersion: storylineVersion, MicroEventID: command.MicroEventID,
		MicroEventVersion: command.MicroEventVersion, RelationType: command.RelationType,
		RelationScore: command.RelationScore, RelationProfileVersion: command.RelationProfileVersion,
		ReasonCodes: command.ReasonCodes, DecisionOrigin: "automatic"}}, nil
}

func TestStorylineServiceUsesSemanticTargetAndReturnsPOJOReceipt(t *testing.T) {
	now := time.Now().UTC()
	repository := &storylineRepositoryStub{target: StorylineAssignmentTargetDTO{MicroEventID: 22,
		MicroEventVersion: 1, MicroEventStatus: "active", PrimarySubjectKey: "entity:case-1",
		PrimaryActionKey: "action:update", EventStartedAt: now,
		RelationProfileVersion: CanonicalStorylineRelationProfileVersion}}
	service, err := NewStorylineService(repository)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Assign(context.Background(), AssignMicroEventToStorylineCommand{MicroEventID: 22,
		MicroEventVersion: 1, RelationProfileVersion: CanonicalStorylineRelationProfileVersion})
	if err != nil || !repository.mutation.CreateNew || result.Storyline.ID != 11 || result.Relation.MicroEventID != 22 {
		t.Fatalf("assignment = %#v / %#v / %v", result, repository.mutation, err)
	}
	if repository.mutation.IdempotencyKey == "" || len(repository.mutation.CommandFingerprint) != 64 {
		t.Fatalf("mutation identity = %#v", repository.mutation)
	}
}
