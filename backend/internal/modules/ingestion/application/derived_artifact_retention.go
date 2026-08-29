package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	knowledgeapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/application"
)

const (
	DerivedArtifactDeleteVaultFailed      = "VAULT_DELETE_FAILED"
	DerivedArtifactDeleteIntegrityFailed  = "VAULT_DELETE_INTEGRITY_FAILED"
	DerivedArtifactDeleteRetentionExpired = "RETENTION_EXPIRED"
	DerivedArtifactDeleteRightsRevoked    = "RIGHTS_REVOKED"
	MaximumDerivedArtifactRetentionBatch  = 100
)

// DerivedArtifactRetentionCandidateDTO is a durable deletion lease for one
// rebuildable automatic Vault projection. Human-maintainable Knowledge files
// have no derived_artifacts row and therefore cannot enter this workflow.
type DerivedArtifactRetentionCandidateDTO struct {
	ArtifactID               int64
	SourceConnectionID       int64
	DocumentID               int64
	DocumentVersionID        int64
	AttemptNo                int
	ArtifactType             string
	TransformerProfileSHA256 string
	VaultRelativePath        string
	MIMEType                 string
	SHA256                   string
	SizeBytes                int64
	RetentionUntil           time.Time
	RetentionPolicyID        int64
	RetentionPolicyVersion   int64
	ReasonCode               string
}

func (candidate DerivedArtifactRetentionCandidateDTO) Validate(at time.Time) error {
	if candidate.ArtifactID <= 0 || candidate.SourceConnectionID <= 0 || candidate.AttemptNo <= 0 ||
		candidate.RetentionUntil.IsZero() || candidate.RetentionPolicyID <= 0 || candidate.RetentionPolicyVersion <= 0 ||
		(candidate.ReasonCode != DerivedArtifactDeleteRetentionExpired && candidate.ReasonCode != DerivedArtifactDeleteRightsRevoked) ||
		(candidate.ReasonCode == DerivedArtifactDeleteRetentionExpired && candidate.RetentionUntil.After(at)) ||
		knowledgeapplication.ValidateProjectionStoreReceiptDTO(candidate.ProjectionReceipt()) != nil {
		return errors.New("derived artifact retention candidate is invalid")
	}
	return nil
}

func (candidate DerivedArtifactRetentionCandidateDTO) ProjectionReceipt() knowledgeapplication.ProjectionStoreReceiptDTO {
	return knowledgeapplication.ProjectionStoreReceiptDTO{
		DocumentID: candidate.DocumentID, DocumentVersionID: candidate.DocumentVersionID,
		Format: candidate.ArtifactType, TransformerProfileSHA256: candidate.TransformerProfileSHA256,
		RelativePath: candidate.VaultRelativePath, MIMEType: candidate.MIMEType,
		SHA256: candidate.SHA256, SizeBytes: candidate.SizeBytes,
	}
}

type CompleteDerivedArtifactDeletionCommand struct {
	ArtifactID        int64
	AttemptNo         int
	VaultRelativePath string
	SHA256            string
	SizeBytes         int64
	DeletedAt         time.Time
	AlreadyMissing    bool
}

type FailDerivedArtifactDeletionCommand struct {
	ArtifactID        int64
	AttemptNo         int
	VaultRelativePath string
	SHA256            string
	SizeBytes         int64
	FailureCode       string
	FailedAt          time.Time
}

type DerivedArtifactRetentionRepository interface {
	ClaimExpired(context.Context, time.Time, int) ([]DerivedArtifactRetentionCandidateDTO, error)
	CompleteDeletion(context.Context, CompleteDerivedArtifactDeletionCommand) error
	FailDeletion(context.Context, FailDerivedArtifactDeletionCommand) error
}

type RunDerivedArtifactRetentionCommand struct {
	At    time.Time
	Limit int
}

type RunDerivedArtifactRetentionResult struct {
	Claimed int
	Deleted int
	Failed  int
	HasMore bool
}

type DerivedArtifactRetentionDependencies struct {
	Repository DerivedArtifactRetentionRepository
	Deleter    knowledgeapplication.ProjectionDeleter
}

type DerivedArtifactRetentionService struct {
	repository DerivedArtifactRetentionRepository
	deleter    knowledgeapplication.ProjectionDeleter
}

func NewDerivedArtifactRetentionService(dependencies DerivedArtifactRetentionDependencies) (*DerivedArtifactRetentionService, error) {
	if dependencies.Repository == nil || dependencies.Deleter == nil {
		return nil, errors.New("derived artifact retention dependencies are required")
	}
	return &DerivedArtifactRetentionService{repository: dependencies.Repository, deleter: dependencies.Deleter}, nil
}

func (service *DerivedArtifactRetentionService) Run(ctx context.Context, command RunDerivedArtifactRetentionCommand) (RunDerivedArtifactRetentionResult, error) {
	if service == nil || service.repository == nil || service.deleter == nil || command.At.IsZero() ||
		command.Limit < 1 || command.Limit > MaximumDerivedArtifactRetentionBatch {
		return RunDerivedArtifactRetentionResult{}, errors.New("derived artifact retention command is invalid")
	}
	at := command.At.UTC()
	candidates, err := service.repository.ClaimExpired(ctx, at, command.Limit)
	if err != nil {
		return RunDerivedArtifactRetentionResult{}, fmt.Errorf("claim expired derived artifacts: %w", err)
	}
	if len(candidates) > command.Limit {
		return RunDerivedArtifactRetentionResult{}, errors.New("derived artifact retention repository exceeded batch limit")
	}
	result := RunDerivedArtifactRetentionResult{Claimed: len(candidates), HasMore: len(candidates) == command.Limit}
	for _, candidate := range candidates {
		if err := candidate.Validate(at); err != nil {
			return RunDerivedArtifactRetentionResult{}, fmt.Errorf("validate derived artifact retention candidate: %w", err)
		}
		deleted, deleteErr := service.deleter.DeleteProjection(ctx, knowledgeapplication.DeleteStoredProjectionCommand{Receipt: candidate.ProjectionReceipt()})
		failureCode := ""
		if deleteErr != nil {
			failureCode = DerivedArtifactDeleteVaultFailed
			if errors.Is(deleteErr, knowledgeapplication.ErrProjectionIntegrity) || errors.Is(deleteErr, knowledgeapplication.ErrProjectionConflict) {
				failureCode = DerivedArtifactDeleteIntegrityFailed
			}
		} else if deleted.RelativePath != candidate.VaultRelativePath || deleted.SHA256 != candidate.SHA256 ||
			deleted.SizeBytes != candidate.SizeBytes || deleted.Deleted == deleted.AlreadyMissing {
			failureCode = DerivedArtifactDeleteIntegrityFailed
		}
		if failureCode != "" {
			if err := service.repository.FailDeletion(ctx, FailDerivedArtifactDeletionCommand{
				ArtifactID: candidate.ArtifactID, AttemptNo: candidate.AttemptNo,
				VaultRelativePath: candidate.VaultRelativePath, SHA256: candidate.SHA256, SizeBytes: candidate.SizeBytes,
				FailureCode: failureCode, FailedAt: at,
			}); err != nil {
				return result, fmt.Errorf("record derived artifact deletion failure: %w", err)
			}
			result.Failed++
			continue
		}
		if err := service.repository.CompleteDeletion(ctx, CompleteDerivedArtifactDeletionCommand{
			ArtifactID: candidate.ArtifactID, AttemptNo: candidate.AttemptNo,
			VaultRelativePath: candidate.VaultRelativePath, SHA256: candidate.SHA256, SizeBytes: candidate.SizeBytes,
			DeletedAt: at, AlreadyMissing: deleted.AlreadyMissing,
		}); err != nil {
			return result, fmt.Errorf("complete derived artifact deletion: %w", err)
		}
		result.Deleted++
	}
	return result, nil
}
