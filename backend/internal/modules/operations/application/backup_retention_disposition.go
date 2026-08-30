package application

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"time"
)

const BackupRetentionDispositionManifestVersion = "hotkey-backup-retention-disposition-v1"

var backupRetentionDispositionReasons = map[string]bool{
	"retention_expired": true,
	"rights_revoked":    true,
}

type backupRetentionDispositionManifest struct {
	Version                string    `json:"version"`
	DispositionSHA256      string    `json:"disposition_sha256"`
	BackupRunSHA256        string    `json:"backup_run_sha256"`
	DeletionEvidenceSHA256 string    `json:"deletion_evidence_sha256"`
	ReasonCode             string    `json:"reason_code"`
	OperatorID             string    `json:"operator_record_id"`
	ReviewerID             string    `json:"reviewer_record_id"`
	DisposedAt             time.Time `json:"disposed_at"`
}

type BackupRetentionDispositionCommand struct {
	DispositionSHA256      string
	ManifestSHA256         string
	BackupRunSHA256        string
	DeletionEvidenceSHA256 string
	ReasonCode             string
	OperatorID             string
	ReviewerID             string
	DisposedAt             time.Time
}

type BackupRetentionDispositionReceiptDTO struct {
	DispositionID   int64
	BackupRunSHA256 string
	Status          string
}

type BackupRetentionDispositionStore interface {
	RecordBackupRetentionDisposition(context.Context, BackupRetentionDispositionCommand) (BackupRetentionDispositionReceiptDTO, error)
}

type BackupRetentionDispositionService struct {
	store BackupRetentionDispositionStore
}

func NewBackupRetentionDispositionService(store BackupRetentionDispositionStore) (*BackupRetentionDispositionService, error) {
	if store == nil {
		return nil, errors.New("backup retention disposition store is required")
	}
	return &BackupRetentionDispositionService{store: store}, nil
}

func (service *BackupRetentionDispositionService) Record(ctx context.Context, payload []byte) (BackupRetentionDispositionReceiptDTO, error) {
	if service == nil || service.store == nil {
		return BackupRetentionDispositionReceiptDTO{}, errors.New("backup retention disposition service is not initialized")
	}
	command, err := decodeBackupRetentionDispositionManifest(payload)
	if err != nil {
		return BackupRetentionDispositionReceiptDTO{}, err
	}
	return service.store.RecordBackupRetentionDisposition(ctx, command)
}

func decodeBackupRetentionDispositionManifest(payload []byte) (BackupRetentionDispositionCommand, error) {
	if len(payload) == 0 || len(payload) > 64*1024 {
		return BackupRetentionDispositionCommand{}, errors.New("backup retention disposition manifest size is invalid")
	}
	var manifest backupRetentionDispositionManifest
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return BackupRetentionDispositionCommand{}, errors.New("decode strict backup retention disposition manifest")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return BackupRetentionDispositionCommand{}, errors.New("backup retention disposition manifest must contain exactly one JSON object")
	}
	if err := validateBackupRetentionDispositionManifest(manifest); err != nil {
		return BackupRetentionDispositionCommand{}, err
	}
	digest := sha256.Sum256(payload)
	return BackupRetentionDispositionCommand{
		DispositionSHA256: manifest.DispositionSHA256, ManifestSHA256: hex.EncodeToString(digest[:]),
		BackupRunSHA256: manifest.BackupRunSHA256, DeletionEvidenceSHA256: manifest.DeletionEvidenceSHA256,
		ReasonCode: manifest.ReasonCode, OperatorID: manifest.OperatorID, ReviewerID: manifest.ReviewerID,
		DisposedAt: manifest.DisposedAt.UTC(),
	}, nil
}

func validateBackupRetentionDispositionManifest(manifest backupRetentionDispositionManifest) error {
	if manifest.Version != BackupRetentionDispositionManifestVersion ||
		!maintenanceSHA256Pattern.MatchString(manifest.DispositionSHA256) ||
		!maintenanceSHA256Pattern.MatchString(manifest.BackupRunSHA256) ||
		!maintenanceSHA256Pattern.MatchString(manifest.DeletionEvidenceSHA256) ||
		!backupRetentionDispositionReasons[manifest.ReasonCode] ||
		!validMaintenanceIdentity(manifest.OperatorID) || !validMaintenanceIdentity(manifest.ReviewerID) ||
		manifest.OperatorID == manifest.ReviewerID || manifest.DisposedAt.IsZero() ||
		manifest.DisposedAt.After(time.Now().UTC().Add(5*time.Minute)) {
		return errors.New("backup retention disposition manifest is invalid")
	}
	return nil
}
