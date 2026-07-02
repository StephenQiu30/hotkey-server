package knowledge

import (
	"context"
	"fmt"
	"log"
)

// AuditRecorder records writeback attempts.
type AuditRecorder interface {
	RecordAttempt(ctx context.Context, in RecordAttemptInput) error
}

// SidecarWriter writes whitelisted fields to sidecar tables.
type SidecarWriter interface {
	SetManualTags(ctx context.Context, objectID int64, tags []string) error
	SetAnalystConclusion(ctx context.Context, objectID int64, conclusion string) error
	SetMaterialStatus(ctx context.Context, topicID int64, status string) error
	SetThemeRef(ctx context.Context, objectType string, objectID int64, themeRef string) error
}

// RecordAttemptInput describes a writeback attempt to be recorded for audit.
type RecordAttemptInput struct {
	ObjectType     string
	ObjectID       int64
	FieldName      string
	FieldValue     interface{}
	Status         string
	ConflictReason string
	SourcePath     string
}

// Service orchestrates writeback validation, conflict detection, and application.
type Service struct {
	audit    AuditRecorder
	sidecar  SidecarWriter
}

// NewService creates a new writeback Service.
func NewService(audit AuditRecorder, sidecar SidecarWriter) *Service {
	return &Service{audit: audit, sidecar: sidecar}
}

// ApplyChange validates, checks conflicts, writes to sidecar, and records an audit log.
func (s *Service) ApplyChange(ctx context.Context, change WritebackChange, conflict ConflictInput) error {
	// 1. Validate field is in whitelist.
	if err := ValidateWriteback(change); err != nil {
		s.recordFailed(ctx, change, "rejected", err.Error())
		return fmt.Errorf("validate: %w", err)
	}

	// 2. Check for revision conflict.
	if err := DetectConflict(conflict); err != nil {
		s.recordFailed(ctx, change, "conflicted", err.Error())
		return fmt.Errorf("conflict: %w", err)
	}

	// 3. Apply to the correct sidecar table based on field name.
	if err := s.applyToSidecar(ctx, change); err != nil {
		s.recordFailed(ctx, change, "rejected", err.Error())
		return fmt.Errorf("apply sidecar: %w", err)
	}

	// 4. Record success.
	s.recordSuccess(ctx, change)
	return nil
}

func (s *Service) applyToSidecar(ctx context.Context, change WritebackChange) error {
	switch change.FieldName {
	case "manual_tags":
		tags, ok := change.Value.([]string)
		if !ok {
			return fmt.Errorf("manual_tags must be a string array")
		}
		return s.sidecar.SetManualTags(ctx, change.ObjectID, tags)
	case "analyst_conclusion":
		val, ok := change.Value.(string)
		if !ok {
			return fmt.Errorf("analyst_conclusion must be a string")
		}
		return s.sidecar.SetAnalystConclusion(ctx, change.ObjectID, val)
	case "material_status":
		val, ok := change.Value.(string)
		if !ok {
			return fmt.Errorf("material_status must be a string")
		}
		return s.sidecar.SetMaterialStatus(ctx, change.ObjectID, val)
	case "theme_ref":
		val, ok := change.Value.(string)
		if !ok {
			return fmt.Errorf("theme_ref must be a string")
		}
		return s.sidecar.SetThemeRef(ctx, change.ObjectType, change.ObjectID, val)
	default:
		return fmt.Errorf("unsupported field: %s", change.FieldName)
	}
}

func (s *Service) recordFailed(ctx context.Context, change WritebackChange, status, reason string) {
	if err := s.audit.RecordAttempt(ctx, RecordAttemptInput{
		ObjectType:     change.ObjectType,
		ObjectID:       change.ObjectID,
		FieldName:      change.FieldName,
		FieldValue:     change.Value,
		Status:         status,
		ConflictReason: reason,
		SourcePath:     change.SourcePath,
	}); err != nil {
		log.Printf("writeback audit: record %s for %s/%s on %q: %v",
			status, change.ObjectType, change.SourcePath, change.FieldName, err)
	}
}

func (s *Service) recordSuccess(ctx context.Context, change WritebackChange) {
	if err := s.audit.RecordAttempt(ctx, RecordAttemptInput{
		ObjectType: change.ObjectType,
		ObjectID:   change.ObjectID,
		FieldName:  change.FieldName,
		FieldValue: change.Value,
		Status:     "applied",
		SourcePath: change.SourcePath,
	}); err != nil {
		log.Printf("writeback audit: record applied for %s/%s on %q: %v",
			change.ObjectType, change.SourcePath, change.FieldName, err)
	}
}
