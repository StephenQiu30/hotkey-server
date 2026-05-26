package rbac

import "testing"

func TestRolePermissionsEnforceAdminBoundaries(t *testing.T) {
	service := NewService()

	service.GrantRole(RoleGrantInput{TenantID: "tenant-alpha", UserID: "owner-1", Role: RoleOwner})
	service.GrantRole(RoleGrantInput{TenantID: "tenant-alpha", UserID: "admin-1", Role: RoleAdmin})
	service.GrantRole(RoleGrantInput{TenantID: "tenant-alpha", UserID: "viewer-1", Role: RoleViewer})

	if !service.Can(AuthorizeInput{TenantID: "tenant-alpha", UserID: "owner-1", Action: ActionManageSources}) {
		t.Fatalf("owner should manage sources")
	}
	if !service.Can(AuthorizeInput{TenantID: "tenant-alpha", UserID: "admin-1", Action: ActionManageKeywords}) {
		t.Fatalf("admin should manage keywords")
	}
	if service.Can(AuthorizeInput{TenantID: "tenant-alpha", UserID: "viewer-1", Action: ActionManageSources}) {
		t.Fatalf("viewer should not manage sources")
	}
	if !service.Can(AuthorizeInput{TenantID: "tenant-alpha", UserID: "viewer-1", Action: ActionViewReports}) {
		t.Fatalf("viewer should view reports")
	}
	if service.Can(AuthorizeInput{TenantID: "tenant-beta", UserID: "admin-1", Action: ActionManageKeywords}) {
		t.Fatalf("role should not cross tenant boundary")
	}
}

func TestCriticalConfigurationChangesAreAudited(t *testing.T) {
	service := NewService()

	service.GrantRole(RoleGrantInput{TenantID: "tenant-alpha", UserID: "admin-1", Role: RoleAdmin})
	event := service.RecordAuditEvent(AuditEventInput{
		TenantID:     "tenant-alpha",
		ActorUserID:  "admin-1",
		Action:       ActionManageSources,
		ResourceType: "source",
		ResourceID:   "arxiv-ai",
		Status:       AuditStatusSucceeded,
		Message:      "disabled source",
	})
	service.RecordAuditEvent(AuditEventInput{
		TenantID:     "tenant-beta",
		ActorUserID:  "admin-2",
		Action:       ActionManageSources,
		ResourceType: "source",
		ResourceID:   "github-trending-ai",
		Status:       AuditStatusSucceeded,
	})

	events := service.ListAuditEvents("tenant-alpha")
	if len(events) != 2 {
		t.Fatalf("tenant-alpha audit events len = %d, want 2: %#v", len(events), events)
	}
	if events[0].ID != event.ID {
		t.Fatalf("newest audit event not first: %#v", events)
	}
	for _, auditEvent := range events {
		if auditEvent.TenantID != "tenant-alpha" {
			t.Fatalf("cross-tenant audit event leaked: %#v", auditEvent)
		}
	}
}
