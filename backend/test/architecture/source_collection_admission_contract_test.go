package architecture_test

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestSourceCollectionAdmissionPrecedesCredentialsBudgetAndNetwork(t *testing.T) {
	repository := filepath.Clean(filepath.Join(repositoryRoot(t), ".."))
	required := map[string][]string{
		"backend/internal/modules/source/application/collection_service.go": {
			"Admission     CollectionAdmissionAuthorizer",
			"service.admission.AuthorizeCollection(ctx, *connection)",
		},
		"backend/internal/bootstrap/app.go": {
			"Rights     *sourcepostgres.RightsDecisionReader",
			"sourceapplication.NewCollectionAdmissionGate",
			"Admission: admission",
		},
		"backend/internal/modules/source/infrastructure/connector_registry.go": {
			"xconnector.NewWithManagedCredentialLookup",
		},
		"backend/test/_suite/internal/modules/source/application/collection_ssrf_integration_test.go": {
			"TestCollectionFetchRightsDenialStopsBeforeConnectorResolutionAndPersistsSanitizedAudit",
			`{"reason_code": "fetch_rights_not_permitted"}`,
		},
		"backend/test/_suite/internal/modules/source/infrastructure/postgres/rights_decision_reader_integration_test.go": {
			"TestRightsDecisionReaderResolvesCurrentEndpointFetchConservatively",
		},
		"backend/test/_suite/internal/modules/source/infrastructure/connector_registry_test.go": {
			"TestConnectorRegistryDefersManagedCredentialDecryptionUntilTheRequestBoundary",
		},
	}
	for relative, fragments := range required {
		content := readRepositoryFile(t, repository, relative)
		for _, fragment := range fragments {
			if !strings.Contains(content, fragment) {
				t.Errorf("source collection admission contract %s is missing %q", relative, fragment)
			}
		}
	}

	service := readRepositoryFile(t, repository, "backend/internal/modules/source/application/collection_service.go")
	assertSourceOrder(t, service, "func (service *CollectionService) execute", "func filterCollectionItems", "service.admission.AuthorizeCollection", "service.connectors.Resolve")

	xConnector := readRepositoryFile(t, repository, "backend/internal/modules/source/infrastructure/x/connector.go")
	assertSourceOrder(t, xConnector, "func (connector *Connector) Fetch", "func (connector *Connector) LookupPostMetrics", "connector.validateEndpointPolicy", "connector.token(ctx)")
	assertSourceOrder(t, xConnector, "func (connector *Connector) LookupPostMetrics", "func (connector *Connector) Health", "connector.validateEndpointPolicy", "connector.token(ctx)")
	assertSourceOrder(t, xConnector, "func (connector *Connector) Health", "func (connector *Connector) token", "connector.validateEndpointPolicy", "connector.token(ctx)")
}

func assertSourceOrder(t *testing.T, source, startMarker, endMarker, before, after string) {
	t.Helper()
	start := strings.Index(source, startMarker)
	if start < 0 {
		t.Fatalf("source contract start marker is missing: %q", startMarker)
	}
	end := strings.Index(source[start+len(startMarker):], endMarker)
	if end < 0 {
		t.Fatalf("source contract end marker is missing after %q: %q", startMarker, endMarker)
	}
	section := source[start : start+len(startMarker)+end]
	beforeIndex := strings.Index(section, before)
	afterIndex := strings.Index(section, after)
	if beforeIndex < 0 || afterIndex < 0 || beforeIndex >= afterIndex {
		t.Fatalf("%q must occur before %q inside %q", before, after, startMarker)
	}
}
