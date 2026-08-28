package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/domain"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

const maximumVaultRecoveryBytes int64 = 4 << 20

// VaultRebuildFact is the complete PostgreSQL-owned input required to
// reproduce the current automatic region. SnapshotObjectKey identifies the
// protected full-file copy attached to the current immutable Revision.
type VaultRebuildFact struct {
	Document          domain.Document
	RenderInput       domain.VaultDocumentRenderInput
	SnapshotObjectKey string
}

type VaultRebuildFactReader interface {
	LoadVaultRebuildFact(context.Context, int64) (VaultRebuildFact, error)
}

type VaultSnapshotReader interface {
	ReadVaultSnapshot(context.Context, string, int64) (string, error)
}

type VaultBackupReader interface {
	ReadVaultBackup(context.Context, int64, int64, int64) (string, error)
}

type VaultRecoveryResult struct {
	DocumentID  int64
	RevisionNo  int64
	ContentHash string
	Source      domain.VaultRecoverySource
	Restored    bool
}

type VaultRecoveryService struct {
	facts    VaultRebuildFactReader
	vault    Vault
	revision VaultSnapshotReader
	backup   VaultBackupReader
}

func NewVaultRecoveryService(facts VaultRebuildFactReader, vault Vault, revision VaultSnapshotReader, backup VaultBackupReader) *VaultRecoveryService {
	return &VaultRecoveryService{facts: facts, vault: vault, revision: revision, backup: backup}
}

// Recover restores only a missing current file from verified protected bytes.
// An existing file is treated as authoritative for human edits: any hash or
// identity disagreement stops for Proposal/Reconciliation instead of reading
// an older copy and overwriting it.
func (service *VaultRecoveryService) Recover(ctx context.Context, documentID int64) (VaultRecoveryResult, error) {
	if service == nil || service.facts == nil || service.vault == nil || documentID <= 0 {
		return VaultRecoveryResult{}, sharedrepository.ErrInvalidInput
	}
	fact, err := service.facts.LoadVaultRebuildFact(ctx, documentID)
	if err != nil {
		return VaultRecoveryResult{}, classifyVaultRecoveryConflict(err)
	}
	if err := validateVaultRebuildFact(fact, documentID); err != nil {
		return VaultRecoveryResult{}, err
	}
	kind, key, err := documentPathParts(fact.Document)
	if err != nil {
		return VaultRecoveryResult{}, err
	}

	sources := domain.VaultRecoverySources{ExpectedHash: fact.Document.ContentHash}
	current, _, readErr := service.vault.Read(kind, key)
	missing := isMissing(readErr)
	if readErr != nil && !missing {
		return VaultRecoveryResult{}, readErr
	}
	if !missing {
		sources.Current = string(current)
	} else {
		sources.Revision, err = service.readRevision(ctx, fact.SnapshotObjectKey)
		if err != nil {
			return VaultRecoveryResult{}, err
		}
		if sources.Revision == "" {
			sources.Backup, err = service.readBackup(ctx, fact.Document)
			if err != nil {
				return VaultRecoveryResult{}, err
			}
		}
	}

	recovered, err := domain.RecoverVaultDocument(sources, fact.RenderInput)
	if err != nil {
		return VaultRecoveryResult{}, classifyVaultRecoveryConflict(err)
	}
	if domain.HashContent("", recovered.Content) != fact.Document.ContentHash {
		return VaultRecoveryResult{}, classifyVaultRecoveryConflict(fmt.Errorf("%w: recovered bytes do not match current projection", domain.ErrVaultConflict))
	}
	result := VaultRecoveryResult{
		DocumentID: fact.Document.ID, RevisionNo: fact.Document.RevisionNo,
		ContentHash: fact.Document.ContentHash, Source: recovered.Source,
	}
	if !missing {
		return result, nil
	}
	receipt, err := service.vault.CompareAndSwap(kind, key, domain.HashContent("", ""), recovered.Content)
	if err != nil {
		return VaultRecoveryResult{}, classifyVaultRecoveryConflict(err)
	}
	if receipt != fact.Document.ContentHash {
		return VaultRecoveryResult{}, sharedrepository.ErrUnavailable
	}
	result.Restored = true
	return result, nil
}

func classifyVaultRecoveryConflict(err error) error {
	if errors.Is(err, domain.ErrVaultConflict) || errors.Is(err, domain.ErrVaultHumanRegionUnavailable) {
		return fmt.Errorf("%w: %w", sharedrepository.ErrConflict, err)
	}
	return err
}

func (service *VaultRecoveryService) readRevision(ctx context.Context, objectKey string) (string, error) {
	if objectKey == "" || service.revision == nil {
		return "", nil
	}
	content, err := service.revision.ReadVaultSnapshot(ctx, objectKey, maximumVaultRecoveryBytes)
	if isMissing(err) {
		return "", nil
	}
	return content, err
}

func (service *VaultRecoveryService) readBackup(ctx context.Context, document domain.Document) (string, error) {
	if service.backup == nil {
		return "", nil
	}
	content, err := service.backup.ReadVaultBackup(ctx, document.ID, document.RevisionNo, maximumVaultRecoveryBytes)
	if isMissing(err) {
		return "", nil
	}
	return content, err
}

func validateVaultRebuildFact(fact VaultRebuildFact, documentID int64) error {
	if err := fact.Document.Validate(); err != nil || fact.Document.ID != documentID || len(fact.Document.ContentHash) != 64 {
		return sharedrepository.ErrInvalidInput
	}
	sourceID, err := documentSourceID(fact.Document)
	if err != nil || fact.RenderInput.DocumentID != fact.Document.ID || fact.RenderInput.RevisionNo != fact.Document.RevisionNo ||
		fact.RenderInput.Type != fact.Document.Type || fact.RenderInput.SourceID != sourceID {
		return sharedrepository.ErrInvalidInput
	}
	return nil
}
