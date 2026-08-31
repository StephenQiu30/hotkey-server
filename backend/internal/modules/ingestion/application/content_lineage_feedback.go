package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	ingestiondomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/domain"
)

var ErrContentLineageFeedbackDenied = errors.New("content lineage feedback is not authorized")

type ReadContentLineageFeedbackTargetQuery struct {
	ActorUserID       int64
	LineageDecisionID int64
}

type ContentLineageFeedbackTargetDTO struct {
	LineageDecisionID       int64
	DocumentVersionID       int64
	FingerprintID           int64
	ContentFamilyID         int64
	FamilyVersion           int64
	MemberID                int64
	MemberVersion           int64
	Relation                string
	ParentDocumentVersionID *int64
	LineageProfileVersion   string
}

type ReviewContentLineageCommand struct {
	ActorUserID                   int64
	LineageDecisionID             int64
	ExpectedMemberVersion         int64
	FeedbackType                  string
	RelationOverride              string
	TargetParentDocumentVersionID int64
	ExpectedTargetMemberVersion   int64
	ReasonCode                    string
	Note                          string
	IdempotencyKey                string
}

type CommitContentLineageFeedbackCommand struct {
	ActorUserID                     int64
	LineageDecisionID               int64
	DocumentVersionID               int64
	FingerprintID                   int64
	OriginalFamilyID                int64
	ExpectedFamilyVersion           int64
	MemberID                        int64
	ExpectedMemberVersion           int64
	OriginalRelation                string
	OriginalParentDocumentVersionID *int64
	ResultRelation                  string
	ResultParentDocumentVersionID   *int64
	ExpectedTargetMemberVersion     int64
	LineageProfileVersion           string
	FeedbackType                    string
	RelationOverride                *string
	ReasonCode                      string
	Note                            string
	IdempotencyKey                  string
	CommandFingerprint              string
}

type ContentLineageFeedbackDTO struct {
	FeedbackID                      int64
	LineageDecisionID               int64
	ResultLineageDecisionID         int64
	DocumentVersionID               int64
	OriginalFamilyID                int64
	ResultFamilyID                  int64
	ResultFamilyVersion             int64
	OriginalRelation                string
	ResultRelation                  string
	OriginalParentDocumentVersionID *int64
	ResultParentDocumentVersionID   *int64
	FeedbackType                    string
	ActorUserID                     int64
	Reused                          bool
}

type FindContentLineageFeedbackReceiptQuery struct {
	ActorUserID        int64
	IdempotencyKey     string
	CommandFingerprint string
}

type ReviewContentLineageResult struct{ Feedback ContentLineageFeedbackDTO }

type ContentLineageFeedbackRepository interface {
	FindContentLineageFeedbackReceipt(context.Context, FindContentLineageFeedbackReceiptQuery) (ContentLineageFeedbackDTO, bool, error)
	ReadContentLineageFeedbackTarget(context.Context, ReadContentLineageFeedbackTargetQuery) (ContentLineageFeedbackTargetDTO, error)
	CommitContentLineageFeedback(context.Context, CommitContentLineageFeedbackCommand) (ContentLineageFeedbackDTO, error)
}

type ContentLineageFeedbackService struct {
	repository ContentLineageFeedbackRepository
}

func NewContentLineageFeedbackService(repository ContentLineageFeedbackRepository) (*ContentLineageFeedbackService, error) {
	if repository == nil {
		return nil, fmt.Errorf("%w: feedback repository is required", ErrInvalidContentFamilyContract)
	}
	return &ContentLineageFeedbackService{repository: repository}, nil
}

func (service *ContentLineageFeedbackService) Review(ctx context.Context, command ReviewContentLineageCommand) (ReviewContentLineageResult, error) {
	if service == nil || service.repository == nil || command.ActorUserID <= 0 || command.LineageDecisionID <= 0 ||
		command.ExpectedMemberVersion <= 0 || !validContentLineageFeedbackType(command.FeedbackType) ||
		strings.TrimSpace(command.ReasonCode) == "" || len(command.ReasonCode) > 64 || len(command.Note) > 1000 ||
		strings.TrimSpace(command.IdempotencyKey) == "" || len(command.IdempotencyKey) > 96 {
		return ReviewContentLineageResult{}, ErrInvalidContentFamilyContract
	}
	requestFingerprint := contentLineageReviewFingerprint(command)
	replayed, found, err := service.repository.FindContentLineageFeedbackReceipt(ctx, FindContentLineageFeedbackReceiptQuery{
		ActorUserID: command.ActorUserID, IdempotencyKey: command.IdempotencyKey, CommandFingerprint: requestFingerprint,
	})
	if err != nil {
		return ReviewContentLineageResult{}, fmt.Errorf("find content lineage feedback receipt: %w", err)
	}
	if found {
		return ReviewContentLineageResult{Feedback: replayed}, nil
	}
	target, err := service.repository.ReadContentLineageFeedbackTarget(ctx, ReadContentLineageFeedbackTargetQuery{
		ActorUserID: command.ActorUserID, LineageDecisionID: command.LineageDecisionID,
	})
	if err != nil {
		return ReviewContentLineageResult{}, fmt.Errorf("read content lineage feedback target: %w", err)
	}
	if target.LineageDecisionID != command.LineageDecisionID || target.DocumentVersionID <= 0 || target.FingerprintID <= 0 ||
		target.ContentFamilyID <= 0 || target.FamilyVersion <= 0 || target.MemberID <= 0 ||
		target.MemberVersion != command.ExpectedMemberVersion || strings.TrimSpace(target.LineageProfileVersion) == "" {
		return ReviewContentLineageResult{}, ErrInvalidContentFamilyContract
	}
	originalRelation := ingestiondomain.ContentRelation(target.Relation)
	if !originalRelation.Valid() {
		return ReviewContentLineageResult{}, ErrInvalidContentFamilyContract
	}
	resultRelation, resultParent, relationOverride, err := contentLineageFeedbackResult(command)
	if err != nil {
		return ReviewContentLineageResult{}, err
	}
	mutation := CommitContentLineageFeedbackCommand{
		ActorUserID: command.ActorUserID, LineageDecisionID: target.LineageDecisionID,
		DocumentVersionID: target.DocumentVersionID, FingerprintID: target.FingerprintID,
		OriginalFamilyID: target.ContentFamilyID, ExpectedFamilyVersion: target.FamilyVersion,
		MemberID: target.MemberID, ExpectedMemberVersion: target.MemberVersion,
		OriginalRelation: target.Relation, OriginalParentDocumentVersionID: cloneLineageParent(target.ParentDocumentVersionID),
		ResultRelation: resultRelation, ResultParentDocumentVersionID: resultParent,
		ExpectedTargetMemberVersion: command.ExpectedTargetMemberVersion,
		LineageProfileVersion:       target.LineageProfileVersion, FeedbackType: command.FeedbackType,
		RelationOverride: relationOverride, ReasonCode: strings.TrimSpace(command.ReasonCode), Note: command.Note,
		IdempotencyKey: command.IdempotencyKey,
	}
	mutation.CommandFingerprint = requestFingerprint
	feedback, err := service.repository.CommitContentLineageFeedback(ctx, mutation)
	if err != nil {
		return ReviewContentLineageResult{}, fmt.Errorf("commit content lineage feedback: %w", err)
	}
	if !contentLineageFeedbackReceiptMatches(feedback, mutation) {
		return ReviewContentLineageResult{}, fmt.Errorf("%w: feedback receipt changed", ErrInvalidContentFamilyContract)
	}
	return ReviewContentLineageResult{Feedback: feedback}, nil
}

func contentLineageFeedbackResult(command ReviewContentLineageCommand) (string, *int64, *string, error) {
	result := ingestiondomain.ContentRelationUnrelated
	var override *string
	switch command.FeedbackType {
	case "duplicate":
		result = ingestiondomain.ContentRelationExactCopy
	case "relation_override":
		candidate := ingestiondomain.ContentRelation(strings.TrimSpace(command.RelationOverride))
		if !candidate.Valid() {
			return "", nil, nil, ErrInvalidContentFamilyContract
		}
		result = candidate
		value := string(candidate)
		override = &value
	}
	if result == ingestiondomain.ContentRelationUnrelated {
		if command.TargetParentDocumentVersionID != 0 || command.ExpectedTargetMemberVersion != 0 {
			return "", nil, nil, ErrInvalidContentFamilyContract
		}
		return string(result), nil, override, nil
	}
	if command.TargetParentDocumentVersionID <= 0 || command.ExpectedTargetMemberVersion <= 0 {
		return "", nil, nil, ErrInvalidContentFamilyContract
	}
	parent := command.TargetParentDocumentVersionID
	return string(result), &parent, override, nil
}

func contentLineageReviewFingerprint(command ReviewContentLineageCommand) string {
	digest := sha256.Sum256([]byte(fmt.Sprintf("%d|%d|%d|%s|%s|%d|%d|%s|%s",
		command.ActorUserID, command.LineageDecisionID, command.ExpectedMemberVersion, command.FeedbackType,
		strings.TrimSpace(command.RelationOverride), command.TargetParentDocumentVersionID,
		command.ExpectedTargetMemberVersion, strings.TrimSpace(command.ReasonCode), command.Note)))
	return hex.EncodeToString(digest[:])
}

func contentLineageFeedbackReceiptMatches(value ContentLineageFeedbackDTO, command CommitContentLineageFeedbackCommand) bool {
	return value.FeedbackID > 0 && value.LineageDecisionID == command.LineageDecisionID &&
		value.ResultLineageDecisionID > 0 && value.DocumentVersionID == command.DocumentVersionID &&
		value.OriginalFamilyID == command.OriginalFamilyID && value.ResultFamilyID > 0 && value.ResultFamilyVersion > 0 &&
		value.OriginalRelation == command.OriginalRelation && value.ResultRelation == command.ResultRelation &&
		lineageParentsEqual(value.OriginalParentDocumentVersionID, command.OriginalParentDocumentVersionID) &&
		lineageParentsEqual(value.ResultParentDocumentVersionID, command.ResultParentDocumentVersionID) &&
		value.FeedbackType == command.FeedbackType && value.ActorUserID == command.ActorUserID
}

func validContentLineageFeedbackType(value string) bool {
	return value == "duplicate" || value == "not_duplicate" || value == "relation_override" || value == "withdraw"
}

func cloneLineageParent(value *int64) *int64 {
	if value == nil {
		return nil
	}
	result := *value
	return &result
}

func lineageParentValue(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}

func lineageParentsEqual(left, right *int64) bool {
	return left == nil && right == nil || left != nil && right != nil && *left == *right
}
