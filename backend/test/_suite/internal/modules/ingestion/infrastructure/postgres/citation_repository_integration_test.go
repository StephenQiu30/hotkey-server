package postgres_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	ingestionpostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
)

func TestCitationRepositoryPinsExactVersionAndRechecksCurrentRights(t *testing.T) {
	runtime := openDocumentVersionRuntime(t)
	defer func() { _ = runtime.Close() }()
	sourceID := createDocumentVersionSource(t, runtime, "citation")
	firstObservationID := insertSourceObservation(t, runtime, sourceID, "stable-work", 0)
	secondObservationID := insertSourceObservation(t, runtime, sourceID, "stable-work", 1)
	reader := &integrationDocumentObservationReader{observations: map[int64]ingestionapplication.DocumentObservationDTO{
		firstObservationID: {
			ID: firstObservationID, SourceConnectionID: sourceID, ExternalWorkID: "stable-work",
			BodyOrigin: ingestionapplication.BodyOriginFeedContent, Completeness: ingestionapplication.BodyCompletenessFull,
			Body: "first immutable body", Language: "en", CapturedAt: documentVersionCapturedAt(0),
		},
		secondObservationID: {
			ID: secondObservationID, SourceConnectionID: sourceID, ExternalWorkID: "stable-work",
			BodyOrigin: ingestionapplication.BodyOriginFeedContent, Completeness: ingestionapplication.BodyCompletenessFull,
			Body: "second immutable body", Language: "en", CapturedAt: documentVersionCapturedAt(1),
		},
	}}
	versions := ingestionpostgres.NewDocumentVersionRepository(runtime)
	service, err := ingestionapplication.NewDocumentVersionService(ingestionapplication.DocumentVersionDependencies{Observations: reader, Versions: versions})
	if err != nil {
		t.Fatal(err)
	}

	first := persistCitationVersion(t, service, firstObservationID)
	firstArtifactSHA := strings.Repeat("c", 64)
	publishCitationVersion(t, runtime, service, sourceID, first, 101, firstArtifactSHA)
	firstDisplayID := createDocumentDisplayDecision(t, runtime, sourceID, first.DocumentVersion.ID,
		first.DocumentVersion.ContentSHA256, 103, nil, first.DocumentVersion.ID)
	transitionDocumentVersionWithDisplay(t, service, first.DocumentVersion.ID, 3, firstDisplayID)

	second := persistCitationVersion(t, service, secondObservationID)
	secondArtifactSHA := strings.Repeat("d", 64)
	publishCitationVersion(t, runtime, service, sourceID, second, 111, secondArtifactSHA)
	secondDisplayID := createDocumentDisplayDecision(t, runtime, sourceID, second.DocumentVersion.ID,
		second.DocumentVersion.ContentSHA256, 113, nil, second.DocumentVersion.ID)
	transitionDocumentVersionWithDisplay(t, service, second.DocumentVersion.ID, 3, secondDisplayID)

	var currentVersionID int64
	if err := runtime.SQL.QueryRow(`SELECT current_document_version_id FROM documents WHERE id=$1`, first.Document.ID).Scan(&currentVersionID); err != nil {
		t.Fatal(err)
	}
	if currentVersionID != second.DocumentVersion.ID {
		t.Fatalf("current document version = %d, want newer %d", currentVersionID, second.DocumentVersion.ID)
	}

	repository := ingestionpostgres.NewCitationRepository(runtime)
	read, err := repository.ReadCitation(context.Background(), first.DocumentVersion.ID)
	if err != nil {
		t.Fatalf("ReadCitation(first) error = %v", err)
	}
	if read.DocumentVersionID != first.DocumentVersion.ID || read.DocumentID != first.Document.ID ||
		read.Title != "revision 0" || read.ContentSHA256 != first.DocumentVersion.ContentSHA256 ||
		read.Artifact == nil || read.Artifact.SHA256 != firstArtifactSHA || !read.DisplayPrivateAllowed ||
		!read.Artifact.StoreDerivedAllowed || !read.Artifact.RetainAllowed {
		t.Fatalf("exact first-version projection = %#v", read)
	}
	if read.DocumentVersionID == currentVersionID || read.Artifact.SHA256 == secondArtifactSHA {
		t.Fatalf("exact read drifted to current version: %#v", read)
	}
	if _, err := runtime.SQL.Exec(`UPDATE source_connections SET enabled=false, deleted_at=now() WHERE id=$1`, sourceID); err != nil {
		t.Fatalf("archive source connection fixture: %v", err)
	}
	archivedSourceRead, err := repository.ReadCitation(context.Background(), first.DocumentVersion.ID)
	if err != nil {
		t.Fatalf("ReadCitation(archived source) error = %v", err)
	}
	if archivedSourceRead.DocumentVersionID != first.DocumentVersion.ID || !archivedSourceRead.DisplayPrivateAllowed ||
		archivedSourceRead.Artifact == nil || !archivedSourceRead.Artifact.StoreDerivedAllowed || !archivedSourceRead.Artifact.RetainAllowed {
		t.Fatalf("operational source archive revoked historical document rights: %#v", archivedSourceRead)
	}

	denyPolicy := createDocumentObservationRightsPolicy(t, runtime, sourceID, firstObservationID, 121, time.Now().UTC().Add(-time.Hour))
	insertDocumentRightsDecisionWithOutcome(t, runtime, denyPolicy, first.DocumentVersion.ID,
		first.DocumentVersion.ContentSHA256, "display_private", "deny", nil, nil, first.DocumentVersion.ID)
	revoked, err := repository.ReadCitation(context.Background(), first.DocumentVersion.ID)
	if err != nil {
		t.Fatalf("ReadCitation(revoked) error = %v", err)
	}
	if revoked.DisplayPrivateAllowed {
		t.Fatalf("revoked exact version remained display-allowed: %#v", revoked)
	}
	if revoked.Artifact == nil || !revoked.Artifact.StoreDerivedAllowed || !revoked.Artifact.RetainAllowed {
		t.Fatalf("display revocation incorrectly changed independent artifact rights: %#v", revoked.Artifact)
	}

	insertDocumentRightsDecisionWithOutcome(t, runtime, denyPolicy, first.DocumentVersion.ID,
		first.DocumentVersion.ContentSHA256, "store_derived", "deny", nil, nil, first.DocumentVersion.ID)
	retentionDays := 1
	insertDocumentRightsDecisionWithOutcome(t, runtime, denyPolicy, first.DocumentVersion.ID,
		first.DocumentVersion.ContentSHA256, "retain", "deny", nil, &retentionDays, first.DocumentVersion.ID)
	fullyRevoked, err := repository.ReadCitation(context.Background(), first.DocumentVersion.ID)
	if err != nil {
		t.Fatalf("ReadCitation(fully revoked) error = %v", err)
	}
	if fullyRevoked.Artifact == nil || fullyRevoked.Artifact.StoreDerivedAllowed || fullyRevoked.Artifact.RetainAllowed ||
		fullyRevoked.Artifact.CurrentRetentionDays != nil {
		t.Fatalf("current artifact rights were not re-evaluated fail-closed: %#v", fullyRevoked.Artifact)
	}
}

func persistCitationVersion(t *testing.T, service *ingestionapplication.DocumentVersionService, observationID int64) ingestionapplication.PersistDocumentVersionResult {
	t.Helper()
	result, err := service.PersistSourceObservation(context.Background(), ingestionapplication.PersistDocumentVersionCommand{
		SourceObservationID: observationID, ExtractorVersion: "rss-entry-v2",
		ExtractorProfileVersion: "rss-profile-v3", ExtractorProfileSHA256: strings.Repeat("a", 64),
	})
	if err != nil {
		t.Fatalf("PersistSourceObservation(%d) error = %v", observationID, err)
	}
	return result
}

func publishCitationVersion(t *testing.T, runtime *database.Runtime, service *ingestionapplication.DocumentVersionService, sourceID int64, persisted ingestionapplication.PersistDocumentVersionResult, policyRevision int64, artifactSHA string) {
	t.Helper()
	documentVersionID := persisted.DocumentVersion.ID
	transitionDocumentVersion(t, service, documentVersionID, 1, ingestionapplication.DocumentDerivedPending)
	now := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
	policy := createDocumentRightsPolicy(t, runtime, sourceID, policyRevision, now.Add(-time.Hour))
	storeDecisionID := insertDocumentRightsDecision(t, runtime, policy, documentVersionID,
		persisted.DocumentVersion.ContentSHA256, "store_derived", nil, nil, documentVersionID)
	retentionDays := 30
	retainDecisionID := insertDocumentRightsDecision(t, runtime, policy, documentVersionID,
		persisted.DocumentVersion.ContentSHA256, "retain", nil, &retentionDays, documentVersionID)
	profileSHA := strings.Repeat("b", 64)
	var artifactID int64
	if err := runtime.SQL.QueryRow(`
INSERT INTO derived_artifacts (
  source_connection_id,document_version_id,store_derived_rights_decision_id,
  retain_rights_decision_id,artifact_type,transformer_profile_sha256,
  vault_relative_path,mime_type,sha256,size_bytes,retention_until
) VALUES ($1,$2,$3,$4,'markdown',$5,$6,'text/markdown; charset=utf-8',$7,12,$8)
RETURNING id`, sourceID, documentVersionID, storeDecisionID, retainDecisionID, profileSHA,
		fmt.Sprintf("documents/%d/%d/markdown/%s.md", persisted.Document.ID, documentVersionID, profileSHA),
		artifactSHA, persisted.DocumentVersion.CapturedAt.Add(30*24*time.Hour)).Scan(&artifactID); err != nil {
		t.Fatalf("insert citation artifact: %v", err)
	}
	if _, err := runtime.SQL.Exec(`
UPDATE derived_artifacts
SET lifecycle_state='derived_available', available_at=now(), active=true, updated_at=now()
WHERE id=$1`, artifactID); err != nil {
		t.Fatalf("publish citation artifact: %v", err)
	}
	transitionDocumentVersion(t, service, documentVersionID, 2, ingestionapplication.DocumentDerivedAvailable)
}
