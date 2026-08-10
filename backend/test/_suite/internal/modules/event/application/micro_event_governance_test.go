package application

import (
	"context"
	"testing"
)

type microEventGovernanceRepositoryStub struct {
	mutation ApplyMicroEventGovernanceCommand
}

func (stub *microEventGovernanceRepositoryStub) ApplyMicroEventGovernance(_ context.Context, command ApplyMicroEventGovernanceCommand) (ApplyMicroEventGovernanceResult, error) {
	stub.mutation = command
	return ApplyMicroEventGovernanceResult{Feedback: MicroEventGovernanceFeedbackDTO{ID: 1, Action: command.Action,
		ActorUserID: command.ActorUserID, MicroEventID: command.MicroEventID,
		OriginalEventVersion: command.ExpectedEventVersion, ResultMicroEventID: command.MicroEventID,
		ResultEventVersion: command.ExpectedEventVersion + 1, GovernanceProfileVersion: command.GovernanceProfileVersion,
		ReasonCode: command.ReasonCode, Note: command.Note, IdempotencyKey: command.IdempotencyKey},
		SourceEvent: MicroEventDTO{ID: command.MicroEventID, Version: command.ExpectedEventVersion + 1,
			Status: "closed"}}, nil
}

func TestMicroEventGovernanceServiceBuildsStablePOJOCommand(t *testing.T) {
	repository := &microEventGovernanceRepositoryStub{}
	service, err := NewMicroEventGovernanceService(repository)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Govern(context.Background(), GovernMicroEventCommand{ActorUserID: 9, Action: "close_event",
		MicroEventID: 4, ExpectedEventVersion: 2, ReasonCode: "resolved", Note: "editor reviewed",
		GovernanceProfileVersion: CanonicalMicroEventGovernanceProfileVersion, IdempotencyKey: "close-event-4-v2"})
	if err != nil || result.SourceEvent.Status != "closed" || len(repository.mutation.CommandFingerprint) != 64 {
		t.Fatalf("governance = %#v / %#v / %v", result, repository.mutation, err)
	}
}
