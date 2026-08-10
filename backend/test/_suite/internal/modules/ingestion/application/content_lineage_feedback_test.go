package application_test

import (
	"context"
	"errors"
	"testing"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
)

func TestContentLineageFeedbackServiceBuildsVersionBoundSplitAndDirectedOverride(t *testing.T) {
	parent := int64(41)
	for _, testCase := range []struct {
		name         string
		command      ingestionapplication.ReviewContentLineageCommand
		wantRelation string
		wantParent   *int64
		wantOverride *string
	}{
		{name: "not duplicate splits to an unrelated family", command: ingestionapplication.ReviewContentLineageCommand{
			ActorUserID: 7, LineageDecisionID: 11, ExpectedMemberVersion: 3, FeedbackType: "not_duplicate",
			ReasonCode: "different_work", IdempotencyKey: "lineage-not-duplicate",
		}, wantRelation: "unrelated"},
		{name: "directed relation binds the reviewed parent version", command: ingestionapplication.ReviewContentLineageCommand{
			ActorUserID: 7, LineageDecisionID: 11, ExpectedMemberVersion: 3, FeedbackType: "relation_override",
			RelationOverride: "revision_of", TargetParentDocumentVersionID: parent, ExpectedTargetMemberVersion: 5,
			ReasonCode: "publisher_revision", IdempotencyKey: "lineage-revision-override",
		}, wantRelation: "revision_of", wantParent: &parent, wantOverride: stringPointer("revision_of")},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			repository := &contentLineageFeedbackRepositoryFake{target: ingestionapplication.ContentLineageFeedbackTargetDTO{
				LineageDecisionID: 11, DocumentVersionID: 31, FingerprintID: 32, ContentFamilyID: 33,
				FamilyVersion: 4, MemberID: 34, MemberVersion: 3, Relation: "near_duplicate",
				ParentDocumentVersionID: int64Pointer(30), LineageProfileVersion: "content-family-decision-v1",
			}}
			service, err := ingestionapplication.NewContentLineageFeedbackService(repository)
			if err != nil {
				t.Fatal(err)
			}
			result, err := service.Review(context.Background(), testCase.command)
			if err != nil {
				t.Fatalf("Review(): %v", err)
			}
			if repository.committed.ResultRelation != testCase.wantRelation ||
				!equalInt64Pointers(repository.committed.ResultParentDocumentVersionID, testCase.wantParent) ||
				!equalStringPointers(repository.committed.RelationOverride, testCase.wantOverride) ||
				repository.committed.CommandFingerprint == "" || result.Feedback.ResultRelation != testCase.wantRelation {
				t.Fatalf("mutation/result = %#v / %#v", repository.committed, result)
			}
		})
	}
}

func TestContentLineageFeedbackServiceRejectsStaleOrUnversionedTargets(t *testing.T) {
	repository := &contentLineageFeedbackRepositoryFake{target: ingestionapplication.ContentLineageFeedbackTargetDTO{
		LineageDecisionID: 11, DocumentVersionID: 31, FingerprintID: 32, ContentFamilyID: 33,
		FamilyVersion: 4, MemberID: 34, MemberVersion: 3, Relation: "near_duplicate",
		LineageProfileVersion: "content-family-decision-v1",
	}}
	service, err := ingestionapplication.NewContentLineageFeedbackService(repository)
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Review(context.Background(), ingestionapplication.ReviewContentLineageCommand{
		ActorUserID: 7, LineageDecisionID: 11, ExpectedMemberVersion: 2, FeedbackType: "withdraw",
		ReasonCode: "wrong_relation", IdempotencyKey: "lineage-stale",
	})
	if !errors.Is(err, ingestionapplication.ErrInvalidContentFamilyContract) || repository.committed.LineageDecisionID != 0 {
		t.Fatalf("stale review error/mutation = %v / %#v", err, repository.committed)
	}
}

type contentLineageFeedbackRepositoryFake struct {
	target    ingestionapplication.ContentLineageFeedbackTargetDTO
	committed ingestionapplication.CommitContentLineageFeedbackCommand
}

func (repository *contentLineageFeedbackRepositoryFake) FindContentLineageFeedbackReceipt(_ context.Context, _ ingestionapplication.FindContentLineageFeedbackReceiptQuery) (ingestionapplication.ContentLineageFeedbackDTO, bool, error) {
	return ingestionapplication.ContentLineageFeedbackDTO{}, false, nil
}

func (repository *contentLineageFeedbackRepositoryFake) ReadContentLineageFeedbackTarget(_ context.Context, _ ingestionapplication.ReadContentLineageFeedbackTargetQuery) (ingestionapplication.ContentLineageFeedbackTargetDTO, error) {
	return repository.target, nil
}

func (repository *contentLineageFeedbackRepositoryFake) CommitContentLineageFeedback(_ context.Context, command ingestionapplication.CommitContentLineageFeedbackCommand) (ingestionapplication.ContentLineageFeedbackDTO, error) {
	repository.committed = command
	return ingestionapplication.ContentLineageFeedbackDTO{FeedbackID: 51, LineageDecisionID: command.LineageDecisionID,
		ResultLineageDecisionID: 52, DocumentVersionID: command.DocumentVersionID,
		OriginalFamilyID: command.OriginalFamilyID, ResultFamilyID: 53, ResultFamilyVersion: 1,
		OriginalRelation: command.OriginalRelation, ResultRelation: command.ResultRelation,
		OriginalParentDocumentVersionID: command.OriginalParentDocumentVersionID,
		ResultParentDocumentVersionID:   command.ResultParentDocumentVersionID,
		FeedbackType:                    command.FeedbackType, ActorUserID: command.ActorUserID}, nil
}

func int64Pointer(value int64) *int64    { return &value }
func stringPointer(value string) *string { return &value }
func equalInt64Pointers(left, right *int64) bool {
	return left == nil && right == nil || left != nil && right != nil && *left == *right
}
func equalStringPointers(left, right *string) bool {
	return left == nil && right == nil || left != nil && right != nil && *left == *right
}
