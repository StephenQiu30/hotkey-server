package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	ingestionpostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/infrastructure/postgres"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

func TestDocumentProjectionAuthorizationReaderSelectsExactCurrentDecisions(t *testing.T) {
	runtime := openDocumentVersionRuntime(t)
	defer func() { _ = runtime.Close() }()
	fixture := createDerivedArtifactDocument(t, runtime, "authorization-reader", 71)
	storeDecisionID, retainDecisionID := createDerivedArtifactRights(t, runtime, fixture, 1)
	displayDecisionID := createDocumentDisplayDecision(
		t,
		runtime,
		fixture.sourceID,
		fixture.persisted.DocumentVersion.ID,
		fixture.persisted.DocumentVersion.ContentSHA256,
		2,
		nil,
		fixture.persisted.DocumentVersion.ID,
	)
	decisionAt := time.Now().UTC().Truncate(time.Microsecond)
	query := ingestionapplication.DocumentProjectionAuthorizationQuery{
		SourceConnectionID: fixture.sourceID,
		DocumentVersionID:  fixture.persisted.DocumentVersion.ID,
		ContentSHA256:      fixture.persisted.DocumentVersion.ContentSHA256,
		DecisionAt:         decisionAt,
	}

	reader := ingestionpostgres.NewDocumentProjectionAuthorizationReader(runtime)
	result, err := reader.ReadDocumentProjectionAuthorization(context.Background(), query)
	if err != nil {
		t.Fatalf("ReadDocumentProjectionAuthorization() error = %v", err)
	}
	if result.SourceConnectionID != query.SourceConnectionID || result.DocumentVersionID != query.DocumentVersionID ||
		result.ContentSHA256 != query.ContentSHA256 || !result.DecisionAt.Equal(decisionAt) ||
		result.StoreDerivedRightsDecisionID != storeDecisionID || result.RetainRightsDecisionID != retainDecisionID ||
		result.DisplayPrivateRightsDecisionID == nil || *result.DisplayPrivateRightsDecisionID != displayDecisionID {
		t.Fatalf("authorization projection = %#v", result)
	}

	wrongDigest := query
	wrongDigest.ContentSHA256 = "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	if _, err := reader.ReadDocumentProjectionAuthorization(context.Background(), wrongDigest); !errors.Is(err, sharedrepository.ErrNotFound) {
		t.Fatalf("wrong digest error = %v, want not found", err)
	}
	wrongSource := query
	wrongSource.SourceConnectionID++
	if _, err := reader.ReadDocumentProjectionAuthorization(context.Background(), wrongSource); !errors.Is(err, sharedrepository.ErrNotFound) {
		t.Fatalf("wrong source error = %v, want not found", err)
	}
}

func TestDocumentProjectionAuthorizationReaderFailsClosedAfterHigherPriorityDeny(t *testing.T) {
	runtime := openDocumentVersionRuntime(t)
	defer func() { _ = runtime.Close() }()
	fixture := createDerivedArtifactDocument(t, runtime, "authorization-revoked", 72)
	createDerivedArtifactRights(t, runtime, fixture, 1)
	decisionAt := time.Now().UTC().Truncate(time.Microsecond)
	query := ingestionapplication.DocumentProjectionAuthorizationQuery{
		SourceConnectionID: fixture.sourceID,
		DocumentVersionID:  fixture.persisted.DocumentVersion.ID,
		ContentSHA256:      fixture.persisted.DocumentVersion.ContentSHA256,
		DecisionAt:         decisionAt,
	}
	reader := ingestionpostgres.NewDocumentProjectionAuthorizationReader(runtime)

	allowed, err := reader.ReadDocumentProjectionAuthorization(context.Background(), query)
	if err != nil || allowed.DisplayPrivateRightsDecisionID != nil {
		t.Fatalf("archive-only authorization = %#v/%v", allowed, err)
	}

	denyPolicy := createDocumentObservationRightsPolicy(
		t,
		runtime,
		fixture.sourceID,
		fixture.observationID,
		2,
		decisionAt.Add(-time.Hour),
	)
	insertDocumentRightsDecisionWithOutcome(
		t,
		runtime,
		denyPolicy,
		fixture.persisted.DocumentVersion.ID,
		fixture.persisted.DocumentVersion.ContentSHA256,
		"store_derived",
		"deny",
		nil,
		nil,
		fixture.persisted.DocumentVersion.ID,
	)
	if _, err := reader.ReadDocumentProjectionAuthorization(context.Background(), query); !errors.Is(err, sharedrepository.ErrConflict) {
		t.Fatalf("revoked store-derived error = %v, want conflict", err)
	}
}

func TestDocumentProjectionAuthorizationReaderTreatsSamePriorityConflictAsDenied(t *testing.T) {
	runtime := openDocumentVersionRuntime(t)
	defer func() { _ = runtime.Close() }()
	fixture := createDerivedArtifactDocument(t, runtime, "authorization-conflict", 73)
	createDerivedArtifactRights(t, runtime, fixture, 1)
	decisionAt := time.Now().UTC().Truncate(time.Microsecond)

	conflictingPolicy := createDocumentRightsPolicyForScope(
		t,
		runtime,
		fixture.sourceID,
		2,
		300,
		"feed_or_account",
		"feed-authorization-conflict",
		decisionAt.Add(-time.Hour),
	)
	retentionDays := 30
	insertDocumentRightsDecisionWithOutcome(
		t,
		runtime,
		conflictingPolicy,
		fixture.persisted.DocumentVersion.ID,
		fixture.persisted.DocumentVersion.ContentSHA256,
		"retain",
		"unknown",
		nil,
		&retentionDays,
		fixture.persisted.DocumentVersion.ID,
	)

	reader := ingestionpostgres.NewDocumentProjectionAuthorizationReader(runtime)
	_, err := reader.ReadDocumentProjectionAuthorization(context.Background(), ingestionapplication.DocumentProjectionAuthorizationQuery{
		SourceConnectionID: fixture.sourceID,
		DocumentVersionID:  fixture.persisted.DocumentVersion.ID,
		ContentSHA256:      fixture.persisted.DocumentVersion.ContentSHA256,
		DecisionAt:         decisionAt,
	})
	if !errors.Is(err, sharedrepository.ErrConflict) {
		t.Fatalf("same-priority retain conflict error = %v, want conflict", err)
	}
}
