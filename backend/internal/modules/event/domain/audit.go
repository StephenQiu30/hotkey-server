package domain

import (
	"fmt"
	"strings"
	"time"
)

type AuditAction string

const MaxReasonCodeLength = 64

const (
	AuditLifecycleTransition AuditAction = "lifecycle_transition"
	AuditMerge               AuditAction = "merge"
	AuditSplit               AuditAction = "split"
	AuditMemberMove          AuditAction = "member_move"
	AuditManualLock          AuditAction = "manual_lock"
	AuditManualUnlock        AuditAction = "manual_unlock"
	AuditEvidenceRecompute   AuditAction = "evidence_recompute"
	AuditDownstreamReconcile AuditAction = "downstream_reconcile_required"
	AuditDeduplicated        AuditAction = "deduplicated"
)

func (action AuditAction) Valid() bool {
	switch action {
	case AuditLifecycleTransition, AuditMerge, AuditSplit, AuditMemberMove, AuditManualLock, AuditManualUnlock, AuditEvidenceRecompute, AuditDownstreamReconcile, AuditDeduplicated:
		return true
	default:
		return false
	}
}

type GovernanceAudit struct {
	ID, EventID                  int64
	Action                       AuditAction
	ActorUserID                  *int64
	ReasonCode                   string
	FromStatus                   *LifecycleStatus
	ToStatus                     *LifecycleStatus
	SourceEventID, TargetEventID *int64
	ExpectedVersion              *int64
	Metadata                     map[string]any
	CreatedAt                    time.Time
}

func (audit GovernanceAudit) Validate() error {
	if audit.EventID <= 0 || !audit.Action.Valid() || !ValidReasonCode(audit.ReasonCode) || audit.CreatedAt.IsZero() {
		return fmt.Errorf("invalid event governance audit")
	}
	if audit.FromStatus != nil && !audit.FromStatus.Valid() || audit.ToStatus != nil && !audit.ToStatus.Valid() {
		return fmt.Errorf("invalid audit lifecycle status")
	}
	if audit.SourceEventID != nil && *audit.SourceEventID <= 0 || audit.TargetEventID != nil && *audit.TargetEventID <= 0 {
		return fmt.Errorf("invalid audit event reference")
	}
	if audit.SourceEventID != nil && audit.TargetEventID != nil && *audit.SourceEventID == *audit.TargetEventID {
		return fmt.Errorf("audit source and target must differ")
	}
	return nil
}

// ValidReasonCode keeps every governance entry within the persisted contract.
// Command validation uses the same rule so invalid input is a stable 400,
// never an audit insertion failure after a transaction has started.
func ValidReasonCode(value string) bool {
	return strings.TrimSpace(value) != "" && len(value) <= MaxReasonCodeLength
}
