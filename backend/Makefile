GO := go
SWAG := $(GO) run github.com/swaggo/swag/cmd/swag
OPENAPI_EXCLUDE := internal/modules/agentaccess,internal/modules/alert,internal/modules/delivery/transport/http,internal/modules/event/transport/http/handler.go,internal/modules/knowledge/transport/http,internal/modules/report

.PHONY: test-env minio-test-env openapi openapi-check vet test worker-recovery-acceptance source-resource-limit-acceptance monitor-publication-acceptance agent-skill-contract-acceptance agent-degradation-acceptance agent-capacity-baseline event-governance-role-acceptance build architecture repository database-runtime schema capacity-fixture capacity-baseline collection-capacity-fixture collection-capacity-baseline codex-capacity-baseline recovery-evidence vulnerability ci-static ci-runtime ci-vulnerability ci

test-env:
	@test -n "$(HOTKEY_TEST_DSN)" || { printf '%s\n' 'HOTKEY_TEST_DSN is required for backend acceptance' >&2; exit 1; }
	@test -n "$(HOTKEY_TEST_REDIS_URL)" || { printf '%s\n' 'HOTKEY_TEST_REDIS_URL is required for backend acceptance' >&2; exit 1; }

minio-test-env:
	@test -n "$(HOTKEY_TEST_MINIO_ENDPOINT)" || { printf '%s\n' 'HOTKEY_TEST_MINIO_ENDPOINT is required for Worker recovery acceptance' >&2; exit 1; }
	@test -n "$(HOTKEY_TEST_MINIO_ACCESS_KEY)" || { printf '%s\n' 'HOTKEY_TEST_MINIO_ACCESS_KEY is required for Worker recovery acceptance' >&2; exit 1; }
	@test -n "$(HOTKEY_TEST_MINIO_SECRET_KEY)" || { printf '%s\n' 'HOTKEY_TEST_MINIO_SECRET_KEY is required for Worker recovery acceptance' >&2; exit 1; }
	@test -n "$(HOTKEY_TEST_MINIO_BUCKET)" || { printf '%s\n' 'HOTKEY_TEST_MINIO_BUCKET is required for Worker recovery acceptance' >&2; exit 1; }

openapi:
	$(SWAG) init --generalInfo cmd/hotkey/main.go --parseInternal --tags '!events,!radar' --exclude "$(OPENAPI_EXCLUDE)" --output openapi --outputTypes go --packageName openapi
	$(SWAG) init --generalInfo cmd/hotkey/main.go --parseInternal --tags '!events,!radar' --exclude "$(OPENAPI_EXCLUDE)" --output ../docs/openapi --outputTypes json --packageName openapi

openapi-check: openapi
	$(GO) run ./test/runner test ./test/architecture -run 'Test(OpenAPIContract|GeneratedOpenAPIRegistryMatchesCommittedArtifact)$$' -count=1
	git -C .. diff --exit-code -- backend/openapi/docs.go docs/openapi/swagger.json

vet:
	$(GO) run ./test/runner vet ./...

test: test-env
	$(GO) run ./test/runner test ./... -count=1

worker-recovery-acceptance: test-env minio-test-env
	$(GO) run ./test/runner test -tags=integration -p=1 \
		./internal/platform/queue \
		./internal/modules/ingestion/application \
		./internal/modules/source/infrastructure/postgres \
		./internal/modules/ingestion/infrastructure/postgres \
		-run '^(TestWorkerKillAfterClaimReclaimsLeaseAndAppliesSideEffectOnce|TestIngestWorkerKillAfterMinIOWriteRecoversWithoutDuplicateSideEffects|TestEvidenceWorkerKillAfterObservationCommitRecoversOneDocumentJob|TestDocumentProjectionWorkerKillAfterVaultWriteRecoversOneArtifact)$$' \
		-count=1 -v

source-resource-limit-acceptance: test-env
	$(GO) run ./test/runner test -p=1 \
		./internal/modules/source/domain \
		./internal/modules/source/application \
		./internal/modules/source/infrastructure/rss \
		./internal/modules/source/infrastructure/hackernews \
		./internal/modules/source/infrastructure/x \
		./internal/modules/source/infrastructure/postgres \
		-run '^(TestExternalRequestBudgetContractsRejectUnboundedOrAmbiguousFacts|TestRSSResourceLimitProfileFreezesEightFiniteDimensions|TestRSSResourceLimitsStopBeforeTheNextExternalOrEvidenceSideEffect|TestRSSResourceLimitsBoundConnectReadAndWallClock|TestRSSResourceLimitProfileRejectsMissingOrUnboundedDimensions|TestHackerNewsResourceLimitProfileFreezesEightFiniteDimensions|TestHackerNewsResourceLimitsStopBeforeNextExternalOrEvidenceSideEffect|TestHackerNewsResourceLimitsBoundConnectReadAndWallClock|TestHackerNewsResourceLimitProfileRejectsMissingOrUnboundedDimensions|TestXResourceLimitProfileFreezesEightFiniteDimensions|TestXResourceLimitsStopBeforeNextExternalOrEvidenceSideEffect|TestXResourceLimitOrderingValidatesInputAndCredentialBeforeBudgetOrNetwork|TestXResourceLimitsBoundConnectReadAndWallClock|TestXResourceLimitProfileRejectsMissingOrUnboundedDimensions|TestExternalRequestBudgetEnforcesUTCSourceProfileQuotaAtomically|TestCollectionServiceFailureRetainsCursorAndPersistsRetryState|TestCollectionServicePersistsAuthenticationAndPermanentFailures)$$' \
		-count=1 -v

monitor-publication-acceptance: test-env
	$(GO) run ./test/runner test -p=1 \
		./internal/modules/monitor/domain \
		./internal/modules/monitor/application \
		./internal/modules/monitor/infrastructure/postgres \
		-run '^(TestMonitorStateTransitionsFollowPublishedStateMachine|TestPublishedIntentVersionKeepsImmutableApprovedSnapshot|TestMonitorServicePublishesImmutableConfigurationAndCoordinatesSourceLifecycle|TestMonitorServiceFirstDraftAndPublishConcurrencyAndAuditRollback|TestIntentCompilerPersistsExactPreviewProfileWithApprovedFactsOnly|TestCompiledIntentProfileHashIsStableAcrossEquivalentPreviewRuns|TestIntentPublicationStagesExactPreviewFactsAndCompletesPublishedProfile|TestIntentRepositoryPersistsExactPreviewCompiledProfileIdempotently|TestIntentPublicationRepositoryPromotesSuccessfulExactPreviewInsidePublishTransaction|TestCollectionSchedulerSkipsPublishedMonitorWithoutReadyCompiledProfile|TestPublishedCollectionTargetReaderPrefersExactCompiledIntentOverLegacyRules|TestCollectionSchedulerEnqueuesDueSourceWithoutWritingCollectionFacts)$$' \
		-count=1 -v

agent-skill-contract-acceptance: test-env
	cd ../agent && uv run pytest
	$(GO) run ./test/runner test -p=1 \
		./internal/modules/intelligence/domain \
		./internal/modules/intelligence/application \
		./internal/modules/intelligence/infrastructure/agent \
		./internal/modules/event/application \
		-run '^(TestProviderRequestsKeepOnlyProviderNeutralContract|TestClientSendsVersionedAuthenticatedBoundedAnalysisRequest|TestClientRejectsInvalidConfigurationAndRequestBeforeNetwork|TestClientMapsStatusTimeoutAndMalformedResponsesToStableErrors|TestClientRejectsIdentityAndEvidenceForgery|TestClientAdaptsStructuredRequestsToVersionedAgentPayload|TestClientDoesNotPretendToProvideEmbeddings|TestStructuredOutputPolicy.*|TestSchemaRegistry.*|TestP0Skill.*|TestRelevanceReviewSchemaRegistryContract|TestEventIntelligenceSchemaRegistryContracts|TestAtomicClaimEvidenceV2SchemaRequiresExactQuotedEvidence|TestAutomaticClaimEvidenceServiceLeavesOperationalModelFailuresPendingWithoutBusinessWrites|TestPromptInjectionFixtureCannotChangeEvidenceOrOutputContract|TestRunServiceNeverPersistsOrReusesForgedEvidenceAndRepairConsumesNewAttempt)$$' \
		-count=1 -v

agent-degradation-acceptance: test-env
	$(GO) run ./test/runner test -p=1 \
		./internal/bootstrap \
		./internal/modules/source/application \
		./internal/modules/ingestion/application \
		./internal/modules/intelligence/application \
		./internal/modules/intelligence/infrastructure/agent \
		./internal/modules/intelligence/infrastructure/jobs \
		./internal/modules/event/application \
		-run '^(TestAgentShadowRunnerIsOptionalAndConstructsOnlyFromValidatedInternalConfig|TestRunServiceKeepsPrimaryResultWhenPythonAgentIsUnavailable|TestShadowRunnerMapsAgentFailureAndBoundsConcurrency|TestRunServicePersistsTerminalOperationalFailureAsPendingAnalysis|TestCollectionServiceProjectsThreeSourcePartialSuccessWithoutPersistingAggregateOutcome|TestContentFamilyServicePersistsOnlyFingerprintAndDecisionFacts|TestEventHeatServiceUsesActiveProfileWeightsAndStableSnapshot|TestMicroEventGovernanceServiceBuildsStablePOJOCommand|TestAutomaticClaimEvidenceServiceLeavesOperationalModelFailuresPendingWithoutBusinessWrites|TestAIRunRecomputeServiceSchedulesOnlyOwnedTerminalFailures|TestAIRunRecomputeWorkerReactivatesOwningJobFromRunIDOnly)$$' \
		-count=1 -v

agent-capacity-baseline:
	cd ../agent && uv run python tests/tools/capacity_baseline.py

event-governance-role-acceptance:
	$(GO) run ./test/runner test ./internal/modules/event/transport/http \
		-run '^(TestMicroEventRoutesRequireAuthenticationAndProtectMutations|TestMicroEventRoutesEnforceFourRoleReadAndGovernanceBoundary)$$' \
		-count=1 -v
	cd ../frontend && npm run test:unit -- dashboard-event-governance-page.test.tsx

build:
	$(GO) build ./...

architecture:
	sh test/tools/validate-architecture.sh

repository:
	sh test/tools/validate-repository.sh

database-runtime: test-env
	sh test/tools/verify-database-runtime.sh

schema: test-env
	sh test/tools/verify-schema.sh

capacity-fixture: test-env
	sh test/tools/generate-capacity-fixture.sh

capacity-baseline: test-env
	$(GO) run ./test/tools/capacity-baseline

collection-capacity-fixture: test-env
	sh test/tools/generate-collection-capacity-fixture.sh

collection-capacity-baseline: test-env
	$(GO) run ./test/tools/collection-capacity-baseline

codex-capacity-baseline:
	$(GO) run ./test/tools/codex-capacity-baseline

recovery-evidence:
	$(GO) run ./test/tools/recovery-evidence

vulnerability:
	$(GO) run golang.org/x/vuln/cmd/govulncheck@v1.6.0 ./...

ci-static: openapi-check vet build architecture repository

ci-runtime: database-runtime schema test

ci-vulnerability: vulnerability

ci: ci-static ci-runtime ci-vulnerability
