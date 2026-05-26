package serviceboundary

import "testing"

func TestAPIAndWorkerRolesCanScaleIndependently(t *testing.T) {
	service := NewService()

	service.SetScale(ServiceAPI, 3)
	service.SetScale(ServiceWorker, 8)

	topology := service.Topology()
	if topology.Services[ServiceAPI].Replicas != 3 {
		t.Fatalf("api replicas = %d, want 3", topology.Services[ServiceAPI].Replicas)
	}
	if topology.Services[ServiceWorker].Replicas != 8 {
		t.Fatalf("worker replicas = %d, want 8", topology.Services[ServiceWorker].Replicas)
	}
	if !topology.Services[ServiceAPI].OpenAPISource {
		t.Fatalf("api service should remain OpenAPI source of truth")
	}
	if topology.Services[ServiceWorker].OpenAPISource {
		t.Fatalf("worker service must not become OpenAPI source of truth")
	}
}

func TestTaskMessageContractIsExplicit(t *testing.T) {
	service := NewService()

	contract := service.TaskMessageContract()
	for _, required := range []string{"id", "type", "tenantId", "priority", "payload", "maxAttempts"} {
		if !contract.RequiredFields[required] {
			t.Fatalf("task message contract missing required field %s: %#v", required, contract.RequiredFields)
		}
	}
	if contract.SchemaVersion == "" {
		t.Fatalf("schema version is empty")
	}
}
