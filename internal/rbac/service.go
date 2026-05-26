package rbac

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	RoleOwner  = "owner"
	RoleAdmin  = "admin"
	RoleViewer = "viewer"

	ActionManageKeywords = "manage_keywords"
	ActionManageSources  = "manage_sources"
	ActionViewReports    = "view_reports"
	ActionManageRoles    = "manage_roles"

	AuditStatusSucceeded = "succeeded"
	AuditStatusDenied    = "denied"
)

type RoleGrantInput struct {
	TenantID string
	UserID   string
	Role     string
}

type AuthorizeInput struct {
	TenantID string
	UserID   string
	Action   string
}

type AuditEventInput struct {
	TenantID     string
	ActorUserID  string
	Action       string
	ResourceType string
	ResourceID   string
	Status       string
	Message      string
}

type RoleBinding struct {
	TenantID string `json:"tenantId"`
	UserID   string `json:"userId"`
	Role     string `json:"role"`
}

type AuditEvent struct {
	ID           string    `json:"id"`
	TenantID     string    `json:"tenantId"`
	ActorUserID  string    `json:"actorUserId"`
	Action       string    `json:"action"`
	ResourceType string    `json:"resourceType"`
	ResourceID   string    `json:"resourceId"`
	Status       string    `json:"status"`
	Message      string    `json:"message,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
}

type Service struct {
	mu        sync.Mutex
	nextAudit int
	roles     map[string]RoleBinding
	audit     []AuditEvent
}

func NewService() *Service {
	return &Service{
		nextAudit: 1,
		roles:     make(map[string]RoleBinding),
	}
}

func (s *Service) GrantRole(input RoleGrantInput) RoleBinding {
	binding := RoleBinding{
		TenantID: strings.TrimSpace(input.TenantID),
		UserID:   strings.TrimSpace(input.UserID),
		Role:     normalizeRole(input.Role),
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.roles[roleKey(binding.TenantID, binding.UserID)] = binding
	s.recordAuditLocked(AuditEventInput{
		TenantID:     binding.TenantID,
		ActorUserID:  binding.UserID,
		Action:       ActionManageRoles,
		ResourceType: "role",
		ResourceID:   binding.UserID,
		Status:       AuditStatusSucceeded,
		Message:      "role granted: " + binding.Role,
	})
	return binding
}

func (s *Service) Can(input AuthorizeInput) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	binding, ok := s.roles[roleKey(input.TenantID, input.UserID)]
	if !ok {
		return false
	}
	return roleAllows(binding.Role, strings.TrimSpace(input.Action))
}

func (s *Service) RecordAuditEvent(input AuditEventInput) AuditEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.recordAuditLocked(input)
}

func (s *Service) ListAuditEvents(tenantID string) []AuditEvent {
	tenantID = strings.TrimSpace(tenantID)
	s.mu.Lock()
	defer s.mu.Unlock()

	events := make([]AuditEvent, 0)
	for i := len(s.audit) - 1; i >= 0; i-- {
		event := s.audit[i]
		if event.TenantID != tenantID {
			continue
		}
		events = append(events, event)
	}
	return events
}

func (s *Service) recordAuditLocked(input AuditEventInput) AuditEvent {
	event := AuditEvent{
		ID:           fmt.Sprintf("audit_%d", s.nextAudit),
		TenantID:     strings.TrimSpace(input.TenantID),
		ActorUserID:  strings.TrimSpace(input.ActorUserID),
		Action:       strings.TrimSpace(input.Action),
		ResourceType: strings.TrimSpace(input.ResourceType),
		ResourceID:   strings.TrimSpace(input.ResourceID),
		Status:       normalizeStatus(input.Status),
		Message:      strings.TrimSpace(input.Message),
		CreatedAt:    time.Now().UTC(),
	}
	s.nextAudit++
	s.audit = append(s.audit, event)
	return event
}

func normalizeRole(role string) string {
	switch strings.TrimSpace(role) {
	case RoleOwner:
		return RoleOwner
	case RoleAdmin:
		return RoleAdmin
	default:
		return RoleViewer
	}
}

func normalizeStatus(status string) string {
	switch strings.TrimSpace(status) {
	case AuditStatusDenied:
		return AuditStatusDenied
	default:
		return AuditStatusSucceeded
	}
}

func roleAllows(role string, action string) bool {
	permissions := map[string][]string{
		RoleOwner:  {ActionManageKeywords, ActionManageSources, ActionViewReports, ActionManageRoles},
		RoleAdmin:  {ActionManageKeywords, ActionManageSources, ActionViewReports},
		RoleViewer: {ActionViewReports},
	}
	allowed := permissions[normalizeRole(role)]
	return sort.SearchStrings(sortedActions(allowed), action) < len(allowed) && containsAction(allowed, action)
}

func sortedActions(actions []string) []string {
	result := append([]string(nil), actions...)
	sort.Strings(result)
	return result
}

func containsAction(actions []string, action string) bool {
	for _, candidate := range actions {
		if candidate == action {
			return true
		}
	}
	return false
}

func roleKey(tenantID string, userID string) string {
	return strings.TrimSpace(tenantID) + ":" + strings.TrimSpace(userID)
}
