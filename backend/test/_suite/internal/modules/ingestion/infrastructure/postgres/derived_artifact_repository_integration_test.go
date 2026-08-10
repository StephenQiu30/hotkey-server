package postgres_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	ingestionpostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/infrastructure/postgres"
	knowledgeapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/application"
	knowledgevault "github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/infrastructure/vault"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

func TestDerivedArtifactSagaPublishesIdempotentlyAndMovesActiveProfileWithoutPersistingBody(t *testing.T) {
	runtime := openDocumentVersionRuntime(t)
	defer func() { _ = runtime.Close() }()
	fixture := createDerivedArtifactDocument(t, runtime, "publish", 40)
	storeDecisionID, retainDecisionID := createDerivedArtifactRights(t, runtime, fixture, 1)
	displayDecisionID := createDocumentDisplayDecision(
		t, runtime, fixture.sourceID, fixture.persisted.DocumentVersion.ID,
		fixture.persisted.DocumentVersion.ContentSHA256, 2, nil, fixture.persisted.DocumentVersion.ID,
	)
	vaultRoot := t.TempDir()
	publisher := newKnowledgeProjectionPublisher(t, vaultRoot)
	saga := newDerivedArtifactSaga(t, runtime, publisher, fixture.documentVersions)
	firstContent := []byte("# Archived\n\n正文来源一。\n")
	firstProfile := strings.Repeat("a", 64)
	command := derivedArtifactProjectCommand(
		fixture, firstProfile, firstContent, storeDecisionID, retainDecisionID, &displayDecisionID,
	)

	first, err := saga.Project(context.Background(), command)
	if err != nil {
		t.Fatalf("Project(first) error = %v", err)
	}
	if first.DocumentVersion.Version != 4 || first.DocumentVersion.LifecycleState != ingestionapplication.DocumentReadable ||
		!first.Artifact.Active || first.Artifact.LifecycleState != ingestionapplication.DerivedArtifactAvailable {
		t.Fatalf("Project(first) = %#v", first)
	}
	firstPath := derivedArtifactFixturePath(fixture.persisted.Document.ID, fixture.persisted.DocumentVersion.ID, firstProfile)
	assertProjectionFile(t, vaultRoot, firstPath, firstContent)
	var blockCount int
	var plaintextStart, plaintextEnd, markdownStart, markdownEnd int64
	var anchor, mapSHA string
	if err := runtime.SQL.QueryRow(`
SELECT count(*),min(plaintext_utf8_byte_start),max(plaintext_utf8_byte_end),
       min(markdown_utf8_byte_start),max(markdown_utf8_byte_end),min(markdown_anchor),min(anchor_map_sha256)
FROM document_anchor_blocks WHERE derived_artifact_id=$1`, first.Artifact.ID).Scan(
		&blockCount, &plaintextStart, &plaintextEnd, &markdownStart, &markdownEnd, &anchor, &mapSHA,
	); err != nil {
		t.Fatalf("read immutable anchor blocks: %v", err)
	}
	if blockCount != 1 || plaintextStart != 0 || plaintextEnd != int64(len("authorized normalized document body")) ||
		markdownStart != 0 || markdownEnd != int64(len(firstContent)) || anchor != command.AnchorMap.Blocks[0].MarkdownAnchor ||
		first.Artifact.AnchorMap == nil || mapSHA != first.Artifact.AnchorMap.AnchorMapSHA256 {
		t.Fatalf("persisted anchor facts = count %d plain %d..%d markdown %d..%d anchor %q map %q artifact %#v", blockCount, plaintextStart, plaintextEnd, markdownStart, markdownEnd, anchor, mapSHA, first.Artifact.AnchorMap)
	}
	if _, err := runtime.SQL.Exec(`UPDATE document_anchor_blocks SET markdown_anchor='body-0000-000000000000' WHERE derived_artifact_id=$1`, first.Artifact.ID); err == nil {
		t.Fatal("document anchor block update was accepted")
	}
	if _, err := runtime.SQL.Exec(`DELETE FROM document_anchor_blocks WHERE derived_artifact_id=$1`, first.Artifact.ID); err == nil {
		t.Fatal("document anchor block deletion was accepted")
	}

	retried, err := saga.Project(context.Background(), command)
	if err != nil || retried.Artifact.ID != first.Artifact.ID || retried.DocumentVersion.Version != first.DocumentVersion.Version {
		t.Fatalf("Project(retry) = %#v/%v, want stable artifact and document versions", retried, err)
	}

	secondContent := []byte("# Archived\n\n正文来源一，转换规则二。\n")
	secondProfile := strings.Repeat("b", 64)
	secondCommand := command
	secondCommand.ExpectedDocumentVersion = retried.DocumentVersion.Version
	secondCommand.TransformerProfileSHA256 = secondProfile
	secondCommand.ProjectionBytes = secondContent
	secondCommand.AnchorMap = derivedArtifactAnchorMap(fixture, secondContent)
	second, err := saga.Project(context.Background(), secondCommand)
	if err != nil {
		t.Fatalf("Project(new profile) error = %v", err)
	}
	if second.Artifact.ID == first.Artifact.ID || !second.Artifact.Active || second.DocumentVersion.Version != retried.DocumentVersion.Version {
		t.Fatalf("Project(new profile) = %#v", second)
	}
	secondPath := derivedArtifactFixturePath(fixture.persisted.Document.ID, fixture.persisted.DocumentVersion.ID, secondProfile)
	assertProjectionFile(t, vaultRoot, secondPath, secondContent)
	assertProjectionFile(t, vaultRoot, firstPath, firstContent)

	rows, err := runtime.SQL.Query(`
SELECT transformer_profile_sha256,active,lifecycle_state
FROM derived_artifacts
WHERE document_version_id=$1 AND artifact_type='markdown'
ORDER BY id`, fixture.persisted.DocumentVersion.ID)
	if err != nil {
		t.Fatalf("query derived artifacts: %v", err)
	}
	defer rows.Close()
	type artifactState struct {
		profile, lifecycle string
		active             bool
	}
	states := make([]artifactState, 0, 2)
	for rows.Next() {
		var state artifactState
		if err := rows.Scan(&state.profile, &state.active, &state.lifecycle); err != nil {
			t.Fatalf("scan derived artifact state: %v", err)
		}
		states = append(states, state)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate derived artifact states: %v", err)
	}
	if len(states) != 2 || states[0] != (artifactState{profile: firstProfile, lifecycle: "derived_available", active: false}) ||
		states[1] != (artifactState{profile: secondProfile, lifecycle: "derived_available", active: true}) {
		t.Fatalf("derived artifact states = %#v", states)
	}

	var bodyColumns int
	if err := runtime.SQL.QueryRow(`
SELECT count(*) FROM information_schema.columns
WHERE table_schema='public' AND table_name='derived_artifacts'
  AND column_name IN ('body','content','markdown','plaintext','projection_bytes')`).Scan(&bodyColumns); err != nil {
		t.Fatalf("inspect derived artifact columns: %v", err)
	}
	if bodyColumns != 0 {
		t.Fatalf("derived_artifacts body-like columns = %d, want 0", bodyColumns)
	}
}

func TestDerivedArtifactSagaQuarantinesSamePathWithDifferentBytes(t *testing.T) {
	runtime := openDocumentVersionRuntime(t)
	defer func() { _ = runtime.Close() }()
	fixture := createDerivedArtifactDocument(t, runtime, "conflict", 41)
	storeDecisionID, retainDecisionID := createDerivedArtifactRights(t, runtime, fixture, 1)
	vaultRoot := t.TempDir()
	saga := newDerivedArtifactSaga(t, runtime, newKnowledgeProjectionPublisher(t, vaultRoot), fixture.documentVersions)
	profile := strings.Repeat("c", 64)
	original := []byte("# Archived\n\n不可变正文。\n")
	command := derivedArtifactProjectCommand(fixture, profile, original, storeDecisionID, retainDecisionID, nil)
	first, err := saga.Project(context.Background(), command)
	if err != nil {
		t.Fatalf("Project(first) error = %v", err)
	}

	conflicting := command
	conflicting.ExpectedDocumentVersion = first.DocumentVersion.Version
	conflicting.ProjectionBytes = []byte("# Archived\n\n冲突正文。\n")
	conflicting.AnchorMap = derivedArtifactAnchorMap(fixture, conflicting.ProjectionBytes)
	if _, err := saga.Project(context.Background(), conflicting); !errors.Is(err, sharedrepository.ErrConflict) {
		t.Fatalf("Project(conflict) error = %v, want conflict", err)
	}
	var artifactState, documentState string
	var active bool
	if err := runtime.SQL.QueryRow(`
SELECT lifecycle_state,active FROM derived_artifacts WHERE id=$1`, first.Artifact.ID).Scan(&artifactState, &active); err != nil {
		t.Fatalf("read quarantined artifact: %v", err)
	}
	if err := runtime.SQL.QueryRow(`
SELECT lifecycle_state FROM document_versions WHERE id=$1`, fixture.persisted.DocumentVersion.ID).Scan(&documentState); err != nil {
		t.Fatalf("read quarantined document version: %v", err)
	}
	if artifactState != "quarantined" || active || documentState != "quarantined" {
		t.Fatalf("artifact/document conflict states = %s active=%t / %s", artifactState, active, documentState)
	}
	assertProjectionFile(t, vaultRoot,
		derivedArtifactFixturePath(fixture.persisted.Document.ID, fixture.persisted.DocumentVersion.ID, profile), original)
}

func TestDerivedArtifactSagaFailsClosedWhenRightsChangeAfterVaultPublish(t *testing.T) {
	runtime := openDocumentVersionRuntime(t)
	defer func() { _ = runtime.Close() }()
	fixture := createDerivedArtifactDocument(t, runtime, "rights-toctou", 42)
	storeDecisionID, retainDecisionID := createDerivedArtifactRights(t, runtime, fixture, 1)
	vaultRoot := t.TempDir()
	inner := newKnowledgeProjectionPublisher(t, vaultRoot)
	revoking := &rightsRevokingProjectionPublisher{inner: inner, revoke: func() {
		policy := createDocumentObservationRightsPolicy(
			t, runtime, fixture.sourceID, fixture.observationID, 2, time.Now().UTC().Add(-time.Hour),
		)
		insertDocumentRightsDecisionWithOutcome(
			t, runtime, policy, fixture.persisted.DocumentVersion.ID, fixture.persisted.DocumentVersion.ContentSHA256,
			"store_derived", "deny", nil, nil, fixture.persisted.DocumentVersion.ID,
		)
	}}
	saga := newDerivedArtifactSaga(t, runtime, revoking, fixture.documentVersions)
	profile := strings.Repeat("d", 64)
	content := []byte("# Archived\n\n授权在对象写入后被撤销。\n")
	command := derivedArtifactProjectCommand(fixture, profile, content, storeDecisionID, retainDecisionID, nil)

	if _, err := saga.Project(context.Background(), command); !errors.Is(err, sharedrepository.ErrConflict) {
		t.Fatalf("Project(rights revoked) error = %v, want conflict", err)
	}
	assertProjectionFile(t, vaultRoot,
		derivedArtifactFixturePath(fixture.persisted.Document.ID, fixture.persisted.DocumentVersion.ID, profile), content)
	assertDerivedArtifactFailureStates(t, runtime, fixture.persisted.DocumentVersion.ID, "artifact_commit_failed")
}

func TestDerivedArtifactSagaDoesNotMoveActivePointerAfterReadableDisplayRevocation(t *testing.T) {
	runtime := openDocumentVersionRuntime(t)
	defer func() { _ = runtime.Close() }()
	fixture := createDerivedArtifactDocument(t, runtime, "display-toctou", 44)
	storeDecisionID, retainDecisionID := createDerivedArtifactRights(t, runtime, fixture, 1)
	displayDecisionID := createDocumentDisplayDecision(
		t, runtime, fixture.sourceID, fixture.persisted.DocumentVersion.ID,
		fixture.persisted.DocumentVersion.ContentSHA256, 2, nil, fixture.persisted.DocumentVersion.ID,
	)
	vaultRoot := t.TempDir()
	inner := newKnowledgeProjectionPublisher(t, vaultRoot)
	initialProfile := strings.Repeat("1", 64)
	initialCommand := derivedArtifactProjectCommand(
		fixture, initialProfile, []byte("# Archived\n\n初始可读正文。\n"),
		storeDecisionID, retainDecisionID, &displayDecisionID,
	)
	saga := newDerivedArtifactSaga(t, runtime, inner, fixture.documentVersions)
	readable, err := saga.Project(context.Background(), initialCommand)
	if err != nil {
		t.Fatalf("Project(initial readable) error = %v", err)
	}
	revoking := &rightsRevokingProjectionPublisher{inner: inner, revoke: func() {
		policy := createDocumentObservationRightsPolicy(
			t, runtime, fixture.sourceID, fixture.observationID, 3, time.Now().UTC().Add(-time.Hour),
		)
		insertDocumentRightsDecisionWithOutcome(
			t, runtime, policy, fixture.persisted.DocumentVersion.ID, fixture.persisted.DocumentVersion.ContentSHA256,
			"display_private", "deny", nil, nil, fixture.persisted.DocumentVersion.ID,
		)
	}}
	changedProfile := strings.Repeat("2", 64)
	changedContent := []byte("# Archived\n\n撤权后的新转换正文。\n")
	changedCommand := initialCommand
	changedCommand.ExpectedDocumentVersion = readable.DocumentVersion.Version
	changedCommand.TransformerProfileSHA256 = changedProfile
	changedCommand.ProjectionBytes = changedContent
	changedCommand.AnchorMap = derivedArtifactAnchorMap(fixture, changedContent)
	changedCommand.DisplayPrivateRightsDecisionID = nil
	changedSaga := newDerivedArtifactSaga(t, runtime, revoking, fixture.documentVersions)
	if _, err := changedSaga.Project(context.Background(), changedCommand); !errors.Is(err, sharedrepository.ErrConflict) {
		t.Fatalf("Project(display revoked) error = %v, want conflict", err)
	}
	assertProjectionFile(t, vaultRoot,
		derivedArtifactFixturePath(fixture.persisted.Document.ID, fixture.persisted.DocumentVersion.ID, changedProfile), changedContent)

	rows, err := runtime.SQL.Query(`
SELECT transformer_profile_sha256,lifecycle_state,active,COALESCE(failure_code,'')
FROM derived_artifacts WHERE document_version_id=$1 ORDER BY id`, fixture.persisted.DocumentVersion.ID)
	if err != nil {
		t.Fatalf("query display-revoked artifacts: %v", err)
	}
	defer rows.Close()
	type state struct {
		profile, lifecycle string
		active             bool
		failureCode        string
	}
	states := make([]state, 0, 2)
	for rows.Next() {
		var item state
		if err := rows.Scan(&item.profile, &item.lifecycle, &item.active, &item.failureCode); err != nil {
			t.Fatalf("scan display-revoked artifact: %v", err)
		}
		states = append(states, item)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate display-revoked artifacts: %v", err)
	}
	if len(states) != 2 || states[0].profile != initialProfile || states[0].lifecycle != "derived_available" || !states[0].active ||
		states[1].profile != changedProfile || states[1].lifecycle != "derive_failed" || states[1].active ||
		states[1].failureCode != "artifact_commit_failed" {
		t.Fatalf("display-revoked artifact states = %#v", states)
	}
	var documentState string
	if err := runtime.SQL.QueryRow(`SELECT lifecycle_state FROM document_versions WHERE id=$1`, fixture.persisted.DocumentVersion.ID).Scan(&documentState); err != nil {
		t.Fatalf("read display-revoked document state: %v", err)
	}
	if documentState != "readable" {
		t.Fatalf("display-revoked projection attempt changed document to %s", documentState)
	}
}

func TestDerivedArtifactSagaMarksRealVaultFailureWithoutQuarantine(t *testing.T) {
	runtime := openDocumentVersionRuntime(t)
	defer func() { _ = runtime.Close() }()
	fixture := createDerivedArtifactDocument(t, runtime, "vault-failure", 43)
	storeDecisionID, retainDecisionID := createDerivedArtifactRights(t, runtime, fixture, 1)
	vaultRoot := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(vaultRoot, []byte("fixture"), 0o600); err != nil {
		t.Fatalf("create invalid Vault root: %v", err)
	}
	saga := newDerivedArtifactSaga(t, runtime, newKnowledgeProjectionPublisher(t, vaultRoot), fixture.documentVersions)
	command := derivedArtifactProjectCommand(
		fixture, strings.Repeat("e", 64), []byte("# Archived\n\nVault failure.\n"),
		storeDecisionID, retainDecisionID, nil,
	)

	if _, err := saga.Project(context.Background(), command); !errors.Is(err, knowledgeapplication.ErrProjectionUnavailable) {
		t.Fatalf("Project(Vault failure) error = %v, want unavailable", err)
	}
	assertDerivedArtifactFailureStates(t, runtime, fixture.persisted.DocumentVersion.ID, "vault_publish_failed")
	var failedArtifactID, failedDocumentVersion int64
	if err := runtime.SQL.QueryRow(`
SELECT artifact.id,version.version
FROM derived_artifacts AS artifact
JOIN document_versions AS version ON version.id=artifact.document_version_id
WHERE artifact.document_version_id=$1`, fixture.persisted.DocumentVersion.ID).Scan(&failedArtifactID, &failedDocumentVersion); err != nil {
		t.Fatalf("read failed saga versions: %v", err)
	}
	if err := os.Remove(vaultRoot); err != nil {
		t.Fatalf("remove invalid Vault root: %v", err)
	}
	if err := os.Mkdir(vaultRoot, 0o700); err != nil {
		t.Fatalf("repair Vault root: %v", err)
	}
	command.ExpectedDocumentVersion = failedDocumentVersion
	retry := newDerivedArtifactSaga(t, runtime, newKnowledgeProjectionPublisher(t, vaultRoot), fixture.documentVersions)
	recovered, err := retry.Project(context.Background(), command)
	if err != nil {
		t.Fatalf("Project(retry after Vault recovery) error = %v", err)
	}
	if recovered.Artifact.ID != failedArtifactID || recovered.Artifact.LifecycleState != ingestionapplication.DerivedArtifactAvailable ||
		recovered.DocumentVersion.LifecycleState != ingestionapplication.DocumentDerivedAvailable {
		t.Fatalf("Project(retry after Vault recovery) = %#v", recovered)
	}
}

type derivedArtifactDocumentFixture struct {
	sourceID, observationID int64
	persisted               ingestionapplication.PersistDocumentVersionResult
	documentVersions        *ingestionapplication.DocumentVersionService
}

func createDerivedArtifactDocument(t *testing.T, runtime *database.Runtime, suffix string, index int) derivedArtifactDocumentFixture {
	t.Helper()
	sourceID := createDocumentVersionSource(t, runtime, suffix)
	externalID := "derived-artifact-" + suffix
	observationID := insertSourceObservation(t, runtime, sourceID, externalID, index)
	reader := &integrationDocumentObservationReader{observations: map[int64]ingestionapplication.DocumentObservationDTO{
		observationID: {
			ID: observationID, SourceConnectionID: sourceID, ExternalWorkID: externalID,
			BodyOrigin: ingestionapplication.BodyOriginFeedContent, Completeness: ingestionapplication.BodyCompletenessFull,
			Body: "authorized normalized document body", Language: "en", CapturedAt: documentVersionCapturedAt(index),
		},
	}}
	documentVersions, err := ingestionapplication.NewDocumentVersionService(ingestionapplication.DocumentVersionDependencies{
		Observations: reader, Versions: ingestionpostgres.NewDocumentVersionRepository(runtime),
	})
	if err != nil {
		t.Fatalf("NewDocumentVersionService() error = %v", err)
	}
	persisted, err := documentVersions.PersistSourceObservation(context.Background(), ingestionapplication.PersistDocumentVersionCommand{
		SourceObservationID: observationID, ExtractorVersion: "rss-entry-v2",
		ExtractorProfileVersion: "rss-profile-v3", ExtractorProfileSHA256: strings.Repeat("f", 64),
	})
	if err != nil {
		t.Fatalf("PersistSourceObservation() error = %v", err)
	}
	return derivedArtifactDocumentFixture{
		sourceID: sourceID, observationID: observationID, persisted: persisted, documentVersions: documentVersions,
	}
}

func createDerivedArtifactRights(t *testing.T, runtime *database.Runtime, fixture derivedArtifactDocumentFixture, policyRevision int64) (int64, int64) {
	t.Helper()
	policy := createDocumentRightsPolicy(t, runtime, fixture.sourceID, policyRevision, time.Now().UTC().Add(-time.Hour))
	documentVersionID := fixture.persisted.DocumentVersion.ID
	contentSHA := fixture.persisted.DocumentVersion.ContentSHA256
	storeDecisionID := insertDocumentRightsDecision(
		t, runtime, policy, documentVersionID, contentSHA, "store_derived", nil, nil, documentVersionID,
	)
	retentionDays := 30
	retainDecisionID := insertDocumentRightsDecision(
		t, runtime, policy, documentVersionID, contentSHA, "retain", nil, &retentionDays, documentVersionID,
	)
	return storeDecisionID, retainDecisionID
}

func newKnowledgeProjectionPublisher(t *testing.T, root string) knowledgeapplication.ProjectionPublisher {
	t.Helper()
	service, err := knowledgeapplication.NewProjectionService(knowledgevault.NewWriter(root))
	if err != nil {
		t.Fatalf("NewProjectionService() error = %v", err)
	}
	return service
}

func newDerivedArtifactSaga(t *testing.T, runtime *database.Runtime, publisher knowledgeapplication.ProjectionPublisher, documentVersions *ingestionapplication.DocumentVersionService) *ingestionapplication.DocumentProjectionService {
	t.Helper()
	service, err := ingestionapplication.NewDocumentProjectionService(ingestionapplication.DocumentProjectionDependencies{
		Publisher: publisher, Repository: ingestionpostgres.NewDerivedArtifactRepository(runtime), DocumentVersions: documentVersions,
	})
	if err != nil {
		t.Fatalf("NewDocumentProjectionService() error = %v", err)
	}
	return service
}

func derivedArtifactProjectCommand(fixture derivedArtifactDocumentFixture, profile string, content []byte, storeDecisionID, retainDecisionID int64, displayDecisionID *int64) ingestionapplication.ProjectDocumentCommand {
	return ingestionapplication.ProjectDocumentCommand{
		DocumentVersionID:       fixture.persisted.DocumentVersion.ID,
		ExpectedDocumentVersion: fixture.persisted.DocumentVersion.Version,
		ArtifactType:            ingestionapplication.DocumentProjectionMarkdown, TransformerProfileSHA256: profile,
		StoreDerivedRightsDecisionID: storeDecisionID, RetainRightsDecisionID: retainDecisionID,
		DisplayPrivateRightsDecisionID: displayDecisionID, ProjectionBytes: append([]byte(nil), content...),
		AnchorMap: derivedArtifactAnchorMap(fixture, content),
	}
}

func derivedArtifactAnchorMap(fixture derivedArtifactDocumentFixture, content []byte) *ingestionapplication.ProjectDocumentAnchorMapCommand {
	plaintext := "authorized normalized document body"
	mapResult := ingestionapplication.MapDocumentTextResult{
		Plaintext: plaintext, NormalizationVersion: ingestionapplication.CanonicalDocumentTextNormalizationVersion,
		AnchorMapProfileVersion: ingestionapplication.CanonicalDocumentAnchorMapProfileVersion,
		PlaintextSHA256:         fixture.persisted.DocumentVersion.ContentSHA256,
		MarkdownSHA256:          fmt.Sprintf("%x", sha256.Sum256(content)),
		Blocks: []ingestionapplication.DocumentAnchorBlockDTO{{
			Ordinal: 0, PlaintextUTF8ByteStart: 0, PlaintextUTF8ByteEnd: int64(len(plaintext)),
			MarkdownUTF8ByteStart: 0, MarkdownUTF8ByteEnd: int64(len(content)),
			MarkdownAnchor: ingestionapplication.DocumentMarkdownAnchor(0, plaintext),
		}},
	}
	mapResult.AnchorMapSHA256 = ingestionapplication.DocumentAnchorMapSHA256(mapResult)
	return &ingestionapplication.ProjectDocumentAnchorMapCommand{
		Plaintext: mapResult.Plaintext, NormalizationVersion: mapResult.NormalizationVersion,
		AnchorMapProfileVersion: mapResult.AnchorMapProfileVersion, PlaintextSHA256: mapResult.PlaintextSHA256,
		MarkdownSHA256: mapResult.MarkdownSHA256, AnchorMapSHA256: mapResult.AnchorMapSHA256,
		Blocks: append([]ingestionapplication.DocumentAnchorBlockDTO(nil), mapResult.Blocks...),
	}
}

func derivedArtifactFixturePath(documentID, documentVersionID int64, profile string) string {
	return fmt.Sprintf("documents/%d/%d/markdown/%s.md", documentID, documentVersionID, profile)
}

func assertProjectionFile(t *testing.T, root, relativePath string, expected []byte) {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relativePath)))
	if err != nil {
		t.Fatalf("read projection %s: %v", relativePath, err)
	}
	if string(content) != string(expected) {
		t.Fatalf("projection %s content changed", relativePath)
	}
}

func assertDerivedArtifactFailureStates(t *testing.T, runtime *database.Runtime, documentVersionID int64, expectedFailureCode string) {
	t.Helper()
	var artifactState, failureCode, documentState string
	var active bool
	if err := runtime.SQL.QueryRow(`
SELECT lifecycle_state,failure_code,active
FROM derived_artifacts WHERE document_version_id=$1`, documentVersionID).Scan(&artifactState, &failureCode, &active); err != nil {
		t.Fatalf("read failed derived artifact: %v", err)
	}
	if err := runtime.SQL.QueryRow(`
SELECT lifecycle_state FROM document_versions WHERE id=$1`, documentVersionID).Scan(&documentState); err != nil {
		t.Fatalf("read failed document version: %v", err)
	}
	if artifactState != "derive_failed" || failureCode != expectedFailureCode || active || documentState != "derive_failed" {
		t.Fatalf("artifact/document failure states = %s/%s active=%t / %s", artifactState, failureCode, active, documentState)
	}
}

type rightsRevokingProjectionPublisher struct {
	inner  knowledgeapplication.ProjectionPublisher
	revoke func()
	once   sync.Once
}

func (publisher *rightsRevokingProjectionPublisher) Publish(ctx context.Context, command knowledgeapplication.PublishProjectionCommand) (knowledgeapplication.PublishProjectionResult, error) {
	result, err := publisher.inner.Publish(ctx, command)
	if err == nil {
		publisher.once.Do(publisher.revoke)
	}
	return result, err
}
