package application_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	operationspostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/infrastructure/postgres"
	sourceapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
	sourcepostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
)

func TestCollectionSSRFSecurityRejectionPersistsOnlySanitizedOperationalAudit(t *testing.T) {
	runtime := openRuntime(t)
	defer func() { _ = runtime.Close() }()
	request := collectionRequestForService(t, runtime, "ssrf-rejection", 1)
	before := readCollectionBusinessFactCounts(t, runtime)
	connector := &collectionConnectorFake{err: domain.NewCollectionError(
		domain.CollectionErrorPermanent,
		errors.New("RSS destination is not permitted"),
	)}
	service, err := newCollectionServiceForTest(sourceapplication.CollectionDependencies{
		Runtime: runtime, Sources: sourcepostgres.NewRepository(runtime), Runs: sourcepostgres.NewCollectionRepository(runtime),
		Connectors: connectorRegistryForAcceptance{connector: connector}, SecurityAudit: operationspostgres.NewAuditWriter(runtime),
		Now: func() time.Time { return request.WindowEnd },
	})
	if err != nil {
		t.Fatalf("NewCollectionService(): %v", err)
	}

	first, firstErr := service.Collect(context.Background(), request)
	second, secondErr := service.Collect(context.Background(), request)
	if firstErr == nil || domain.ClassifyCollectionError(firstErr) != domain.CollectionErrorPermanent || first.Status != domain.CollectionRunFailed {
		t.Fatalf("first SSRF rejection = %#v / %v", first, firstErr)
	}
	if secondErr != nil || second.ID != first.ID || second.Status != domain.CollectionRunFailed {
		t.Fatalf("repeated SSRF rejection = %#v / %v, want persisted failed audit run", second, secondErr)
	}
	if got := connector.calls.Load(); got != 1 {
		t.Fatalf("connector fetch calls = %d, want one attempt and no replay side effect", got)
	}
	if after := readCollectionBusinessFactCounts(t, runtime); after != before {
		t.Fatalf("business facts changed after repeated SSRF rejection: before=%#v after=%#v", before, after)
	}

	var runStatus, runError string
	var candidateCount, acceptedCount, rejectedCount int64
	if err := runtime.SQL.QueryRow(`
SELECT status, COALESCE(error_code, ''), candidate_count, accepted_count, rejected_count
FROM collection_runs WHERE id = $1`, first.ID).Scan(&runStatus, &runError, &candidateCount, &acceptedCount, &rejectedCount); err != nil {
		t.Fatalf("read sanitized collection run: %v", err)
	}
	if runStatus != "failed" || runError != "permanent" || candidateCount != 0 || acceptedCount != 0 || rejectedCount != 0 {
		t.Fatalf("sanitized collection run = %q/%q/%d/%d/%d", runStatus, runError, candidateCount, acceptedCount, rejectedCount)
	}
	var failures int
	var cursor string
	var lastSuccessfulRunID int64
	if err := runtime.SQL.QueryRow(`
SELECT consecutive_failures, COALESCE(cursor_value, ''), COALESCE(last_successful_run_id, 0)
FROM source_checkpoints WHERE id = $1`, request.Targets[0].Checkpoint.ID).Scan(&failures, &cursor, &lastSuccessfulRunID); err != nil {
		t.Fatalf("read checkpoint after SSRF rejection: %v", err)
	}
	if failures != 1 || cursor != "" || lastSuccessfulRunID != 0 {
		t.Fatalf("checkpoint after replay = failures:%d cursor:%q success:%d", failures, cursor, lastSuccessfulRunID)
	}

	var auditCount int
	var actorType, action, resourceType, result, beforeData, afterData string
	if err := runtime.SQL.QueryRow(`
SELECT count(*), min(actor_type), min(action), min(resource_type), min(result),
       COALESCE(min(before_data::text), ''), COALESCE(min(after_data::text), '')
FROM audit_logs WHERE action = 'collection.security_rejected' AND resource_id = $1`, first.ID).
		Scan(&auditCount, &actorType, &action, &resourceType, &result, &beforeData, &afterData); err != nil {
		t.Fatalf("read SSRF security audit: %v", err)
	}
	if auditCount != 1 || actorType != "system" || action != "collection.security_rejected" || resourceType != "collection_run" || result != "denied" || beforeData != "" || afterData != `{"reason_code": "ssrf_destination_not_permitted"}` {
		t.Fatalf("SSRF audit = %d/%q/%q/%q/%q/%q/%q", auditCount, actorType, action, resourceType, result, beforeData, afterData)
	}
	if strings.Contains(afterData, "feeds.example") || strings.Contains(afterData, "127.0.0.1") || strings.Contains(afterData, "destination") && afterData != `{"reason_code": "ssrf_destination_not_permitted"}` {
		t.Fatalf("SSRF audit leaked endpoint or network input: %s", afterData)
	}
}

func TestCollectionCompressionSecurityRejectionPersistsNoBusinessFactsOrPayload(t *testing.T) {
	runtime := openRuntime(t)
	defer func() { _ = runtime.Close() }()
	request := collectionRequestForService(t, runtime, "compression-rejection", 1)
	before := readCollectionBusinessFactCounts(t, runtime)
	connector := &collectionConnectorFake{err: domain.NewCollectionError(
		domain.CollectionErrorPermanent,
		errors.New("RSS compressed response is not permitted"),
	)}
	service, err := newCollectionServiceForTest(sourceapplication.CollectionDependencies{
		Runtime: runtime, Sources: sourcepostgres.NewRepository(runtime), Runs: sourcepostgres.NewCollectionRepository(runtime),
		Connectors: connectorRegistryForAcceptance{connector: connector}, SecurityAudit: operationspostgres.NewAuditWriter(runtime),
		Now: func() time.Time { return request.WindowEnd },
	})
	if err != nil {
		t.Fatalf("NewCollectionService(): %v", err)
	}

	first, firstErr := service.Collect(context.Background(), request)
	second, secondErr := service.Collect(context.Background(), request)
	if firstErr == nil || domain.ClassifyCollectionError(firstErr) != domain.CollectionErrorPermanent || first.Status != domain.CollectionRunFailed {
		t.Fatalf("first compression rejection = %#v / %v", first, firstErr)
	}
	if secondErr != nil || second.ID != first.ID || second.Status != domain.CollectionRunFailed {
		t.Fatalf("repeated compression rejection = %#v / %v", second, secondErr)
	}
	if got := connector.calls.Load(); got != 1 {
		t.Fatalf("connector fetch calls = %d, want one attempt and no replay side effect", got)
	}
	if after := readCollectionBusinessFactCounts(t, runtime); after != before {
		t.Fatalf("business facts changed after repeated compression rejection: before=%#v after=%#v", before, after)
	}

	var runStatus, runError string
	var candidateCount, acceptedCount, rejectedCount int64
	if err := runtime.SQL.QueryRow(`
SELECT status, COALESCE(error_code, ''), candidate_count, accepted_count, rejected_count
FROM collection_runs WHERE id = $1`, first.ID).Scan(&runStatus, &runError, &candidateCount, &acceptedCount, &rejectedCount); err != nil {
		t.Fatal(err)
	}
	if runStatus != "failed" || runError != "permanent" || candidateCount != 0 || acceptedCount != 0 || rejectedCount != 0 {
		t.Fatalf("sanitized compression run = %q/%q/%d/%d/%d", runStatus, runError, candidateCount, acceptedCount, rejectedCount)
	}

	var auditCount int
	var afterData, allRows string
	if err := runtime.SQL.QueryRow(`
SELECT count(*), COALESCE(min(after_data::text), ''), COALESCE(string_agg(row_to_json(audit_logs)::text, ''), '')
FROM audit_logs WHERE action='collection.security_rejected' AND resource_id=$1`, first.ID).
		Scan(&auditCount, &afterData, &allRows); err != nil {
		t.Fatal(err)
	}
	if auditCount != 1 || afterData != `{"reason_code": "compressed_content_not_permitted"}` {
		t.Fatalf("compression audit = %d/%q", auditCount, afterData)
	}
	for _, forbidden := range []string{"gzip", "zip", "expanded", "payload", "RSS compressed response"} {
		if strings.Contains(allRows, forbidden) {
			t.Fatalf("compression audit leaked %q: %s", forbidden, allRows)
		}
	}
}

func TestCollectionFetchRightsDenialStopsBeforeConnectorResolutionAndPersistsSanitizedAudit(t *testing.T) {
	runtime := openRuntime(t)
	defer func() { _ = runtime.Close() }()
	request := collectionRequestForService(t, runtime, "fetch-rights-rejection", 1)
	before := readCollectionBusinessFactCounts(t, runtime)
	admission := &denyingCollectionAdmission{}
	registry := &countingCollectionConnectorRegistry{}
	service, err := sourceapplication.NewCollectionService(sourceapplication.CollectionDependencies{
		Runtime: runtime, Sources: sourcepostgres.NewRepository(runtime), Runs: sourcepostgres.NewCollectionRepository(runtime),
		Admission: admission, Connectors: registry, SecurityAudit: operationspostgres.NewAuditWriter(runtime),
		Now: func() time.Time { return request.WindowEnd },
	})
	if err != nil {
		t.Fatalf("NewCollectionService(): %v", err)
	}

	run, collectErr := service.Collect(context.Background(), request)
	if collectErr == nil || domain.ClassifyCollectionError(collectErr) != domain.CollectionErrorPermanent || run.Status != domain.CollectionRunFailed {
		t.Fatalf("fetch Rights rejection = %#v / %v", run, collectErr)
	}
	if admission.calls != 1 || registry.calls != 0 {
		t.Fatalf("admission/connector resolution calls = %d/%d, want 1/0", admission.calls, registry.calls)
	}
	if after := readCollectionBusinessFactCounts(t, runtime); after != before {
		t.Fatalf("business facts changed after fetch Rights rejection: before=%#v after=%#v", before, after)
	}

	var runStatus, runError, afterData, allRows string
	var auditCount int
	if err := runtime.SQL.QueryRow(`
SELECT status, COALESCE(error_code, '') FROM collection_runs WHERE id=$1`, run.ID).Scan(&runStatus, &runError); err != nil {
		t.Fatalf("read denied collection run: %v", err)
	}
	if err := runtime.SQL.QueryRow(`
SELECT count(*), COALESCE(min(after_data::text), ''), COALESCE(string_agg(row_to_json(audit_logs)::text, ''), '')
FROM audit_logs WHERE action='collection.security_rejected' AND resource_id=$1`, run.ID).
		Scan(&auditCount, &afterData, &allRows); err != nil {
		t.Fatalf("read fetch Rights security audit: %v", err)
	}
	if runStatus != "failed" || runError != "permanent" || auditCount != 1 || afterData != `{"reason_code": "fetch_rights_not_permitted"}` {
		t.Fatalf("fetch Rights persisted result = %q/%q/%d/%q", runStatus, runError, auditCount, afterData)
	}
	for _, forbidden := range []string{"credential", "token", "endpoint", "feeds.example"} {
		if strings.Contains(allRows, forbidden) {
			t.Fatalf("fetch Rights audit leaked %q: %s", forbidden, allRows)
		}
	}
}

type connectorRegistryForAcceptance struct{ connector domain.Connector }

func (registry connectorRegistryForAcceptance) Resolve(context.Context, domain.SourceConnection) (domain.Connector, error) {
	return registry.connector, nil
}

type denyingCollectionAdmission struct{ calls int }

func (admission *denyingCollectionAdmission) AuthorizeCollection(context.Context, domain.SourceConnection) error {
	admission.calls++
	return domain.NewCollectionError(domain.CollectionErrorPermanent, errors.New("source fetch rights are not permitted"))
}

type countingCollectionConnectorRegistry struct{ calls int }

func (registry *countingCollectionConnectorRegistry) Resolve(context.Context, domain.SourceConnection) (domain.Connector, error) {
	registry.calls++
	return nil, errors.New("connector resolution must not be reached")
}

type collectionBusinessFactCounts struct {
	Items, Evidence, Contents, MicroEvents, Events, Claims int64
}

func readCollectionBusinessFactCounts(t *testing.T, runtime *database.Runtime) collectionBusinessFactCounts {
	t.Helper()
	var counts collectionBusinessFactCounts
	if err := runtime.SQL.QueryRow(`
SELECT
  (SELECT count(*) FROM collection_run_items),
  (SELECT count(*) FROM evidence_snapshots),
  (SELECT count(*) FROM contents),
  (SELECT count(*) FROM micro_events),
  (SELECT count(*) FROM events),
  (SELECT count(*) FROM event_claims)`).Scan(
		&counts.Items, &counts.Evidence, &counts.Contents, &counts.MicroEvents, &counts.Events, &counts.Claims,
	); err != nil {
		t.Fatalf("read collection business fact counts: %v", err)
	}
	return counts
}
