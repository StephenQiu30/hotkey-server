package postgres_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	ingestiondomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/domain"
	ingestionpostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

func TestContentFamilyRepositoryCreatesJoinsAndReplaysImmutableLineage(t *testing.T) {
	runtime := openDocumentVersionRuntime(t)
	defer runtime.Close()
	firstDocument := createDerivedArtifactDocument(t, runtime, "family-root", 201)
	secondDocument := createDerivedArtifactDocument(t, runtime, "family-copy", 202)
	firstArtifact, firstStore, firstRetain := createContentFamilyPlaintext(t, runtime, firstDocument, 21)
	secondArtifact, secondStore, secondRetain := createContentFamilyPlaintext(t, runtime, secondDocument, 22)
	repository, err := ingestionpostgres.NewContentFamilyRepository(runtime)
	if err != nil {
		t.Fatal(err)
	}
	service, err := ingestionapplication.NewContentFamilyService(repository)
	if err != nil {
		t.Fatal(err)
	}
	plaintext := "authorized normalized document body"
	now := time.Now().UTC().Truncate(time.Microsecond)
	root, err := service.Assign(context.Background(), ingestionapplication.AssignDocumentContentFamilyCommand{
		SourceConnectionID: firstDocument.sourceID, DocumentVersionID: firstDocument.persisted.DocumentVersion.ID,
		DerivedArtifactID: firstArtifact.ID, StoreDerivedRightsDecisionID: firstStore, RetainRightsDecisionID: firstRetain,
		DecisionAt: now, RetentionUntil: firstArtifact.RetentionUntil, CanonicalPlaintext: plaintext,
		FingerprintProfile: "content-fingerprint-v1", DecisionProfileVersion: "content-family-decision-v1",
	})
	if err != nil || root.Decision.Action != "create" || root.Decision.RootDocumentVersionID != firstDocument.persisted.DocumentVersion.ID {
		t.Fatalf("root assignment = %#v / %v", root, err)
	}
	copyDecision, err := service.Assign(context.Background(), ingestionapplication.AssignDocumentContentFamilyCommand{
		SourceConnectionID: secondDocument.sourceID, DocumentVersionID: secondDocument.persisted.DocumentVersion.ID,
		DerivedArtifactID: secondArtifact.ID, StoreDerivedRightsDecisionID: secondStore, RetainRightsDecisionID: secondRetain,
		DecisionAt: now, RetentionUntil: secondArtifact.RetentionUntil, CanonicalPlaintext: plaintext,
		FingerprintProfile: "content-fingerprint-v1", DecisionProfileVersion: "content-family-decision-v1",
	})
	if err != nil || copyDecision.Decision.Action != "join" || copyDecision.Decision.Relation != "exact_copy" ||
		copyDecision.Decision.FamilyID != root.Decision.FamilyID || copyDecision.Decision.FamilyVersion != 2 {
		t.Fatalf("copy assignment = %#v / %v", copyDecision, err)
	}
	replayed, err := service.Assign(context.Background(), ingestionapplication.AssignDocumentContentFamilyCommand{
		SourceConnectionID: secondDocument.sourceID, DocumentVersionID: secondDocument.persisted.DocumentVersion.ID,
		DerivedArtifactID: secondArtifact.ID, StoreDerivedRightsDecisionID: secondStore, RetainRightsDecisionID: secondRetain,
		DecisionAt: now, RetentionUntil: secondArtifact.RetentionUntil, CanonicalPlaintext: plaintext,
		FingerprintProfile: "content-fingerprint-v1", DecisionProfileVersion: "content-family-decision-v1",
	})
	if err != nil || replayed.Decision.DecisionID != copyDecision.Decision.DecisionID || replayed.Decision.FamilyVersion != 2 {
		t.Fatalf("replay = %#v / %v", replayed, err)
	}
	if !replayed.Decision.Reused || copyDecision.Decision.Reused {
		t.Fatalf("created/replayed receipt flags = %#v / %#v", copyDecision.Decision, replayed.Decision)
	}

	var fingerprints, families, decisions, members int
	for table, destination := range map[string]*int{
		"content_fingerprints": &fingerprints, "content_families": &families,
		"content_lineage_decisions": &decisions, "content_family_members": &members,
	} {
		if err := runtime.SQL.QueryRow(`SELECT count(*) FROM ` + table).Scan(destination); err != nil {
			t.Fatal(err)
		}
	}
	if fingerprints != 2 || families != 1 || decisions != 2 || members != 2 {
		t.Fatalf("fingerprints/families/decisions/members = %d/%d/%d/%d", fingerprints, families, decisions, members)
	}
	if _, err := runtime.SQL.Exec(`UPDATE content_lineage_decisions SET relation='near_duplicate' WHERE id=$1`, copyDecision.Decision.DecisionID); err == nil {
		t.Fatal("append-only content lineage decision accepted mutation")
	}
	for _, table := range []string{"content_fingerprints", "content_families", "content_lineage_decisions", "content_family_members"} {
		var count int
		if err := runtime.SQL.QueryRow(`SELECT count(*) FROM information_schema.columns WHERE table_schema='public' AND table_name=$1 AND column_name IN ('body','content','plaintext','markdown')`, table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("%s exposes %d plaintext-like columns", table, count)
		}
	}
}

func TestContentFamilyRepositoryRejectsConflictingIdempotencyReceipt(t *testing.T) {
	runtime := openDocumentVersionRuntime(t)
	defer runtime.Close()
	fixture := createDerivedArtifactDocument(t, runtime, "family-conflict", 203)
	artifact, storeDecisionID, retainDecisionID := createContentFamilyPlaintext(t, runtime, fixture, 23)
	repository, err := ingestionpostgres.NewContentFamilyRepository(runtime)
	if err != nil {
		t.Fatal(err)
	}
	fingerprint, err := ingestiondomain.BuildContentFingerprint("immutable family input", "content-fingerprint-v1")
	if err != nil {
		t.Fatal(err)
	}
	command := ingestionapplication.CommitContentFamilyDecisionCommand{
		SourceConnectionID: fixture.sourceID, DocumentVersionID: fixture.persisted.DocumentVersion.ID,
		DerivedArtifactID: artifact.ID, StoreDerivedRightsDecisionID: storeDecisionID, RetainRightsDecisionID: retainDecisionID,
		DecisionAt: time.Now().UTC().Truncate(time.Microsecond), RetentionUntil: artifact.RetentionUntil,
		Fingerprint: ingestionapplication.ContentFingerprintDTO{ProfileVersion: fingerprint.ProfileVersion,
			NormalizedContentSHA256: fingerprint.NormalizedContentSHA256, SimHashHex: fingerprint.SimHashHex, MinHash: fingerprint.MinHash},
		Action: "create", Relation: "unrelated", DecisionProfileVersion: "content-family-decision-v1",
		ReasonCodes: []string{"no_candidate"}, IdempotencyKey: "content-family-conflict-key",
		CommandFingerprint: strings.Repeat("a", 64),
	}
	if _, err := repository.CommitContentFamilyDecision(context.Background(), command); err != nil {
		t.Fatal(err)
	}
	command.CommandFingerprint = strings.Repeat("b", 64)
	if _, err := repository.CommitContentFamilyDecision(context.Background(), command); !errors.Is(err, sharedrepository.ErrConflict) {
		t.Fatalf("conflicting replay error = %v, want conflict", err)
	}
}

func TestContentLineageFeedbackRepositorySplitsReparentsReplaysAndRejectsCycles(t *testing.T) {
	runtime := openDocumentVersionRuntime(t)
	defer runtime.Close()
	rootDocument := createDerivedArtifactDocument(t, runtime, "lineage-feedback-root", 231)
	childDocument := createDerivedArtifactDocument(t, runtime, "lineage-feedback-child", 232)
	rootArtifact, rootStore, rootRetain := createContentFamilyPlaintext(t, runtime, rootDocument, 51)
	childArtifact, childStore, childRetain := createContentFamilyPlaintext(t, runtime, childDocument, 52)
	repository, err := ingestionpostgres.NewContentFamilyRepository(runtime)
	if err != nil {
		t.Fatal(err)
	}
	service, err := ingestionapplication.NewContentFamilyService(repository)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	plaintext := "same licensed publisher body for manual lineage review"
	root, err := service.Assign(context.Background(), ingestionapplication.AssignDocumentContentFamilyCommand{
		SourceConnectionID: rootDocument.sourceID, DocumentVersionID: rootDocument.persisted.DocumentVersion.ID,
		DerivedArtifactID: rootArtifact.ID, StoreDerivedRightsDecisionID: rootStore, RetainRightsDecisionID: rootRetain,
		DecisionAt: now, RetentionUntil: rootArtifact.RetentionUntil, CanonicalPlaintext: plaintext,
		FingerprintProfile: "content-fingerprint-v1", DecisionProfileVersion: "content-family-decision-v1",
	})
	if err != nil {
		t.Fatal(err)
	}
	child, err := service.Assign(context.Background(), ingestionapplication.AssignDocumentContentFamilyCommand{
		SourceConnectionID: childDocument.sourceID, DocumentVersionID: childDocument.persisted.DocumentVersion.ID,
		DerivedArtifactID: childArtifact.ID, StoreDerivedRightsDecisionID: childStore, RetainRightsDecisionID: childRetain,
		DecisionAt: now, RetentionUntil: childArtifact.RetentionUntil, CanonicalPlaintext: plaintext,
		FingerprintProfile: "content-fingerprint-v1", DecisionProfileVersion: "content-family-decision-v1",
	})
	if err != nil || child.Decision.Relation != "exact_copy" {
		t.Fatalf("child assignment = %#v / %v", child, err)
	}
	editorID := insertContentLineageReviewer(t, runtime, "lineage-feedback-editor")
	feedbacks, err := ingestionapplication.NewContentLineageFeedbackService(repository)
	if err != nil {
		t.Fatal(err)
	}
	splitCommand := ingestionapplication.ReviewContentLineageCommand{
		ActorUserID: editorID, LineageDecisionID: child.Decision.DecisionID, ExpectedMemberVersion: 1,
		FeedbackType: "not_duplicate", ReasonCode: "different_underlying_work", Note: "reviewed against the original body",
		IdempotencyKey: "content-lineage-split-child",
	}
	split, err := feedbacks.Review(context.Background(), splitCommand)
	if err != nil {
		t.Fatalf("split feedback: %v", err)
	}
	if split.Feedback.ResultFamilyID == root.Decision.FamilyID || split.Feedback.ResultRelation != "unrelated" || split.Feedback.Reused {
		t.Fatalf("split feedback = %#v", split)
	}
	replayed, err := feedbacks.Review(context.Background(), splitCommand)
	if err != nil || !replayed.Feedback.Reused || replayed.Feedback.FeedbackID != split.Feedback.FeedbackID {
		t.Fatalf("split replay = %#v / %v", replayed, err)
	}

	reparent, err := feedbacks.Review(context.Background(), ingestionapplication.ReviewContentLineageCommand{
		ActorUserID: editorID, LineageDecisionID: split.Feedback.ResultLineageDecisionID, ExpectedMemberVersion: 1,
		FeedbackType: "relation_override", RelationOverride: "revision_of",
		TargetParentDocumentVersionID: rootDocument.persisted.DocumentVersion.ID, ExpectedTargetMemberVersion: 1,
		ReasonCode: "confirmed_revision", IdempotencyKey: "content-lineage-reparent-child",
	})
	if err != nil || reparent.Feedback.ResultFamilyID != root.Decision.FamilyID || reparent.Feedback.ResultRelation != "revision_of" {
		t.Fatalf("reparent feedback = %#v / %v", reparent, err)
	}
	_, err = feedbacks.Review(context.Background(), ingestionapplication.ReviewContentLineageCommand{
		ActorUserID: editorID, LineageDecisionID: root.Decision.DecisionID, ExpectedMemberVersion: 1,
		FeedbackType: "relation_override", RelationOverride: "revision_of",
		TargetParentDocumentVersionID: childDocument.persisted.DocumentVersion.ID, ExpectedTargetMemberVersion: 1,
		ReasonCode: "must_not_cycle", IdempotencyKey: "content-lineage-cycle-attempt",
	})
	if !errors.Is(err, sharedrepository.ErrConflict) {
		t.Fatalf("cycle error = %v, want conflict", err)
	}
	var activeChildMembers, feedbackCount int
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM content_family_members WHERE document_version_id=$1 AND active`,
		childDocument.persisted.DocumentVersion.ID).Scan(&activeChildMembers); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM content_lineage_feedbacks`).Scan(&feedbackCount); err != nil {
		t.Fatal(err)
	}
	if activeChildMembers != 1 || feedbackCount != 2 {
		t.Fatalf("active child members/feedbacks = %d/%d, want 1/2", activeChildMembers, feedbackCount)
	}
}

func insertContentLineageReviewer(t *testing.T, runtime *database.Runtime, suffix string) int64 {
	t.Helper()
	var id int64
	if err := runtime.SQL.QueryRow(`INSERT INTO users (email,password_hash,display_name,role)
VALUES ($1,'not-a-real-password-hash',$2,'editor') RETURNING id`, suffix+"@example.test", suffix).Scan(&id); err != nil {
		t.Fatalf("insert lineage reviewer: %v", err)
	}
	return id
}

func createContentFamilyPlaintext(t *testing.T, runtime *database.Runtime, fixture derivedArtifactDocumentFixture, policyRevision int64) (ingestionapplication.DerivedArtifactDTO, int64, int64) {
	t.Helper()
	storeDecisionID, retainDecisionID := createDerivedArtifactRights(t, runtime, fixture, policyRevision)
	saga := newDerivedArtifactSaga(t, runtime, newKnowledgeProjectionPublisher(t, t.TempDir()), fixture.documentVersions)
	projected, err := saga.Project(context.Background(), ingestionapplication.ProjectDocumentCommand{
		DocumentVersionID: fixture.persisted.DocumentVersion.ID, ExpectedDocumentVersion: fixture.persisted.DocumentVersion.Version,
		ArtifactType: ingestionapplication.DocumentProjectionPlaintext, TransformerProfileSHA256: strings.Repeat("c", 64),
		StoreDerivedRightsDecisionID: storeDecisionID, RetainRightsDecisionID: retainDecisionID,
		ProjectionBytes: []byte("authorized normalized document body"),
	})
	if err != nil {
		t.Fatalf("project family plaintext: %v", err)
	}
	return projected.Artifact, storeDecisionID, retainDecisionID
}
