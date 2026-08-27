package application

import (
	"context"
	"errors"
	"fmt"
	"time"
)

const (
	RawEvidenceDeleteObjectFailed    = "OBJECT_DELETE_FAILED"
	RawEvidenceDeleteIntegrityFailed = "OBJECT_DELETE_INTEGRITY_FAILED"
	MaximumRawEvidenceRetentionBatch = 100
)

// RawEvidenceRetentionCandidateDTO is a durable deletion lease. Object bytes
// may be removed only when the immutable object identity still matches these
// database facts. Metadata and hashes remain in PostgreSQL after tombstoning.
type RawEvidenceRetentionCandidateDTO struct {
	SnapshotID             int64
	SourceConnectionID     int64
	AttemptNo              int
	EvidenceKey            string
	ObjectKey              string
	PayloadSHA256          string
	RetentionUntil         time.Time
	RetentionPolicyID      int64
	RetentionPolicyVersion int64
}

func (candidate RawEvidenceRetentionCandidateDTO) Validate(at time.Time) error {
	if candidate.SnapshotID <= 0 || candidate.SourceConnectionID <= 0 || candidate.AttemptNo <= 0 ||
		!validSHA256Hex(candidate.EvidenceKey) || !validSHA256Hex(candidate.PayloadSHA256) ||
		candidate.ObjectKey != RawEvidenceObjectKey(candidate.SourceConnectionID, candidate.EvidenceKey) ||
		candidate.RetentionUntil.IsZero() || candidate.RetentionUntil.After(at) ||
		candidate.RetentionPolicyID <= 0 || candidate.RetentionPolicyVersion <= 0 {
		return errors.New("raw evidence retention candidate is invalid")
	}
	return nil
}

type DeleteRawEvidenceObjectCommand struct {
	SnapshotID         int64
	SourceConnectionID int64
	EvidenceKey        string
	ObjectKey          string
	PayloadSHA256      string
}

type DeleteRawEvidenceObjectResult struct {
	ObjectKey      string
	PayloadSHA256  string
	Deleted        bool
	AlreadyMissing bool
}

func (result DeleteRawEvidenceObjectResult) ValidateAgainst(command DeleteRawEvidenceObjectCommand) error {
	if command.SnapshotID <= 0 || command.SourceConnectionID <= 0 || !validSHA256Hex(command.EvidenceKey) ||
		!validSHA256Hex(command.PayloadSHA256) || command.ObjectKey != RawEvidenceObjectKey(command.SourceConnectionID, command.EvidenceKey) ||
		result.ObjectKey != command.ObjectKey || result.PayloadSHA256 != command.PayloadSHA256 || result.Deleted == result.AlreadyMissing {
		return fmt.Errorf("raw evidence delete receipt conflicts with immutable identity")
	}
	return nil
}

type CompleteRawEvidenceDeletionCommand struct {
	SnapshotID     int64
	AttemptNo      int
	ObjectKey      string
	PayloadSHA256  string
	DeletedAt      time.Time
	AlreadyMissing bool
}

type FailRawEvidenceDeletionCommand struct {
	SnapshotID    int64
	AttemptNo     int
	ObjectKey     string
	PayloadSHA256 string
	FailureCode   string
	FailedAt      time.Time
}

type RawEvidenceRetentionRepository interface {
	ClaimExpired(context.Context, time.Time, int) ([]RawEvidenceRetentionCandidateDTO, error)
	CompleteDeletion(context.Context, CompleteRawEvidenceDeletionCommand) error
	FailDeletion(context.Context, FailRawEvidenceDeletionCommand) error
}

type RawEvidenceObjectDeleter interface {
	DeleteIfMatches(context.Context, DeleteRawEvidenceObjectCommand) (DeleteRawEvidenceObjectResult, error)
}

type RunRawEvidenceRetentionCommand struct {
	At    time.Time
	Limit int
}

type RunRawEvidenceRetentionResult struct {
	Claimed int
	Deleted int
	Failed  int
	HasMore bool
}

type RawEvidenceRetentionDependencies struct {
	Repository RawEvidenceRetentionRepository
	Deleter    RawEvidenceObjectDeleter
}

type RawEvidenceRetentionService struct {
	repository RawEvidenceRetentionRepository
	deleter    RawEvidenceObjectDeleter
}

func NewRawEvidenceRetentionService(dependencies RawEvidenceRetentionDependencies) (*RawEvidenceRetentionService, error) {
	if dependencies.Repository == nil || dependencies.Deleter == nil {
		return nil, errors.New("raw evidence retention dependencies are required")
	}
	return &RawEvidenceRetentionService{repository: dependencies.Repository, deleter: dependencies.Deleter}, nil
}

func (service *RawEvidenceRetentionService) Run(ctx context.Context, command RunRawEvidenceRetentionCommand) (RunRawEvidenceRetentionResult, error) {
	if service == nil || service.repository == nil || service.deleter == nil || command.At.IsZero() ||
		command.Limit < 1 || command.Limit > MaximumRawEvidenceRetentionBatch {
		return RunRawEvidenceRetentionResult{}, errors.New("raw evidence retention command is invalid")
	}
	at := command.At.UTC()
	candidates, err := service.repository.ClaimExpired(ctx, at, command.Limit)
	if err != nil {
		return RunRawEvidenceRetentionResult{}, fmt.Errorf("claim expired raw evidence: %w", err)
	}
	if len(candidates) > command.Limit {
		return RunRawEvidenceRetentionResult{}, errors.New("raw evidence retention repository exceeded batch limit")
	}
	result := RunRawEvidenceRetentionResult{Claimed: len(candidates), HasMore: len(candidates) == command.Limit}
	for _, candidate := range candidates {
		if err := candidate.Validate(at); err != nil {
			return RunRawEvidenceRetentionResult{}, fmt.Errorf("validate expired raw evidence candidate: %w", err)
		}
		deleteCommand := DeleteRawEvidenceObjectCommand{
			SnapshotID: candidate.SnapshotID, SourceConnectionID: candidate.SourceConnectionID,
			EvidenceKey: candidate.EvidenceKey, ObjectKey: candidate.ObjectKey, PayloadSHA256: candidate.PayloadSHA256,
		}
		deleteResult, deleteErr := service.deleter.DeleteIfMatches(ctx, deleteCommand)
		failureCode := ""
		if deleteErr != nil {
			failureCode = RawEvidenceDeleteObjectFailed
		} else if err := deleteResult.ValidateAgainst(deleteCommand); err != nil {
			failureCode = RawEvidenceDeleteIntegrityFailed
		}
		if failureCode != "" {
			if err := service.repository.FailDeletion(ctx, FailRawEvidenceDeletionCommand{
				SnapshotID: candidate.SnapshotID, AttemptNo: candidate.AttemptNo, ObjectKey: candidate.ObjectKey,
				PayloadSHA256: candidate.PayloadSHA256, FailureCode: failureCode, FailedAt: at,
			}); err != nil {
				return result, fmt.Errorf("record raw evidence deletion failure: %w", err)
			}
			result.Failed++
			continue
		}
		if err := service.repository.CompleteDeletion(ctx, CompleteRawEvidenceDeletionCommand{
			SnapshotID: candidate.SnapshotID, AttemptNo: candidate.AttemptNo, ObjectKey: candidate.ObjectKey,
			PayloadSHA256: candidate.PayloadSHA256, DeletedAt: at, AlreadyMissing: deleteResult.AlreadyMissing,
		}); err != nil {
			return result, fmt.Errorf("complete raw evidence deletion: %w", err)
		}
		result.Deleted++
	}
	return result, nil
}
