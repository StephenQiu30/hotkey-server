// Package domain defines the deliberately narrow audit vocabulary shared by
// new business modules. Identity keeps its legacy audit implementation private.
package domain

import (
	"fmt"
	"strings"
)

type AuditAction string
type AuditResult string

const (
	ActionMonitorCreated             AuditAction = "monitor.created"
	ActionMonitorDraftUpdated        AuditAction = "monitor.draft_updated"
	ActionMonitorAICandidateCreated  AuditAction = "monitor.ai_candidate_created"
	ActionMonitorAICandidateApproved AuditAction = "monitor.ai_candidate_approved"
	ActionMonitorAICandidateRejected AuditAction = "monitor.ai_candidate_rejected"
	ActionMonitorPublished           AuditAction = "monitor.published"
	ActionMonitorPaused              AuditAction = "monitor.paused"
	ActionMonitorResumed             AuditAction = "monitor.resumed"
	ActionMonitorArchived            AuditAction = "monitor.archived"
	ActionMonitorRestored            AuditAction = "monitor.restored"
	ActionSourceCreated              AuditAction = "source.created"
	ActionSourceUpdated              AuditAction = "source.updated"
	ActionSourceEnabled              AuditAction = "source.enabled"
	ActionSourceDisabled             AuditAction = "source.disabled"
	ActionSourceArchived             AuditAction = "source.archived"
	ActionSourceRestored             AuditAction = "source.restored"

	AuditResultSuccess AuditResult = "success"
	AuditResultFailure AuditResult = "failure"
	AuditResultDenied  AuditResult = "denied"
)

var allowedActions = map[AuditAction]struct{}{
	ActionMonitorCreated: {}, ActionMonitorDraftUpdated: {}, ActionMonitorAICandidateCreated: {}, ActionMonitorAICandidateApproved: {}, ActionMonitorAICandidateRejected: {},
	ActionMonitorPublished: {}, ActionMonitorPaused: {}, ActionMonitorResumed: {}, ActionMonitorArchived: {}, ActionMonitorRestored: {},
	ActionSourceCreated: {}, ActionSourceUpdated: {}, ActionSourceEnabled: {}, ActionSourceDisabled: {}, ActionSourceArchived: {}, ActionSourceRestored: {},
}

// AuditEntry intentionally contains no free-form payload field. Before and
// After may only use the small safe-metadata whitelist validated below.
type AuditEntry struct {
	ActorType    string
	ActorID      int64
	Action       AuditAction
	ResourceType string
	ResourceID   int64
	RequestID    string
	TraceID      string
	Before       map[string]any
	After        map[string]any
	Result       AuditResult
	IPHash       string
}

func (action AuditAction) Valid() bool {
	_, ok := allowedActions[action]
	return ok
}

func (result AuditResult) Valid() bool {
	return result == AuditResultSuccess || result == AuditResultFailure || result == AuditResultDenied
}

func (entry AuditEntry) Validate() error {
	if strings.TrimSpace(entry.ActorType) == "" || strings.TrimSpace(entry.ResourceType) == "" {
		return fmt.Errorf("audit actor type and resource type are required")
	}
	if !entry.Action.Valid() {
		return fmt.Errorf("audit action %q is not allowed", entry.Action)
	}
	if !entry.Result.Valid() {
		return fmt.Errorf("audit result %q is invalid", entry.Result)
	}
	if err := ValidateMetadata(entry.Before); err != nil {
		return fmt.Errorf("invalid before audit metadata: %w", err)
	}
	if err := ValidateMetadata(entry.After); err != nil {
		return fmt.Errorf("invalid after audit metadata: %w", err)
	}
	return nil
}

var safeMetadataKeys = map[string]struct{}{
	"monitor_version": {}, "draft_version": {}, "source_version": {}, "config_version": {}, "revision": {}, "rule_count": {}, "source_count": {},
	"status": {}, "previous_status": {}, "approval_status": {}, "config_hash": {}, "published_at": {},
	"enabled": {}, "deleted": {}, "credential_configured": {},
}

// ValidateMetadata rejects rather than silently redacts unknown data. This
// makes accidental endpoint/config/rule/credential leakage visible in tests
// and prevents audit rows from becoming a secret side channel.
func ValidateMetadata(metadata map[string]any) error {
	for key, value := range metadata {
		normalized := strings.ToLower(strings.TrimSpace(key))
		if _, ok := safeMetadataKeys[normalized]; !ok {
			if sensitiveKey(normalized) {
				return fmt.Errorf("sensitive audit metadata key %q", key)
			}
			return fmt.Errorf("audit metadata key %q is not allowed", key)
		}
		if !validMetadataValue(normalized, value) {
			return fmt.Errorf("audit metadata value for %q is invalid", key)
		}
	}
	return nil
}

// SanitizeMetadata is intended for observational callers that need a safe
// best-effort projection. Business audit writes must still call Validate via
// AuditEntry.Validate and therefore fail closed on unsafe caller input.
func SanitizeMetadata(metadata map[string]any) map[string]any {
	if len(metadata) == 0 {
		return nil
	}
	clean := make(map[string]any, len(metadata))
	for key, value := range metadata {
		normalized := strings.ToLower(strings.TrimSpace(key))
		if _, ok := safeMetadataKeys[normalized]; ok && validMetadataValue(normalized, value) {
			clean[normalized] = value
		}
	}
	return clean
}

func validMetadataValue(key string, value any) bool {
	switch key {
	case "monitor_version", "draft_version", "source_version", "config_version", "revision", "rule_count", "source_count":
		switch value.(type) {
		case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
			return true
		default:
			return false
		}
	case "enabled", "deleted", "credential_configured":
		_, ok := value.(bool)
		return ok
	default:
		_, ok := value.(string)
		return ok
	}
}

func sensitiveKey(key string) bool {
	for _, fragment := range []string{"endpoint", "credential", "secret", "password", "authorization", "token", "config", "rule", "raw", "uri", "url"} {
		if strings.Contains(key, fragment) {
			return true
		}
	}
	return false
}
