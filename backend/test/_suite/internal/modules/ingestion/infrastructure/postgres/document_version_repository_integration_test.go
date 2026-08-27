package postgres_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	ingestionpostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
	"github.com/StephenQiu30/hotkey-server/backend/test/postgresfixture"
)

func TestDocumentVersionRepositoryCreatesIdempotentConcurrentRevisionsWithoutPersistingBody(t *testing.T) {
	runtime := openDocumentVersionRuntime(t)
	defer func() { _ = runtime.Close() }()
	sourceID := createDocumentVersionSource(t, runtime, "concurrent")
	reader := &integrationDocumentObservationReader{observations: make(map[int64]ingestionapplication.DocumentObservationDTO)}
	const observations = 8
	observationIDs := make([]int64, 0, observations)
	for index := range observations {
		observationID := insertSourceObservation(t, runtime, sourceID, "publisher-work-1", index)
		observationIDs = append(observationIDs, observationID)
		reader.observations[observationID] = ingestionapplication.DocumentObservationDTO{
			ID: observationID, SourceConnectionID: sourceID, ExternalWorkID: "publisher-work-1",
			BodyOrigin: ingestionapplication.BodyOriginFeedContent, Completeness: ingestionapplication.BodyCompletenessFull,
			Body: fmt.Sprintf("private body marker %d\nsecond line", index), Language: "en",
			CapturedAt: documentVersionCapturedAt(index),
		}
	}
	repository := ingestionpostgres.NewDocumentVersionRepository(runtime)
	service, err := ingestionapplication.NewDocumentVersionService(ingestionapplication.DocumentVersionDependencies{Observations: reader, Versions: repository})
	if err != nil {
		t.Fatalf("NewDocumentVersionService() error = %v", err)
	}

	type persistenceResult struct {
		observationID int64
		result        ingestionapplication.PersistDocumentVersionResult
		err           error
	}
	results := make(chan persistenceResult, observations)
	var group sync.WaitGroup
	for index, observationID := range observationIDs {
		index, observationID := index, observationID
		group.Add(1)
		go func() {
			defer group.Done()
			var qualityScore *float64
			if index == 1 {
				explicitZero := 0.0
				qualityScore = &explicitZero
			} else if index > 1 {
				value := 90.123
				qualityScore = &value
			}
			result, err := service.PersistSourceObservation(context.Background(), ingestionapplication.PersistDocumentVersionCommand{
				SourceObservationID: observationID, ExtractorVersion: "rss-entry-v2",
				ExtractorProfileVersion: "rss-profile-v3", ExtractorProfileSHA256: strings.Repeat("a", 64),
				QualityScore: qualityScore, QualityWarnings: []string{"fixture"},
			})
			results <- persistenceResult{observationID: observationID, result: result, err: err}
		}()
	}
	group.Wait()
	close(results)

	versionIDs := make(map[int64]int64, observations)
	createdDocuments := 0
	var documentID int64
	for item := range results {
		if item.err != nil {
			t.Fatalf("PersistSourceObservation(%d) error = %v", item.observationID, item.err)
		}
		if !item.result.DocumentVersionCreated {
			t.Fatalf("PersistSourceObservation(%d) version created = false", item.observationID)
		}
		if item.result.DocumentCreated {
			createdDocuments++
		}
		if documentID == 0 {
			documentID = item.result.Document.ID
		}
		if item.result.Document.ID != documentID || item.result.DocumentVersion.DocumentID != documentID {
			t.Fatalf("observation %d document ids = %d/%d, want stable %d", item.observationID, item.result.Document.ID, item.result.DocumentVersion.DocumentID, documentID)
		}
		versionIDs[item.observationID] = item.result.DocumentVersion.ID
	}
	if createdDocuments != 1 {
		t.Fatalf("created document count = %d, want 1", createdDocuments)
	}

	for _, observationID := range observationIDs {
		retried, err := service.PersistSourceObservation(context.Background(), ingestionapplication.PersistDocumentVersionCommand{
			SourceObservationID: observationID, ExtractorVersion: "rss-entry-v2",
			ExtractorProfileVersion: "rss-profile-v3", ExtractorProfileSHA256: strings.Repeat("a", 64),
			QualityScore: qualityScoreForDocumentFixture(observationIDs, observationID), QualityWarnings: []string{"fixture"},
		})
		if err != nil || retried.DocumentCreated || retried.DocumentVersionCreated || retried.DocumentVersion.ID != versionIDs[observationID] {
			t.Fatalf("retry observation %d = %#v/%v, want same immutable version", observationID, retried, err)
		}
	}
	concurrentRetries := make(chan persistenceResult, observations)
	for range observations {
		go func() {
			result, err := service.PersistSourceObservation(context.Background(), ingestionapplication.PersistDocumentVersionCommand{
				SourceObservationID: observationIDs[0], ExtractorVersion: "rss-entry-v2",
				ExtractorProfileVersion: "rss-profile-v3", ExtractorProfileSHA256: strings.Repeat("a", 64),
				QualityWarnings: []string{"fixture"},
			})
			concurrentRetries <- persistenceResult{observationID: observationIDs[0], result: result, err: err}
		}()
	}
	for range observations {
		retry := <-concurrentRetries
		if retry.err != nil || retry.result.DocumentCreated || retry.result.DocumentVersionCreated || retry.result.DocumentVersion.ID != versionIDs[observationIDs[0]] {
			t.Fatalf("concurrent retry = %#v/%v, want one stable immutable version", retry.result, retry.err)
		}
	}

	reader.setBody(observationIDs[0], "changed body for the same observation")
	changedBodyVersion, err := service.PersistSourceObservation(context.Background(), ingestionapplication.PersistDocumentVersionCommand{
		SourceObservationID: observationIDs[0], ExtractorVersion: "rss-entry-v2",
		ExtractorProfileVersion: "rss-profile-v3", ExtractorProfileSHA256: strings.Repeat("a", 64),
		QualityWarnings: []string{"fixture"},
	})
	if err != nil || !changedBodyVersion.DocumentVersionCreated || changedBodyVersion.DocumentVersion.ID == versionIDs[observationIDs[0]] {
		t.Fatalf("same observation changed body version = %#v/%v, want a new revision", changedBodyVersion, err)
	}
	_, err = service.PersistSourceObservation(context.Background(), ingestionapplication.PersistDocumentVersionCommand{
		SourceObservationID: observationIDs[0], ExtractorVersion: "rss-entry-v3",
		ExtractorProfileVersion: "rss-profile-v3", ExtractorProfileSHA256: strings.Repeat("a", 64),
		QualityWarnings: []string{"fixture"},
	})
	if !errors.Is(err, sharedrepository.ErrConflict) {
		t.Fatalf("same business key with changed immutable metadata error = %v, want conflict", err)
	}

	reader.setBody(observationIDs[0], "private body marker 0\nsecond line")
	newProfileVersion, err := service.PersistSourceObservation(context.Background(), ingestionapplication.PersistDocumentVersionCommand{
		SourceObservationID: observationIDs[0], ExtractorVersion: "rss-entry-v3",
		ExtractorProfileVersion: "rss-profile-v4", ExtractorProfileSHA256: strings.Repeat("d", 64),
		QualityWarnings: []string{"fixture"},
	})
	if err != nil || !newProfileVersion.DocumentVersionCreated || newProfileVersion.DocumentVersion.ID == versionIDs[observationIDs[0]] {
		t.Fatalf("same observation new profile version = %#v/%v, want a new revision", newProfileVersion, err)
	}

	expectedVersions := observations + 2
	var documentCount, versionCount, bodyColumns int
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM documents WHERE source_connection_id=$1`, sourceID).Scan(&documentCount); err != nil {
		t.Fatalf("count documents: %v", err)
	}
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM document_versions WHERE document_id=$1`, documentID).Scan(&versionCount); err != nil {
		t.Fatalf("count document versions: %v", err)
	}
	if err := runtime.SQL.QueryRow(`
SELECT count(*) FROM information_schema.columns
WHERE table_schema='public' AND table_name='document_versions'
  AND column_name IN ('body','content','markdown','plaintext')`).Scan(&bodyColumns); err != nil {
		t.Fatalf("inspect document version columns: %v", err)
	}
	if documentCount != 1 || versionCount != expectedVersions || bodyColumns != 0 {
		t.Fatalf("persisted documents/versions/body columns = %d/%d/%d, want 1/%d/0", documentCount, versionCount, bodyColumns, expectedVersions)
	}

	rows, err := runtime.SQL.Query(`SELECT revision_no FROM document_versions WHERE document_id=$1 ORDER BY revision_no`, documentID)
	if err != nil {
		t.Fatalf("read revisions: %v", err)
	}
	defer rows.Close()
	revisions := make([]int64, 0, expectedVersions)
	for rows.Next() {
		var revision int64
		if err := rows.Scan(&revision); err != nil {
			t.Fatalf("scan revision: %v", err)
		}
		revisions = append(revisions, revision)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate revisions: %v", err)
	}
	wantRevisions := make([]int64, expectedVersions)
	for index := range wantRevisions {
		wantRevisions[index] = int64(index + 1)
	}
	if fmt.Sprint(revisions) != fmt.Sprint(wantRevisions) {
		t.Fatalf("revision numbers = %v, want %v", revisions, wantRevisions)
	}

	var unknownScore, explicitZero sql.NullFloat64
	if err := runtime.SQL.QueryRow(`SELECT quality_score FROM document_versions WHERE id=$1`, versionIDs[observationIDs[0]]).Scan(&unknownScore); err != nil {
		t.Fatalf("read unknown quality score: %v", err)
	}
	if err := runtime.SQL.QueryRow(`SELECT quality_score FROM document_versions WHERE source_observation_id=$1`, observationIDs[1]).Scan(&explicitZero); err != nil {
		t.Fatalf("read explicit zero quality score: %v", err)
	}
	if unknownScore.Valid || !explicitZero.Valid || explicitZero.Float64 != 0 {
		t.Fatalf("quality score unknown/zero = %#v/%#v, want NULL/0", unknownScore, explicitZero)
	}
}

func TestDocumentVersionRepositoryLifecycleCASAndReadableProjection(t *testing.T) {
	runtime := openDocumentVersionRuntime(t)
	defer func() { _ = runtime.Close() }()
	sourceID := createDocumentVersionSource(t, runtime, "lifecycle")
	observationID := insertSourceObservation(t, runtime, sourceID, "lifecycle-work", 0)
	reader := &integrationDocumentObservationReader{observations: map[int64]ingestionapplication.DocumentObservationDTO{
		observationID: {
			ID: observationID, SourceConnectionID: sourceID, ExternalWorkID: "lifecycle-work",
			BodyOrigin: ingestionapplication.BodyOriginFeedContent, Completeness: ingestionapplication.BodyCompletenessFull,
			Body: "lifecycle body", Language: "en", CapturedAt: documentVersionCapturedAt(0),
		},
	}}
	repository := ingestionpostgres.NewDocumentVersionRepository(runtime)
	service, err := ingestionapplication.NewDocumentVersionService(ingestionapplication.DocumentVersionDependencies{Observations: reader, Versions: repository})
	if err != nil {
		t.Fatalf("NewDocumentVersionService() error = %v", err)
	}
	persisted, err := service.PersistSourceObservation(context.Background(), ingestionapplication.PersistDocumentVersionCommand{
		SourceObservationID: observationID, ExtractorVersion: "rss-entry-v2",
		ExtractorProfileVersion: "rss-profile-v3", ExtractorProfileSHA256: strings.Repeat("a", 64),
	})
	if err != nil {
		t.Fatalf("PersistSourceObservation() error = %v", err)
	}
	documentVersionID := persisted.DocumentVersion.ID

	derivePending := transitionDocumentVersion(t, service, documentVersionID, 1, ingestionapplication.DocumentDerivedPending)
	if derivePending.Version != 2 {
		t.Fatalf("derive pending version = %d, want 2", derivePending.Version)
	}
	_, err = service.TransitionDocumentVersion(context.Background(), ingestionapplication.TransitionDocumentVersionCommand{
		DocumentVersionID: documentVersionID, ExpectedVersion: 2, To: ingestionapplication.DocumentDerivedAvailable,
	})
	if !errors.Is(err, sharedrepository.ErrConflict) {
		t.Fatalf("derived_available without artifact error = %v, want conflict", err)
	}

	createAvailableDocumentArtifact(t, runtime, sourceID, persisted.Document.ID, documentVersionID, persisted.DocumentVersion.ContentSHA256)
	derivedAvailable := transitionDocumentVersion(t, service, documentVersionID, 2, ingestionapplication.DocumentDerivedAvailable)
	_, err = service.TransitionDocumentVersion(context.Background(), ingestionapplication.TransitionDocumentVersionCommand{
		DocumentVersionID: documentVersionID, ExpectedVersion: derivedAvailable.Version, To: ingestionapplication.DocumentReadable,
	})
	if !errors.Is(err, sharedrepository.ErrInvalidInput) {
		t.Fatalf("readable without display decision error = %v, want invalid input", err)
	}

	rightsNow := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
	expiredAt := rightsNow.Add(-time.Second)
	expiredDisplayID := createDocumentDisplayDecision(t, runtime, sourceID, documentVersionID,
		persisted.DocumentVersion.ContentSHA256, 2, &expiredAt, documentVersionID)
	wrongSubjectDisplayID := createDocumentDisplayDecision(t, runtime, sourceID, documentVersionID,
		persisted.DocumentVersion.ContentSHA256, 3, nil, documentVersionID+1)
	for name, decisionID := range map[string]int64{"expired": expiredDisplayID, "wrong_subject": wrongSubjectDisplayID} {
		_, err = service.TransitionDocumentVersion(context.Background(), ingestionapplication.TransitionDocumentVersionCommand{
			DocumentVersionID: documentVersionID, ExpectedVersion: derivedAvailable.Version, To: ingestionapplication.DocumentReadable,
			DisplayPrivateRightsDecisionID: &decisionID,
		})
		if !errors.Is(err, sharedrepository.ErrConflict) {
			t.Fatalf("readable with %s display decision error = %v, want conflict", name, err)
		}
	}
	currentDisplayID := createDocumentDisplayDecision(t, runtime, sourceID, documentVersionID,
		persisted.DocumentVersion.ContentSHA256, 4, nil, documentVersionID)
	readable := transitionDocumentVersionWithDisplay(t, service, documentVersionID, derivedAvailable.Version, currentDisplayID)
	if readable.Version != 4 {
		t.Fatalf("readable version = %d, want 4", readable.Version)
	}
	var currentVersionID sql.NullInt64
	if err := runtime.SQL.QueryRow(`SELECT current_document_version_id FROM documents WHERE id=$1`, persisted.Document.ID).Scan(&currentVersionID); err != nil {
		t.Fatalf("read current document version: %v", err)
	}
	if !currentVersionID.Valid || currentVersionID.Int64 != documentVersionID {
		t.Fatalf("current document version = %#v, want %d", currentVersionID, documentVersionID)
	}

	results := make(chan error, 2)
	for range 2 {
		go func() {
			_, err := service.TransitionDocumentVersion(context.Background(), ingestionapplication.TransitionDocumentVersionCommand{
				DocumentVersionID: documentVersionID, ExpectedVersion: readable.Version, To: ingestionapplication.DocumentPolicyBlocked,
			})
			results <- err
		}()
	}
	successes, conflicts := 0, 0
	for range 2 {
		if err := <-results; err == nil {
			successes++
		} else if errors.Is(err, sharedrepository.ErrConflict) {
			conflicts++
		} else {
			t.Fatalf("concurrent CAS error = %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent CAS success/conflict = %d/%d, want 1/1", successes, conflicts)
	}
	if err := runtime.SQL.QueryRow(`SELECT current_document_version_id FROM documents WHERE id=$1`, persisted.Document.ID).Scan(&currentVersionID); err != nil {
		t.Fatalf("read cleared current version: %v", err)
	}
	if currentVersionID.Valid {
		t.Fatalf("blocked current document version = %#v, want NULL", currentVersionID)
	}

	recovered := transitionDocumentVersion(t, service, documentVersionID, 5, ingestionapplication.DocumentDerivedPending)
	if recovered.Version != 6 {
		t.Fatalf("rights recovery version = %d, want 6", recovered.Version)
	}
	_, err = service.TransitionDocumentVersion(context.Background(), ingestionapplication.TransitionDocumentVersionCommand{
		DocumentVersionID: documentVersionID, ExpectedVersion: 5, To: ingestionapplication.DocumentReadable,
		DisplayPrivateRightsDecisionID: &currentDisplayID,
	})
	if !errors.Is(err, sharedrepository.ErrConflict) {
		t.Fatalf("stale lifecycle error = %v, want conflict", err)
	}
}

func TestDocumentVersionRepositoryRejectsHigherPriorityRightsDeny(t *testing.T) {
	runtime := openDocumentVersionRuntime(t)
	defer func() { _ = runtime.Close() }()

	for index, action := range []string{"store_derived", "retain", "display_private"} {
		sourceID := createDocumentVersionSource(t, runtime, "deny-"+action)
		observationID := insertSourceObservation(t, runtime, sourceID, "deny-work-"+action, index+20)
		reader := &integrationDocumentObservationReader{observations: map[int64]ingestionapplication.DocumentObservationDTO{
			observationID: {
				ID: observationID, SourceConnectionID: sourceID, ExternalWorkID: "deny-work-" + action,
				BodyOrigin: ingestionapplication.BodyOriginFeedContent, Completeness: ingestionapplication.BodyCompletenessFull,
				Body: "rights deny fixture", Language: "en", CapturedAt: documentVersionCapturedAt(index + 20),
			},
		}}
		service, err := ingestionapplication.NewDocumentVersionService(ingestionapplication.DocumentVersionDependencies{
			Observations: reader,
			Versions:     ingestionpostgres.NewDocumentVersionRepository(runtime),
		})
		if err != nil {
			t.Fatalf("NewDocumentVersionService(%s) error = %v", action, err)
		}
		persisted, err := service.PersistSourceObservation(context.Background(), ingestionapplication.PersistDocumentVersionCommand{
			SourceObservationID: observationID, ExtractorVersion: "rss-entry-v2",
			ExtractorProfileVersion: "rss-profile-v3", ExtractorProfileSHA256: strings.Repeat("a", 64),
		})
		if err != nil {
			t.Fatalf("PersistSourceObservation(%s) error = %v", action, err)
		}
		documentVersionID := persisted.DocumentVersion.ID
		transitionDocumentVersion(t, service, documentVersionID, 1, ingestionapplication.DocumentDerivedPending)
		createAvailableDocumentArtifact(t, runtime, sourceID, persisted.Document.ID, documentVersionID, persisted.DocumentVersion.ContentSHA256)

		var selectedDisplayDecisionID *int64
		expectedVersion := int64(2)
		to := ingestionapplication.DocumentDerivedAvailable
		if action == "display_private" {
			derived := transitionDocumentVersion(t, service, documentVersionID, expectedVersion, ingestionapplication.DocumentDerivedAvailable)
			expectedVersion = derived.Version
			to = ingestionapplication.DocumentReadable
			allowPolicy := createDocumentRightsPolicy(t, runtime, sourceID, 2, time.Now().UTC().Add(-time.Hour))
			allowID := insertDocumentRightsDecision(t, runtime, allowPolicy, documentVersionID,
				persisted.DocumentVersion.ContentSHA256, action, nil, nil, documentVersionID)
			selectedDisplayDecisionID = &allowID
		}
		denyPolicy := createDocumentObservationRightsPolicy(t, runtime, sourceID, observationID, 3, time.Now().UTC().Add(-time.Hour))
		insertDocumentRightsDecisionWithOutcome(t, runtime, denyPolicy, documentVersionID, persisted.DocumentVersion.ContentSHA256,
			action, "deny", nil, nil, documentVersionID)

		_, err = service.TransitionDocumentVersion(context.Background(), ingestionapplication.TransitionDocumentVersionCommand{
			DocumentVersionID: documentVersionID, ExpectedVersion: expectedVersion, To: to,
			DisplayPrivateRightsDecisionID: selectedDisplayDecisionID,
		})
		if !errors.Is(err, sharedrepository.ErrConflict) {
			t.Fatalf("%s transition with unsuperseded higher-priority deny error = %v, want conflict", action, err)
		}
	}
}

func transitionDocumentVersion(t *testing.T, service *ingestionapplication.DocumentVersionService, id, expected int64, to string) ingestionapplication.DocumentVersionDTO {
	t.Helper()
	updated, err := service.TransitionDocumentVersion(context.Background(), ingestionapplication.TransitionDocumentVersionCommand{
		DocumentVersionID: id, ExpectedVersion: expected, To: to,
	})
	if err != nil {
		t.Fatalf("TransitionDocumentVersion(%d,%s) error = %v", expected, to, err)
	}
	return updated.DocumentVersion
}

func transitionDocumentVersionWithDisplay(t *testing.T, service *ingestionapplication.DocumentVersionService, id, expected, displayDecisionID int64) ingestionapplication.DocumentVersionDTO {
	t.Helper()
	updated, err := service.TransitionDocumentVersion(context.Background(), ingestionapplication.TransitionDocumentVersionCommand{
		DocumentVersionID: id, ExpectedVersion: expected, To: ingestionapplication.DocumentReadable,
		DisplayPrivateRightsDecisionID: &displayDecisionID,
	})
	if err != nil {
		t.Fatalf("TransitionDocumentVersion(%d,readable) error = %v", expected, err)
	}
	return updated.DocumentVersion
}

type integrationDocumentObservationReader struct {
	mu           sync.RWMutex
	observations map[int64]ingestionapplication.DocumentObservationDTO
}

func (reader *integrationDocumentObservationReader) ReadDocumentObservation(_ context.Context, id int64) (ingestionapplication.DocumentObservationDTO, error) {
	reader.mu.RLock()
	defer reader.mu.RUnlock()
	observation, found := reader.observations[id]
	if !found {
		return ingestionapplication.DocumentObservationDTO{}, fmt.Errorf("%w: source observation %d", sharedrepository.ErrNotFound, id)
	}
	return observation, nil
}

func (reader *integrationDocumentObservationReader) setBody(id int64, body string) {
	reader.mu.Lock()
	defer reader.mu.Unlock()
	observation := reader.observations[id]
	observation.Body = body
	reader.observations[id] = observation
}

func openDocumentVersionRuntime(t *testing.T) *database.Runtime {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatalf("database.Open(): %v", err)
	}
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		_ = runtime.Close()
		t.Fatalf("database.InitializeEmpty(): %v", err)
	}
	return runtime
}

func createDocumentVersionSource(t *testing.T, runtime *database.Runtime, suffix string) int64 {
	t.Helper()
	var sourceID int64
	if err := runtime.SQL.QueryRow(`
INSERT INTO source_connections (source_type, name, endpoint)
VALUES ('rss', $1, $2) RETURNING id`,
		fmt.Sprintf("document-version-%s-%d", suffix, time.Now().UnixNano()),
		fmt.Sprintf("https://feed.example.test/%s", suffix)).Scan(&sourceID); err != nil {
		t.Fatalf("insert source connection: %v", err)
	}
	return sourceID
}

func insertSourceObservation(t *testing.T, runtime *database.Runtime, sourceID int64, externalID string, index int) int64 {
	t.Helper()
	capturedAt := documentVersionCapturedAt(index)
	upstreamIdentity := fmt.Sprintf("%064x", index+1)
	var observationID int64
	if err := runtime.SQL.QueryRow(`
INSERT INTO source_observations (
  source_connection_id, external_id, upstream_identity, source_code, content_type,
  title, language, source_record_url, canonical_url, body_origin, completeness,
  published_at,published_utc_offset_minutes,discovered_at, captured_at
) VALUES ($1,$2,$3,'rss','article',$4,'en',$5,$6,'feed_content','full',$8,$9,$7,$7)
RETURNING id`, sourceID, externalID, upstreamIdentity, fmt.Sprintf("revision %d", index),
		fmt.Sprintf("https://feed.example.test/records/%d", index),
		fmt.Sprintf("https://publisher.example.test/articles/%s", externalID), capturedAt,
		capturedAt.Add(-time.Hour), 480).Scan(&observationID); err != nil {
		t.Fatalf("insert source observation %d: %v", index, err)
	}
	return observationID
}

func documentVersionCapturedAt(index int) time.Time {
	return time.Date(2026, time.August, 9, 9, index, 0, 0, time.UTC)
}

func qualityScoreForDocumentFixture(observationIDs []int64, observationID int64) *float64 {
	index := sort.Search(len(observationIDs), func(index int) bool { return observationIDs[index] >= observationID })
	if index == 0 {
		return nil
	}
	if index == 1 {
		value := 0.0
		return &value
	}
	value := 90.123
	return &value
}

func createAvailableDocumentArtifact(t *testing.T, runtime *database.Runtime, sourceID, documentID, documentVersionID int64, contentSHA string) {
	t.Helper()
	now := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
	policy := createDocumentRightsPolicy(t, runtime, sourceID, 1, now.Add(-time.Hour))
	storeDerivedDecisionID := insertDocumentRightsDecision(t, runtime, policy, documentVersionID, contentSHA, "store_derived", nil, nil, documentVersionID)
	retentionDays := 30
	retainDecisionID := insertDocumentRightsDecision(t, runtime, policy, documentVersionID, contentSHA, "retain", nil, &retentionDays, documentVersionID)
	var documentCapturedAt time.Time
	if err := runtime.SQL.QueryRow(`SELECT captured_at FROM document_versions WHERE id=$1`, documentVersionID).Scan(&documentCapturedAt); err != nil {
		t.Fatalf("read document capture time: %v", err)
	}
	profileSHA := strings.Repeat("b", 64)
	artifactSHA := strings.Repeat("c", 64)
	anchorMapSHA := strings.Repeat("d", 64)
	var artifactID int64
	if err := runtime.SQL.QueryRow(`
INSERT INTO derived_artifacts (
  source_connection_id,document_version_id,store_derived_rights_decision_id,
  retain_rights_decision_id,artifact_type,transformer_profile_sha256,
  vault_relative_path,mime_type,sha256,size_bytes,
  anchor_normalization_version,anchor_map_profile_version,anchor_plaintext_sha256,
  anchor_markdown_sha256,anchor_map_sha256,retention_until
	) VALUES ($1,$2,$3,$4,'markdown',$5,$6,'text/markdown; charset=utf-8',$7,12,
	          'nfc-lf-collapse-space-v1','commonmark-gfm-visible-blocks-v1',$8,$7,$9,$10)
RETURNING id`, sourceID, documentVersionID, storeDerivedDecisionID, retainDecisionID, profileSHA,
		fmt.Sprintf("documents/%d/%d/markdown/%s.md", documentID, documentVersionID, profileSHA), artifactSHA,
		contentSHA, anchorMapSHA, documentCapturedAt.Add(30*24*time.Hour)).Scan(&artifactID); err != nil {
		t.Fatalf("insert derived artifact: %v", err)
	}
	if _, err := runtime.SQL.Exec(`
INSERT INTO document_anchor_blocks (
  derived_artifact_id,anchor_map_sha256,block_ordinal,
  plaintext_utf8_byte_start,plaintext_utf8_byte_end,
  markdown_utf8_byte_start,markdown_utf8_byte_end,markdown_anchor
) VALUES ($1,$2,0,0,2,0,2,'body-0000-000000000001')`, artifactID, anchorMapSHA); err != nil {
		t.Fatalf("insert document anchor block: %v", err)
	}
	if _, err := runtime.SQL.Exec(`
UPDATE derived_artifacts
SET lifecycle_state='derived_available', available_at=now(), active=true, updated_at=now()
WHERE id=$1`, artifactID); err != nil {
		t.Fatalf("publish derived artifact: %v", err)
	}
}

type documentRightsPolicyFixture struct {
	ID, Revision int64
	SourceID     int64
	Priority     int
	ScopeType    string
	Subject      string
	Basis        string
	PolicyHash   string
	EffectiveAt  time.Time
}

func createDocumentRightsPolicy(t *testing.T, runtime *database.Runtime, sourceID, revision int64, effectiveAt time.Time) documentRightsPolicyFixture {
	t.Helper()
	return createDocumentRightsPolicyForScope(t, runtime, sourceID, revision, 300, "source_endpoint",
		fmt.Sprintf("document-source-%d", sourceID), effectiveAt)
}

func createDocumentObservationRightsPolicy(t *testing.T, runtime *database.Runtime, sourceID, observationID, revision int64, effectiveAt time.Time) documentRightsPolicyFixture {
	t.Helper()
	return createDocumentRightsPolicyForScope(t, runtime, sourceID, revision, 400, "observation",
		fmt.Sprintf("%d", observationID), effectiveAt)
}

func createDocumentRightsPolicyForScope(t *testing.T, runtime *database.Runtime, sourceID, revision int64, priority int, scopeType, scopeSubject string, effectiveAt time.Time) documentRightsPolicyFixture {
	t.Helper()
	fixture := documentRightsPolicyFixture{
		Revision: revision, SourceID: sourceID, Priority: priority, ScopeType: scopeType,
		Subject: scopeSubject, Basis: fmt.Sprintf("document fixture policy %d", revision),
		EffectiveAt: effectiveAt,
	}
	policyHash := documentRightsFixtureDigest("policy", fmt.Sprint(sourceID), fmt.Sprint(revision), scopeType, scopeSubject)
	fixture.PolicyHash = policyHash
	actorID := insertDocumentRightsFixtureActor(t, runtime, policyHash)
	idempotencyKey, commandFingerprint := documentRightsFixtureReceipt("policy", policyHash)
	if err := runtime.SQL.QueryRow(`
INSERT INTO source_rights_policies (
  recorded_by_user_id,approved_by_user_id,idempotency_key,command_fingerprint,
  source_connection_id,scope_type,scope_subject,policy_revision,priority,
  basis_summary,policy_hash,effective_at
) VALUES ($1,$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
RETURNING id`, actorID, idempotencyKey, commandFingerprint, sourceID, scopeType, fixture.Subject,
		revision, priority, fixture.Basis, policyHash, effectiveAt).Scan(&fixture.ID); err != nil {
		t.Fatalf("insert document rights policy %d: %v", revision, err)
	}
	return fixture
}

func insertDocumentEndpointRightsDecision(t *testing.T, runtime *database.Runtime, policy documentRightsPolicyFixture, action, decision string, retentionDays *int) int64 {
	t.Helper()
	evaluatedAt := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
	idempotencyKey, commandFingerprint := documentRightsFixtureReceipt(
		"endpoint-decision", fmt.Sprint(policy.SourceID), fmt.Sprint(policy.ID), action, decision,
	)
	var decisionID int64
	if err := runtime.SQL.QueryRow(`
WITH decision_batch AS (
  INSERT INTO source_rights_decision_batches (
    source_connection_id,policy_id,expected_policy_version,subject_type,subject_key,input_digest,
    recorded_by_user_id,idempotency_key,command_fingerprint,decision_count
  ) SELECT $1::bigint,$2,policy.version,'source_endpoint',($1::bigint)::text,policy.policy_hash,
           policy.recorded_by_user_id,$10,$11,1
    FROM source_rights_policies AS policy WHERE policy.id=$2
  RETURNING id
)
INSERT INTO source_rights_decisions (
  decision_batch_id,source_connection_id,policy_id,policy_revision,policy_scope_type,policy_scope_subject,
  priority_rank,basis_summary,subject_type,subject_key,input_digest,action,decision,
  evaluator,evaluated_at,effective_from,retention_days
) SELECT decision_batch.id,$1::bigint,$2,$3,$4,$5,$6,$7,'source_endpoint',($1::bigint)::text,$8,$9,$12,
         'ingestion-endpoint-test',$13,$13,$14
  FROM decision_batch RETURNING id`, policy.SourceID, policy.ID, policy.Revision, policy.ScopeType, policy.Subject,
		policy.Priority, policy.Basis, policy.PolicyHash, action, idempotencyKey, commandFingerprint,
		decision, evaluatedAt, retentionDays).Scan(&decisionID); err != nil {
		t.Fatalf("insert endpoint %s %s decision: %v", action, decision, err)
	}
	return decisionID
}

func createDocumentDisplayDecision(t *testing.T, runtime *database.Runtime, sourceID, documentVersionID int64, contentSHA string, policyRevision int64, expiresAt *time.Time, subjectDocumentVersionID int64) int64 {
	t.Helper()
	now := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
	policy := createDocumentRightsPolicy(t, runtime, sourceID, policyRevision, now.Add(-time.Hour))
	return insertDocumentRightsDecision(t, runtime, policy, documentVersionID, contentSHA, "display_private", expiresAt, nil, subjectDocumentVersionID)
}

func insertDocumentRightsDecision(t *testing.T, runtime *database.Runtime, policy documentRightsPolicyFixture, documentVersionID int64, contentSHA, action string, expiresAt *time.Time, retentionDays *int, subjectDocumentVersionID int64) int64 {
	t.Helper()
	return insertDocumentRightsDecisionWithOutcome(t, runtime, policy, documentVersionID, contentSHA, action, "allow", expiresAt, retentionDays, subjectDocumentVersionID)
}

func insertDocumentRightsDecisionWithOutcome(t *testing.T, runtime *database.Runtime, policy documentRightsPolicyFixture, documentVersionID int64, contentSHA, action, decision string, expiresAt *time.Time, retentionDays *int, subjectDocumentVersionID int64) int64 {
	t.Helper()
	evaluatedAt := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
	expiresValue := ""
	if expiresAt != nil {
		expiresValue = expiresAt.UTC().Format(time.RFC3339Nano)
	}
	retentionValue := ""
	if retentionDays != nil {
		retentionValue = fmt.Sprint(*retentionDays)
	}
	idempotencyKey, commandFingerprint := documentRightsFixtureReceipt(
		"decision", fmt.Sprint(policy.SourceID), fmt.Sprint(policy.ID), fmt.Sprint(policy.Revision),
		fmt.Sprint(subjectDocumentVersionID), contentSHA, action, decision,
		evaluatedAt.Format(time.RFC3339Nano), expiresValue, retentionValue,
	)
	var decisionID int64
	if err := runtime.SQL.QueryRow(`
WITH decision_batch AS (
  INSERT INTO source_rights_decision_batches (
    source_connection_id,policy_id,expected_policy_version,subject_type,subject_key,input_digest,
    recorded_by_user_id,idempotency_key,command_fingerprint,decision_count
  )
  SELECT $1,$2,policy.version,'document_version',$8,$9,policy.recorded_by_user_id,$16,$17,1
  FROM source_rights_policies AS policy WHERE policy.id=$2
  RETURNING id
)
INSERT INTO source_rights_decisions (
  decision_batch_id,source_connection_id,policy_id,policy_revision,policy_scope_type,policy_scope_subject,
  priority_rank,basis_summary,subject_type,subject_key,input_digest,action,decision,
  evaluator,evaluated_at,effective_from,expires_at,retention_days
) SELECT decision_batch.id,$1,$2,$3,$4,$5,$6,$7,'document_version',$8,$9,$10,$11,
  'ingestion-document-test',$12,$13,$14,$15
FROM decision_batch RETURNING id`, policy.SourceID, policy.ID, policy.Revision, policy.ScopeType, policy.Subject,
		policy.Priority, policy.Basis, fmt.Sprintf("%d", subjectDocumentVersionID), contentSHA,
		action, decision, evaluatedAt, policy.EffectiveAt, expiresAt, retentionDays,
		idempotencyKey, commandFingerprint).Scan(&decisionID); err != nil {
		t.Fatalf("insert %s %s decision for document version %d (subject %d): %v", action, decision, documentVersionID, subjectDocumentVersionID, err)
	}
	return decisionID
}

func insertDocumentRightsFixtureActor(t *testing.T, runtime *database.Runtime, seed string) int64 {
	t.Helper()
	digest := documentRightsFixtureDigest("actor", seed)
	var actorID int64
	if err := runtime.SQL.QueryRow(`
INSERT INTO users (email,password_hash,display_name,role)
VALUES ($1,'fixture-not-a-credential','Document rights fixture operator','admin')
RETURNING id`, "document-rights-fixture-"+digest[:24]+"@example.test").Scan(&actorID); err != nil {
		t.Fatalf("insert document rights fixture actor: %v", err)
	}
	return actorID
}

func documentRightsFixtureReceipt(kind string, values ...string) (string, string) {
	fingerprint := documentRightsFixtureDigest(append([]string{kind}, values...)...)
	return "fixture." + kind + "." + fingerprint[:32], fingerprint
}

func documentRightsFixtureDigest(values ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(values, "\x00")))
	return fmt.Sprintf("%x", digest[:])
}
