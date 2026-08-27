package application

import (
	"context"
	"errors"
	"testing"
	"time"
)

type evidenceSummaryRepositoryFake struct {
	command CommitEvidenceSummaryCommand
	commits int
}

func (fake *evidenceSummaryRepositoryFake) CommitEvidenceSummary(_ context.Context, command CommitEvidenceSummaryCommand) (EvidenceSummaryDTO, error) {
	fake.commits++
	fake.command = command
	sentences := make([]EvidenceSummarySentenceDTO, len(command.Sentences))
	for index, input := range command.Sentences {
		sentences[index] = EvidenceSummarySentenceDTO{ID: int64(index + 2), Version: 1, SummaryID: 1, Ordinal: index,
			Text: input.Text, EditorialNote: input.EditorialNote, ClaimEvidenceVersionIDs: input.ClaimEvidenceVersionIDs,
			DecisionOrigin: input.DecisionOrigin, ModelRunID: input.ModelRunID, ActorUserID: input.ActorUserID, CreatedAt: command.CreatedAt}
	}
	return EvidenceSummaryDTO{ID: 1, Version: 1, MicroEventID: command.MicroEventID, EventVersion: command.EventVersion,
		SummaryProfileVersion: command.SummaryProfileVersion, Sentences: sentences, CreatedAt: command.CreatedAt, Created: true}, nil
}

func TestEvidenceSummaryServiceRejectsEventAggregateWithoutSentenceClaimEvidence(t *testing.T) {
	actorID := int64(8)
	fake := &evidenceSummaryRepositoryFake{}
	service, _ := NewEvidenceSummaryService(fake)
	_, err := service.Publish(context.Background(), PublishEvidenceSummaryCommand{MicroEventID: 1, ExpectedEventVersion: 2,
		SummaryProfileVersion: "evidence-summary-v2", IdempotencyKey: "aggregate-only-summary", CreatedAt: time.Now().UTC(),
		Sentences: []EvidenceSummarySentenceInputDTO{{Text: "Event 汇总状态显示多个独立来源。",
			DecisionOrigin: "manual", ActorUserID: &actorID}}})
	if !errors.Is(err, ErrInvalidEvidenceSummaryContract) || fake.commits != 0 {
		t.Fatalf("aggregate-only Publish() error=%v, repository commits=%d", err, fake.commits)
	}
}

func TestEvidenceSummaryServicePublishesCanonicalCitationBoundSentences(t *testing.T) {
	actorID := int64(8)
	fake := &evidenceSummaryRepositoryFake{}
	service, _ := NewEvidenceSummaryService(fake)
	result, err := service.Publish(context.Background(), PublishEvidenceSummaryCommand{MicroEventID: 1, ExpectedEventVersion: 2,
		SummaryProfileVersion: "evidence-summary-v2", IdempotencyKey: "summary-1", CreatedAt: time.Now().UTC(),
		Sentences: []EvidenceSummarySentenceInputDTO{{Text: "  cited   sentence ", ClaimEvidenceVersionIDs: []int64{3},
			DecisionOrigin: "manual", ActorUserID: &actorID}}})
	if err != nil || result.Summary.Sentences[0].Text != "cited sentence" || len(fake.command.CommandFingerprint) != 64 {
		t.Fatalf("Publish() = %#v, command=%#v, error=%v", result, fake.command, err)
	}
}
