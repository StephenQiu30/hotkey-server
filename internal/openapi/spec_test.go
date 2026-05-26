package openapi

import (
	"encoding/json"
	"testing"
)

func TestSpecContainsFoundationEndpoints(t *testing.T) {
	spec := Spec()

	if spec.OpenAPI != "3.1.0" {
		t.Fatalf("OpenAPI = %q, want 3.1.0", spec.OpenAPI)
	}
	if spec.Info.Title != "HotKey Server API" {
		t.Fatalf("title = %q, want HotKey Server API", spec.Info.Title)
	}
	if _, ok := spec.Paths["/healthz"]; !ok {
		t.Fatalf("paths missing /healthz")
	}
	if _, ok := spec.Paths["/openapi.json"]; !ok {
		t.Fatalf("paths missing /openapi.json")
	}
}

func TestSpecContainsKeywordEndpoints(t *testing.T) {
	spec := Spec()

	for _, path := range []string{
		"/api/v1/admin/keywords",
		"/api/v1/admin/keywords/{id}",
		"/api/v1/keywords/follow",
		"/api/v1/keywords/block",
		"/api/v1/keywords/additional",
		"/api/v1/keywords/preferences",
	} {
		if _, ok := spec.Paths[path]; !ok {
			t.Fatalf("paths missing %s", path)
		}
	}
}

func TestSpecContainsSourceEndpoints(t *testing.T) {
	spec := Spec()

	for _, path := range []string{
		"/api/v1/admin/sources",
		"/api/v1/admin/sources/{id}",
	} {
		if _, ok := spec.Paths[path]; !ok {
			t.Fatalf("paths missing %s", path)
		}
	}
}

func TestSpecContainsSourceItemEndpoints(t *testing.T) {
	spec := Spec()

	if _, ok := spec.Paths["/api/v1/admin/source-items"]; !ok {
		t.Fatalf("paths missing /api/v1/admin/source-items")
	}
}

func TestSpecContainsEventClusterEndpoints(t *testing.T) {
	spec := Spec()

	for _, path := range []string{
		"/api/v1/admin/event-candidates",
		"/api/v1/admin/event-clusters",
		"/api/v1/realtime/events",
		"/api/v1/admin/event-graph/events",
		"/api/v1/admin/event-graph/relations",
		"/api/v1/events/{id}/graph",
		"/api/v1/admin/events/{id}/propagation",
		"/api/v1/events/{id}/propagation",
		"/api/v1/admin/events/{id}/claims",
		"/api/v1/events/{id}/arbitration",
	} {
		if _, ok := spec.Paths[path]; !ok {
			t.Fatalf("paths missing %s", path)
		}
	}
}

func TestSpecContainsEventEvidenceEndpoints(t *testing.T) {
	spec := Spec()

	for _, path := range []string{
		"/api/v1/admin/event-evidence",
		"/api/v1/admin/events/{id}/ai-summary",
		"/api/v1/events/{id}/evidence",
	} {
		if _, ok := spec.Paths[path]; !ok {
			t.Fatalf("paths missing %s", path)
		}
	}
}

func TestSpecContainsHotspotEndpoints(t *testing.T) {
	spec := Spec()

	for _, path := range []string{
		"/api/v1/hotspots",
		"/api/v1/hotspots/{id}",
	} {
		if _, ok := spec.Paths[path]; !ok {
			t.Fatalf("paths missing %s", path)
		}
	}
}

func TestSpecContainsReportEndpoints(t *testing.T) {
	spec := Spec()

	for _, path := range []string{
		"/api/v1/reports/daily",
		"/api/v1/users/{id}/reports/daily",
	} {
		if _, ok := spec.Paths[path]; !ok {
			t.Fatalf("paths missing %s", path)
		}
	}
}

func TestSpecContainsRedisInfraEndpoints(t *testing.T) {
	spec := Spec()

	for _, path := range []string{
		"/api/v1/refresh-queue",
		"/api/v1/admin/refresh-queue",
		"/api/v1/admin/redis/health",
	} {
		if _, ok := spec.Paths[path]; !ok {
			t.Fatalf("paths missing %s", path)
		}
	}
}

func TestSpecContainsMiniappClientContract(t *testing.T) {
	spec := Spec()

	for _, path := range []string{
		"/api/v1/hotspots",
		"/api/v1/hotspots/{id}",
		"/api/v1/keywords/preferences",
		"/api/v1/reports/daily",
		"/api/v1/users/{id}/reports/daily",
		"/api/v1/refresh-queue",
	} {
		if _, ok := spec.Paths[path]; !ok {
			t.Fatalf("miniapp contract missing %s", path)
		}
	}
	if spec.Components.SecuritySchemes["BearerAuth"].Type != "http" {
		t.Fatalf("BearerAuth security scheme missing")
	}
	if len(spec.Security) == 0 || spec.Security[0]["BearerAuth"] == nil {
		t.Fatalf("global BearerAuth requirement missing: %#v", spec.Security)
	}
	if _, ok := spec.Paths["/api/v1/hotspots"].Get.Responses["401"]; !ok {
		t.Fatalf("hotspot list missing 401 response")
	}

	encoded, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("marshal OpenAPI spec: %v", err)
	}
	if !json.Valid(encoded) {
		t.Fatalf("OpenAPI spec JSON is invalid")
	}
}

func TestSpecContainsAdminAPIContract(t *testing.T) {
	spec := Spec()

	for _, path := range []string{
		"/api/v1/admin/keywords",
		"/api/v1/admin/keywords/{id}",
		"/api/v1/admin/sources",
		"/api/v1/admin/sources/{id}",
		"/api/v1/admin/task-runs",
		"/api/v1/admin/reports/daily",
		"/api/v1/admin/event-clusters",
		"/api/v1/admin/event-evidence",
	} {
		if _, ok := spec.Paths[path]; !ok {
			t.Fatalf("admin contract missing %s", path)
		}
	}
	if _, ok := spec.Paths["/api/v1/admin/reports/daily"].Post.Responses["202"]; !ok {
		t.Fatalf("admin daily report trigger missing 202 response")
	}
	if _, ok := spec.Paths["/api/v1/admin/task-runs"].Get.Responses["401"]; !ok {
		t.Fatalf("admin task runs missing 401 response")
	}
}

func TestSpecContainsTenantContract(t *testing.T) {
	spec := Spec()

	for _, path := range []string{
		"/api/v1/admin/tenants",
		"/api/v1/admin/tenants/{id}/members",
		"/api/v1/users/{id}/tenants",
		"/api/v1/tenants/{id}/reports/daily",
	} {
		if _, ok := spec.Paths[path]; !ok {
			t.Fatalf("tenant contract missing %s", path)
		}
	}
	if _, ok := spec.Paths["/api/v1/admin/tenants"].Post.Responses["201"]; !ok {
		t.Fatalf("tenant create missing 201 response")
	}
	if _, ok := spec.Paths["/api/v1/users/{id}/tenants"].Get.Responses["401"]; !ok {
		t.Fatalf("user tenant list missing 401 response")
	}
}

func TestSpecContainsRBACAuditContract(t *testing.T) {
	spec := Spec()

	for _, path := range []string{
		"/api/v1/admin/tenants/{id}/roles",
		"/api/v1/admin/tenants/{id}/authorize",
		"/api/v1/admin/tenants/{id}/audit-logs",
	} {
		if _, ok := spec.Paths[path]; !ok {
			t.Fatalf("rbac contract missing %s", path)
		}
	}
	if _, ok := spec.Paths["/api/v1/admin/tenants/{id}/roles"].Post.Responses["201"]; !ok {
		t.Fatalf("role grant missing 201 response")
	}
	if _, ok := spec.Paths["/api/v1/admin/tenants/{id}/audit-logs"].Get.Responses["401"]; !ok {
		t.Fatalf("audit logs missing 401 response")
	}
}

func TestSpecContainsTenantAdminExtensionContract(t *testing.T) {
	spec := Spec()

	for _, path := range []string{
		"/api/v1/admin/tenants/{id}/keywords",
		"/api/v1/admin/tenants/{id}/sources",
		"/api/v1/admin/tenants/{id}/sources/{sourceId}",
	} {
		if _, ok := spec.Paths[path]; !ok {
			t.Fatalf("tenant admin extension missing %s", path)
		}
	}
	if _, ok := spec.Paths["/api/v1/admin/tenants"].Get.Responses["200"]; !ok {
		t.Fatalf("platform tenant list missing 200 response")
	}
	if _, ok := spec.Paths["/api/v1/admin/tenants/{id}/keywords"].Post.Responses["201"]; !ok {
		t.Fatalf("tenant keyword create missing 201 response")
	}
}

func TestSpecContainsBillingContract(t *testing.T) {
	spec := Spec()

	for _, path := range []string{
		"/api/v1/admin/tenants/{id}/billing/plan",
		"/api/v1/admin/tenants/{id}/billing/usage",
	} {
		if _, ok := spec.Paths[path]; !ok {
			t.Fatalf("billing contract missing %s", path)
		}
	}
	if _, ok := spec.Paths["/api/v1/admin/tenants/{id}/billing/usage"].Post.Responses["402"]; !ok {
		t.Fatalf("billing usage missing quota exceeded response")
	}
}

func TestSpecContainsWorkQueueContract(t *testing.T) {
	spec := Spec()

	for _, path := range []string{
		"/api/v1/admin/work-queue/jobs",
		"/api/v1/admin/work-queue/run",
		"/api/v1/admin/work-queue/compensations",
	} {
		if _, ok := spec.Paths[path]; !ok {
			t.Fatalf("work queue contract missing %s", path)
		}
	}
	if _, ok := spec.Paths["/api/v1/admin/work-queue/jobs"].Post.Responses["201"]; !ok {
		t.Fatalf("work queue enqueue missing 201 response")
	}
}

func TestSpecContainsServiceBoundaryContract(t *testing.T) {
	spec := Spec()

	if _, ok := spec.Paths["/api/v1/admin/service-boundaries"]; !ok {
		t.Fatalf("service boundary contract missing")
	}
	if _, ok := spec.Paths["/api/v1/admin/service-boundaries"].Get.Responses["200"]; !ok {
		t.Fatalf("service boundary response missing 200")
	}
}
