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
		"backend/internal/modules/source/application/collection_admission.go": {
			"CollectionProbeAuthorizer",
			"AuthorizeProbe",
			"ManagedCredentialStatusReader",
			"gate.credentials.ManagedCredentialAvailable",
			"CollectionRequestBudgetStatusReader",
			"gate.budget.CollectionRequestAvailable",
		},
		"backend/internal/modules/source/application/x_metric_refresh.go": {
			"service.admission.AuthorizeCollection(ctx, *connection)",
		},
		"backend/internal/modules/source/application/instant_search.go": {
			"service.admission.AuthorizeCollection(ctx, connection)",
		},
		"backend/internal/modules/source/application/collection_control.go": {
			"service.admission.AuthorizeProbe(ctx, *connection)",
		},
		"backend/internal/modules/source/infrastructure/credentialstore/store.go": {
			"ManagedCredentialAvailable",
			"SELECT key_version",
			"supportsKeyVersion",
		},
		"backend/internal/bootstrap/app.go": {
			"func newCollectionAdmissionGate(rights *sourcepostgres.RightsDecisionReader",
			"func newCollectionRequestBudgetStatus(budget *sourcepostgres.ExternalRequestBudget)",
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
		"backend/test/_suite/internal/modules/source/application/x_metric_refresh_test.go": {
			"TestXMetricRefreshAdmissionDenialStopsBeforeConnectorResolution",
		},
		"backend/test/_suite/internal/modules/source/application/instant_search_test.go": {
			"TestInstantSearchAdmissionDenialStopsBeforeConnectorResolution",
		},
		"backend/test/_suite/internal/modules/source/application/collection_service_integration_test.go": {
			"TestCollectionHealthAdmissionDenialStopsBeforeConnectorResolutionAndPersistsSafeAudit",
		},
		"backend/test/_suite/internal/modules/source/infrastructure/postgres/rights_decision_reader_integration_test.go": {
			"TestRightsDecisionReaderResolvesCurrentEndpointFetchConservatively",
		},
		"backend/test/_suite/internal/modules/source/infrastructure/connector_registry_test.go": {
			"TestConnectorRegistryDefersManagedCredentialDecryptionUntilTheRequestBoundary",
		},
		"backend/test/_suite/internal/modules/source/infrastructure/postgres/external_request_budget_integration_test.go": {
			"TestExternalRequestBudgetEnforcesPerMinuteRateLimitAtomicallyWithoutConsumingDailyBudget",
		},
		"backend/test/_suite/internal/modules/source/infrastructure/collection_request_budget_status_test.go": {
			"TestCollectionRequestBudgetStatusMapsEveryP0ProfileWithoutConsumingARequest",
		},
		"backend/Makefile": {
			"source-admission-matrix-acceptance:",
			"TestCollectionAdmissionChecksManagedCredentialStatusAfterRights",
			"TestCollectionAdmissionChecksBudgetAndRateLimitAfterCredentialStatus",
			"TestCollectionRequestBudgetStatusMapsEveryP0ProfileWithoutConsumingARequest",
			"TestXMetricRefreshAdmissionDenialStopsBeforeConnectorResolution",
			"TestInstantSearchAdmissionDenialStopsBeforeConnectorResolution",
			"TestCollectionHealthAdmissionDenialStopsBeforeConnectorResolutionAndPersistsSafeAudit",
			"TestExternalRequestBudgetEnforcesPerMinuteRateLimitAtomicallyWithoutConsumingDailyBudget",
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
	admission := readRepositoryFile(t, repository, "backend/internal/modules/source/application/collection_admission.go")
	assertSourceOrder(t, admission, "func (gate *CollectionAdmissionGate) AuthorizeCollection", "\treturn nil\n}", "gate.rights.ResolveCurrentFetch", "gate.credentials.ManagedCredentialAvailable")
	assertSourceOrder(t, admission, "func (gate *CollectionAdmissionGate) AuthorizeCollection", "\treturn nil\n}", "gate.credentials.ManagedCredentialAvailable", "gate.budget.CollectionRequestAvailable")
	xMetricRefresh := readRepositoryFile(t, repository, "backend/internal/modules/source/application/x_metric_refresh.go")
	assertSourceOrder(t, xMetricRefresh, "func (service *XMetricRefreshService) Refresh", "CandidateCount: len(postIDs)", "service.admission.AuthorizeCollection", "service.connectors.Resolve")
	instantSearch := readRepositoryFile(t, repository, "backend/internal/modules/source/application/instant_search.go")
	assertSourceOrder(t, instantSearch, "func (service *InstantSearchService) searchConnection", "func normalizeInstantSearchSourceTypes", "service.admission.AuthorizeCollection", "service.connectors.Resolve")
	collectionControl := readRepositoryFile(t, repository, "backend/internal/modules/source/application/collection_control.go")
	assertSourceOrder(t, collectionControl, "func (service *CollectionControlService) Health", "type unavailableCollectionProbeAuthorizer", "service.admission.AuthorizeProbe", "service.connectors.Resolve")

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
