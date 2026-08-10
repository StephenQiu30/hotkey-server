package postgres

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	operationsapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/application"
)

type evidenceLineageMigrationRunRecord struct {
	ID             int64
	Version        int64
	Phase          string
	Status         string
	BatchSize      int
	LastResourceID int64
	ExaminedCount  int64
	ReusedCount    int64
	CreatedCount   int64
	SkippedCount   int64
	BlockedCount   int64
	FailedCount    int64
	StartedAt      time.Time
	CompletedAt    *time.Time
}

func evidenceLineageMaintenanceRunDTOFromRecord(record evidenceLineageMigrationRunRecord) operationsapplication.EvidenceLineageMaintenanceRunDTO {
	return operationsapplication.EvidenceLineageMaintenanceRunDTO{
		RunID: record.ID, Status: record.Status, LastResourceID: record.LastResourceID,
		ExaminedCount: record.ExaminedCount, ReusedCount: record.ReusedCount,
		CreatedCount: record.CreatedCount, SkippedCount: record.SkippedCount,
		BlockedCount: record.BlockedCount, FailedCount: record.FailedCount,
	}
}

type evidenceLineageBackfillCandidateRecord struct {
	ID                 int64
	LegacyResourceType string
	FingerprintParts   []string
	Disposition        string
	TargetResourceType *string
	TargetResourceID   *int64
	ReasonCode         string
}

func (record evidenceLineageBackfillCandidateRecord) inputSHA256(phase string) (string, error) {
	if record.ID <= 0 || record.LegacyResourceType == "" || phase == "" {
		return "", fmt.Errorf("evidence lineage backfill candidate identity is invalid")
	}
	digest := sha256.New()
	for _, part := range append([]string{phase, record.LegacyResourceType, strconv.FormatInt(record.ID, 10)}, record.FingerprintParts...) {
		value := []byte(part)
		_, _ = digest.Write([]byte(strconv.Itoa(len(value))))
		_, _ = digest.Write([]byte{0})
		_, _ = digest.Write(value)
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func normalizedMaintenanceReason(value string) (string, error) {
	if value == "" || value != strings.TrimSpace(value) || len(value) > 64 {
		return "", fmt.Errorf("maintenance reason code is invalid")
	}
	for index, character := range value {
		if (character >= 'a' && character <= 'z') || (index > 0 && (character >= '0' && character <= '9' || character == '_')) {
			continue
		}
		return "", fmt.Errorf("maintenance reason code is invalid")
	}
	return value, nil
}
