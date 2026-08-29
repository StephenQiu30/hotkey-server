package application

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"time"
)

const BackupRunManifestVersion = "hotkey-backup-run-v1"

var backupDigestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
var backupGitRevisionPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

var backupAssetNames = map[string]bool{
	"postgres_facts": true, "minio_evidence": true, "vault_all_files": true,
	"vault_manual_regions": true, "river_jobs_attempts": true,
}

var backupFailureCodes = map[string]bool{
	"postgres_backup_failed": true, "minio_backup_failed": true, "vault_backup_failed": true,
	"manifest_incomplete": true, "integrity_check_failed": true, "backup_timeout": true,
	"unknown": true,
}

type backupRunManifest struct {
	Version         string                   `json:"version"`
	RunSHA256       string                   `json:"run_sha256"`
	GitRevision     string                   `json:"git_revision"`
	Status          string                   `json:"status"`
	RecoveryPointAt *time.Time               `json:"recovery_point_at"`
	StartedAt       time.Time                `json:"started_at"`
	CompletedAt     time.Time                `json:"completed_at"`
	FailureCode     *string                  `json:"failure_code"`
	Assets          []backupRunManifestAsset `json:"assets"`
}

type backupRunManifestAsset struct {
	Name   string `json:"name"`
	Count  int64  `json:"count"`
	SHA256 string `json:"sha256"`
}

type BackupRunCommand struct {
	RunSHA256       string
	ManifestSHA256  string
	GitRevision     string
	Status          string
	RecoveryPointAt *time.Time
	StartedAt       time.Time
	CompletedAt     time.Time
	FailureCode     string
	AssetCount      int64
}

type BackupRunReceiptDTO struct {
	RunID     int64
	RunSHA256 string
	Status    string
}

type BackupRunStore interface {
	RecordBackupRun(context.Context, BackupRunCommand) (BackupRunReceiptDTO, error)
}

type BackupRunService struct {
	store BackupRunStore
}

func NewBackupRunService(store BackupRunStore) (*BackupRunService, error) {
	if store == nil {
		return nil, errors.New("backup run store is required")
	}
	return &BackupRunService{store: store}, nil
}

func (service *BackupRunService) Record(ctx context.Context, payload []byte) (BackupRunReceiptDTO, error) {
	if service == nil || service.store == nil {
		return BackupRunReceiptDTO{}, errors.New("backup run service is not initialized")
	}
	command, err := decodeBackupRunManifest(payload)
	if err != nil {
		return BackupRunReceiptDTO{}, err
	}
	return service.store.RecordBackupRun(ctx, command)
}

func decodeBackupRunManifest(payload []byte) (BackupRunCommand, error) {
	if len(payload) == 0 || len(payload) > 64*1024 {
		return BackupRunCommand{}, errors.New("backup run manifest size is invalid")
	}
	var manifest backupRunManifest
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return BackupRunCommand{}, errors.New("decode strict backup run manifest")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return BackupRunCommand{}, errors.New("backup run manifest must contain exactly one JSON object")
	}
	if err := validateBackupRunManifest(manifest); err != nil {
		return BackupRunCommand{}, err
	}
	digest := sha256.Sum256(payload)
	failureCode := ""
	if manifest.FailureCode != nil {
		failureCode = *manifest.FailureCode
	}
	return BackupRunCommand{
		RunSHA256: manifest.RunSHA256, ManifestSHA256: hex.EncodeToString(digest[:]), GitRevision: manifest.GitRevision,
		Status: manifest.Status, RecoveryPointAt: manifest.RecoveryPointAt, StartedAt: manifest.StartedAt.UTC(),
		CompletedAt: manifest.CompletedAt.UTC(), FailureCode: failureCode, AssetCount: int64(len(manifest.Assets)),
	}, nil
}

func validateBackupRunManifest(manifest backupRunManifest) error {
	if manifest.Version != BackupRunManifestVersion || !backupDigestPattern.MatchString(manifest.RunSHA256) ||
		!backupGitRevisionPattern.MatchString(manifest.GitRevision) || manifest.StartedAt.IsZero() || manifest.CompletedAt.IsZero() ||
		manifest.CompletedAt.Before(manifest.StartedAt) || manifest.CompletedAt.After(time.Now().UTC().Add(5*time.Minute)) {
		return errors.New("backup run manifest identity or timeline is invalid")
	}
	seen := make(map[string]struct{}, len(manifest.Assets))
	for _, asset := range manifest.Assets {
		if !backupAssetNames[asset.Name] || asset.Count < 0 || !backupDigestPattern.MatchString(asset.SHA256) {
			return errors.New("backup run manifest asset is invalid")
		}
		if _, duplicate := seen[asset.Name]; duplicate {
			return errors.New("backup run manifest contains a duplicate asset")
		}
		seen[asset.Name] = struct{}{}
	}
	switch manifest.Status {
	case "succeeded":
		if manifest.FailureCode != nil || manifest.RecoveryPointAt == nil || manifest.RecoveryPointAt.IsZero() ||
			manifest.RecoveryPointAt.After(manifest.CompletedAt) || len(seen) != len(backupAssetNames) {
			return errors.New("successful backup run manifest is incomplete")
		}
		for name := range backupAssetNames {
			if _, found := seen[name]; !found {
				return errors.New("successful backup run manifest is missing a required asset")
			}
		}
	case "failed":
		if manifest.RecoveryPointAt != nil || manifest.FailureCode == nil || !backupFailureCodes[*manifest.FailureCode] {
			return errors.New("failed backup run manifest requires a bounded failure code")
		}
	default:
		return errors.New("backup run manifest status is invalid")
	}
	return nil
}
