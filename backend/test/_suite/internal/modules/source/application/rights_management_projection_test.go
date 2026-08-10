package application

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	sharederrors "github.com/StephenQiu30/hotkey-server/backend/internal/shared/errors"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

type rightsReadAuthorizerFake struct {
	calls int
	last  RightsReadAuthorizationDTO
}

func (authorizer *rightsReadAuthorizerFake) AuthorizeRightsRead(_ context.Context, authorization RightsReadAuthorizationDTO) error {
	authorizer.calls++
	authorizer.last = authorization
	switch authorization.ActorID {
	case 7:
		return nil
	case 1:
		if authorization.Operation == RightsReadSourceEndpointCapability {
			return nil
		}
		return sharederrors.New(sharederrors.CodeForbidden, 403, "")
	default:
		return sharederrors.New(sharederrors.CodeForbidden, 403, "")
	}
}

type rightsManagementProjectionRepositoryFake struct {
	endpoint       SourceEndpointCapabilityFactsDTO
	policies       ListRightsPoliciesRepositoryResultDTO
	batches        ListRightsDecisionBatchesRepositoryResultDTO
	decision       RightsDecisionReadDTO
	actionMatrix   RightsActionMatrixDTO
	err            error
	lastEndpointID int64
	lastPolicies   ListRightsPoliciesRepositoryDTO
	lastBatches    ListRightsDecisionBatchesRepositoryDTO
	lastDecision   FindRightsDecisionReadRepositoryDTO
	lastEvaluation EvaluateRightsActionMatrixRepositoryDTO
}

func (repository *rightsManagementProjectionRepositoryFake) FindSourceEndpointCapabilityFacts(_ context.Context, sourceEndpointID int64) (SourceEndpointCapabilityFactsDTO, error) {
	repository.lastEndpointID = sourceEndpointID
	return repository.endpoint, repository.err
}

func (repository *rightsManagementProjectionRepositoryFake) ListRightsPolicies(_ context.Context, query ListRightsPoliciesRepositoryDTO) (ListRightsPoliciesRepositoryResultDTO, error) {
	repository.lastPolicies = query
	return repository.policies, repository.err
}

func (repository *rightsManagementProjectionRepositoryFake) ListRightsDecisionBatches(_ context.Context, query ListRightsDecisionBatchesRepositoryDTO) (ListRightsDecisionBatchesRepositoryResultDTO, error) {
	repository.lastBatches = query
	return repository.batches, repository.err
}

func (repository *rightsManagementProjectionRepositoryFake) FindRightsDecisionRead(_ context.Context, query FindRightsDecisionReadRepositoryDTO) (RightsDecisionReadDTO, error) {
	repository.lastDecision = query
	return repository.decision, repository.err
}

func (repository *rightsManagementProjectionRepositoryFake) EvaluateRightsActionMatrix(_ context.Context, query EvaluateRightsActionMatrixRepositoryDTO) (RightsActionMatrixDTO, error) {
	repository.lastEvaluation = query
	return repository.actionMatrix, repository.err
}

func TestRightsManagementProjectionReportsOnlyNonAuthorizingPublicCapability(t *testing.T) {
	t.Parallel()
	repository := &rightsManagementProjectionRepositoryFake{endpoint: SourceEndpointCapabilityFactsDTO{
		SourceEndpointID: 42, SourceType: "rss", Enabled: true, HealthStatus: "healthy",
	}}
	service := newRightsManagementProjectionServiceForTest(t, repository)

	result, err := service.GetSourceEndpointCapability(context.Background(), GetSourceEndpointCapabilityQuery{ActorID: 1, SourceEndpointID: 42})
	if err != nil {
		t.Fatalf("GetSourceEndpointCapability(): %v", err)
	}
	if repository.lastEndpointID != 42 || result.SourceEndpointID != 42 || result.SourceType != "rss" ||
		result.CollectionInterface != "rss_atom_feed" || result.ContentScope != "feed_payload" ||
		result.DocumentCaptureMode != "policy_gated_body" || result.DefaultAccessMode != "metadata_only" ||
		!reflect.DeepEqual(result.RequiredActions, []string{"fetch", "store_raw", "store_derived", "display_private", "quote", "embed_local", "retain"}) ||
		result.Availability != SourceEndpointCapabilityAvailable || result.RightsStatus != SourceEndpointRightsPolicyRequired ||
		result.FollowsCanonicalURL {
		t.Fatalf("public capability = %#v", result)
	}

	projectionType := reflect.TypeOf(SourceEndpointCapabilityDTO{})
	for _, forbidden := range []string{"SubjectKey", "InputDigest", "Allowed", "AllowBodyStorage", "ObjectKey"} {
		if _, found := projectionType.FieldByName(forbidden); found {
			t.Fatalf("public capability exposes authorizing or sensitive field %s", forbidden)
		}
	}
}

func TestRightsManagementProjectionFreezesEveryConnectorRightsCapability(t *testing.T) {
	t.Parallel()
	tests := []struct {
		sourceType, collectionInterface, contentScope, captureMode string
	}{
		{"rss", "rss_atom_feed", "feed_payload", "policy_gated_body"},
		{"hacker_news", "official_api", "platform_post", "policy_gated_body"},
		{"x", "authorized_official_api", "platform_post", "policy_gated_body"},
		{"bilibili", "authorized_official_api", "platform_post", "policy_gated_body"},
		{"weibo", "authorized_official_api", "platform_post", "policy_gated_body"},
		{"google_agent_search", "authorized_official_api", "discovery_snippet", "policy_gated_snippet"},
		{"bing_grounding", "authorized_official_api", "discovery_synthesis", "metadata_only"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.sourceType, func(t *testing.T) {
			t.Parallel()
			result, err := sourceEndpointCapability(SourceEndpointCapabilityFactsDTO{
				SourceEndpointID: 42, SourceType: test.sourceType, Enabled: true, HealthStatus: "healthy",
			})
			if err != nil {
				t.Fatalf("sourceEndpointCapability(): %v", err)
			}
			if result.CollectionInterface != test.collectionInterface || result.ContentScope != test.contentScope ||
				result.DocumentCaptureMode != test.captureMode || result.DefaultAccessMode != "metadata_only" ||
				result.RightsStatus != SourceEndpointRightsPolicyRequired {
				t.Fatalf("capability = %#v", result)
			}
			if test.captureMode == "metadata_only" && containsRightsAction(result.RequiredActions, "display_private") {
				t.Fatalf("metadata-only connector requires body display rights: %#v", result.RequiredActions)
			}
		})
	}
}

func containsRightsAction(actions []string, expected string) bool {
	for _, action := range actions {
		if action == expected {
			return true
		}
	}
	return false
}

func TestRightsManagementProjectionEvaluatesNineActionsOnlyForExactSubjectDigestAndTime(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, time.August, 9, 13, 0, 0, 123456789, time.UTC)
	repository := &rightsManagementProjectionRepositoryFake{actionMatrix: RightsActionMatrixDTO{
		SourceEndpointID: 42,
		EvaluatedAt:      at.Truncate(time.Microsecond),
		Actions: []RightsActionCapabilityDTO{
			{Action: "fetch", Decision: "allow", DecisionIDs: []int64{10}, PolicyIDs: []int64{20}, Priority: intPointer(300)},
			{Action: "store_raw", Decision: "unknown", DecisionIDs: []int64{}, PolicyIDs: []int64{}},
			{Action: "store_derived", Decision: "unknown", DecisionIDs: []int64{}, PolicyIDs: []int64{}},
			{Action: "display_private", Decision: "deny", DecisionIDs: []int64{11}, PolicyIDs: []int64{20}, Priority: intPointer(300)},
			{Action: "redistribute", Decision: "unknown", DecisionIDs: []int64{}, PolicyIDs: []int64{}},
			{Action: "quote", Decision: "unknown", DecisionIDs: []int64{}, PolicyIDs: []int64{}},
			{Action: "embed_local", Decision: "unknown", DecisionIDs: []int64{}, PolicyIDs: []int64{}},
			{Action: "send_external_model", Decision: "unknown", DecisionIDs: []int64{}, PolicyIDs: []int64{}},
			{Action: "retain", Decision: "unknown", DecisionIDs: []int64{}, PolicyIDs: []int64{}},
		},
	}}
	service := newRightsManagementProjectionServiceForTest(t, repository)
	query := EvaluateRightsActionMatrixQuery{
		ActorID: 7, SourceEndpointID: 42, SubjectType: "raw_response",
		SubjectKey: strings.Repeat("b", 64), InputDigest: strings.Repeat("c", 64), At: at,
	}

	result, err := service.EvaluateRightsActionMatrix(context.Background(), query)
	if err != nil {
		t.Fatalf("EvaluateRightsActionMatrix(): %v", err)
	}
	if repository.lastEvaluation.SourceEndpointID != query.SourceEndpointID ||
		repository.lastEvaluation.SubjectType != query.SubjectType || repository.lastEvaluation.SubjectKey != query.SubjectKey ||
		repository.lastEvaluation.InputDigest != query.InputDigest ||
		!repository.lastEvaluation.At.Equal(at.Truncate(time.Microsecond)) || len(result.Actions) != 9 {
		t.Fatalf("exact evaluation request/result = %#v / %#v", repository.lastEvaluation, result)
	}
	if result.Actions[0].Decision != "allow" || result.Actions[1].Decision != "unknown" || result.Actions[3].Decision != "deny" {
		t.Fatalf("action matrix changed allow/unknown/deny semantics: %#v", result.Actions)
	}

	for _, invalid := range []EvaluateRightsActionMatrixQuery{
		{ActorID: 7, SourceEndpointID: 42, SubjectType: "raw_response", SubjectKey: query.SubjectKey, InputDigest: query.InputDigest},
		{ActorID: 7, SourceEndpointID: 42, SubjectType: "raw_response", SubjectKey: query.SubjectKey, At: at},
		{ActorID: 7, SourceEndpointID: 42, SubjectType: "unsupported", SubjectKey: query.SubjectKey, InputDigest: query.InputDigest, At: at},
	} {
		if _, err := service.EvaluateRightsActionMatrix(context.Background(), invalid); !errors.Is(err, sharedrepository.ErrInvalidInput) {
			t.Fatalf("invalid exact evaluation error = %v", err)
		}
	}
}

func TestRightsManagementProjectionListsSafeImmutablePolicyAndDecisionHistory(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.August, 9, 12, 0, 0, 0, time.UTC)
	policy := RightsPolicyReadDTO{
		ID: 71, Version: 1, SourceEndpointID: int64Pointer(42), ScopeType: "source_endpoint", ScopeSubject: "feed-main",
		Revision: 7, Priority: 300, BasisSummary: "approved feed terms", TermsURL: "https://publisher.example.test/terms",
		LicenseURI: "urn:license:feed-main", PolicyHash: strings.Repeat("a", 64), RecordedByUserID: 7,
		ApprovedByUserID: int64Pointer(7), EffectiveFrom: now, CreatedAt: now,
	}
	decision := RightsDecisionReadDTO{
		ID: 501, DecisionBatchID: 90, SourceEndpointID: 42, PolicyID: 71, PolicyRevision: 7,
		PolicyScopeType: "source_endpoint", PolicyScopeSubject: "feed-main", Priority: 300,
		BasisSummary: "approved feed terms", TermsURL: "https://publisher.example.test/terms", LicenseURI: "urn:license:feed-main",
		SubjectType: "raw_response", SubjectKey: strings.Repeat("b", 64), InputDigest: strings.Repeat("c", 64),
		Action: "store_raw", Decision: "allow", ReasonCodes: []string{"terms_confirmed"}, Evaluator: "rights-admin",
		EvaluatedAt: now, EffectiveFrom: now, RecordedByUserID: 7, CreatedAt: now,
	}
	batch := RightsDecisionBatchDTO{
		ID: 90, Version: 1, SourceEndpointID: 42, PolicyID: 71, ExpectedPolicyVersion: 1,
		SubjectType: decision.SubjectType, SubjectKey: decision.SubjectKey, InputDigest: decision.InputDigest,
		RecordedByUserID: 7, DecisionCount: 1, CreatedAt: now, Decisions: []RightsDecisionReadDTO{decision},
	}
	repository := &rightsManagementProjectionRepositoryFake{
		policies: ListRightsPoliciesRepositoryResultDTO{Items: []RightsPolicyReadDTO{policy}, NextCursor: "policy-next"},
		batches:  ListRightsDecisionBatchesRepositoryResultDTO{Items: []RightsDecisionBatchDTO{batch}, NextCursor: "batch-next"},
		decision: decision,
	}
	service := newRightsManagementProjectionServiceForTest(t, repository)

	policies, err := service.ListRightsPolicies(context.Background(), ListRightsPoliciesQuery{ActorID: 7, SourceEndpointID: 42, Cursor: "policy-cursor"})
	if err != nil || repository.lastPolicies.Limit != defaultRightsManagementPageLimit || policies.NextCursor != "policy-next" || len(policies.Items) != 1 {
		t.Fatalf("ListRightsPolicies() = %#v, %v; repository query=%#v", policies, err, repository.lastPolicies)
	}
	batches, err := service.ListRightsDecisionBatches(context.Background(), ListRightsDecisionBatchesQuery{ActorID: 7, SourceEndpointID: 42, Cursor: "batch-cursor", Limit: 25})
	if err != nil || repository.lastBatches.Limit != 25 || batches.NextCursor != "batch-next" || len(batches.Items) != 1 || len(batches.Items[0].Decisions) != 1 {
		t.Fatalf("ListRightsDecisionBatches() = %#v, %v; repository query=%#v", batches, err, repository.lastBatches)
	}
	loaded, err := service.GetRightsDecision(context.Background(), GetRightsDecisionQuery{ActorID: 7, SourceEndpointID: 42, DecisionID: 501})
	if err != nil || repository.lastDecision.DecisionID != 501 || repository.lastDecision.SourceEndpointID != 42 || loaded.ID != 501 {
		t.Fatalf("GetRightsDecision() = %#v, %v", loaded, err)
	}

	policy.PolicyHash = "caller-mutated"
	decision.ReasonCodes[0] = "caller-mutated"
	if policies.Items[0].PolicyHash == "caller-mutated" || batches.Items[0].Decisions[0].ReasonCodes[0] == "caller-mutated" {
		t.Fatal("read result retained mutable repository-owned data")
	}
}

func TestRightsManagementProjectionAuthorizesEveryReadInsideApplication(t *testing.T) {
	t.Parallel()
	repository := &rightsManagementProjectionRepositoryFake{endpoint: SourceEndpointCapabilityFactsDTO{
		SourceEndpointID: 42, SourceType: "rss", Enabled: true, HealthStatus: "healthy",
	}}
	authorizer := &rightsReadAuthorizerFake{}
	service := newRightsManagementProjectionServiceWithAuthorizerForTest(t, repository, authorizer)

	if _, err := service.GetSourceEndpointCapability(context.Background(), GetSourceEndpointCapabilityQuery{ActorID: 1, SourceEndpointID: 42}); err != nil {
		t.Fatalf("active viewer public capability: %v", err)
	}
	if _, err := service.ListRightsPolicies(context.Background(), ListRightsPoliciesQuery{ActorID: 1, SourceEndpointID: 42}); !isRightsForbidden(err) {
		t.Fatalf("viewer policy history error = %v", err)
	}
	if _, err := service.EvaluateRightsActionMatrix(context.Background(), EvaluateRightsActionMatrixQuery{
		ActorID: 1, SourceEndpointID: 42, SubjectType: "raw_response",
		SubjectKey: strings.Repeat("b", 64), InputDigest: strings.Repeat("c", 64), At: time.Now().UTC(),
	}); !isRightsForbidden(err) {
		t.Fatalf("viewer exact evaluation error = %v", err)
	}
	if _, err := service.GetSourceEndpointCapability(context.Background(), GetSourceEndpointCapabilityQuery{ActorID: 8, SourceEndpointID: 42}); !isRightsForbidden(err) {
		t.Fatalf("disabled/deleted user public capability error = %v", err)
	}
	if repository.lastPolicies.SourceEndpointID != 0 || !repository.lastEvaluation.At.IsZero() {
		t.Fatal("unauthorized Application read reached projection repository")
	}
}

func TestRightsManagementProjectionRejectsRepositoryCrossEndpointDrift(t *testing.T) {
	t.Parallel()
	otherEndpointID := int64(99)
	repository := &rightsManagementProjectionRepositoryFake{
		endpoint: SourceEndpointCapabilityFactsDTO{SourceEndpointID: otherEndpointID, SourceType: "rss", Enabled: true, HealthStatus: "healthy"},
		policies: ListRightsPoliciesRepositoryResultDTO{Items: []RightsPolicyReadDTO{{
			ID: 71, Version: 1, SourceEndpointID: &otherEndpointID, ScopeType: "source_endpoint", ScopeSubject: "other-feed",
			Revision: 1, Priority: 300, BasisSummary: "approved", PolicyHash: strings.Repeat("a", 64),
			EffectiveFrom: time.Now().UTC().Add(-time.Hour), RecordedByUserID: 7, CreatedAt: time.Now().UTC(),
		}}},
	}
	service := newRightsManagementProjectionServiceForTest(t, repository)
	if _, err := service.GetSourceEndpointCapability(context.Background(), GetSourceEndpointCapabilityQuery{ActorID: 7, SourceEndpointID: 42}); !errors.Is(err, sharedrepository.ErrConstraint) {
		t.Fatalf("cross-endpoint capability error = %v", err)
	}
	if _, err := service.ListRightsPolicies(context.Background(), ListRightsPoliciesQuery{ActorID: 7, SourceEndpointID: 42}); !errors.Is(err, sharedrepository.ErrConstraint) {
		t.Fatalf("cross-endpoint policy error = %v", err)
	}
}

func newRightsManagementProjectionServiceForTest(t *testing.T, projection RightsManagementProjectionRepository) *RightsManagementService {
	return newRightsManagementProjectionServiceWithAuthorizerForTest(t, projection, &rightsReadAuthorizerFake{})
}

func newRightsManagementProjectionServiceWithAuthorizerForTest(t *testing.T, projection RightsManagementProjectionRepository, readAuthorizer RightsReadAuthorizer) *RightsManagementService {
	t.Helper()
	service, err := NewRightsManagementService(RightsManagementDependencies{
		Repository:     newRightsManagementRepositoryFake(),
		Projection:     projection,
		ReadAuthorizer: readAuthorizer,
		Authorizer:     &rightsActorAuthorizerFake{},
		Audit:          &rightsManagementAuditFake{},
		Transactions:   &rightsManagementTransactionFake{},
	})
	if err != nil {
		t.Fatalf("NewRightsManagementService(): %v", err)
	}
	return service
}

func isRightsForbidden(err error) bool {
	var applicationError *sharederrors.AppError
	return errors.As(err, &applicationError) && applicationError.Code == sharederrors.CodeForbidden
}

func intPointer(value int) *int       { return &value }
func int64Pointer(value int64) *int64 { return &value }
