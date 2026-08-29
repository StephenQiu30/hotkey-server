GO := go
SWAG := $(GO) run github.com/swaggo/swag/cmd/swag
OPENAPI_EXCLUDE := internal/modules/agentaccess,internal/modules/alert,internal/modules/delivery/transport/http,internal/modules/event/transport/http/handler.go

.PHONY: test-env minio-test-env openapi openapi-check vet test ci-test worker-recovery-acceptance m4-fault-recovery-acceptance joint-recovery-acceptance source-resource-limit-acceptance source-admission-matrix-acceptance source-management-audit-acceptance source-live-smoke request-boundary-acceptance monitor-publication-acceptance agent-skill-contract-acceptance agent-degradation-acceptance agent-capacity-baseline event-governance-role-acceptance event-semantic-separation-acceptance notification-convergence-acceptance report-publication-acceptance secret-rotation-acceptance observability-cardinality-acceptance observability-alert-acceptance build architecture repository database-runtime schema capacity-fixture capacity-baseline collection-capacity-fixture collection-capacity-baseline search-capacity-fixture search-capacity-baseline codex-capacity-baseline recovery-evidence vulnerability ci-static ci-runtime ci-vulnerability ci

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
	$(GO) run ./test/runner test -p=1 ./... -count=1

# database-runtime already executes these exact packages against the disposable
# database. PostgreSQL integration tests create a disposable database per test,
# so CI can safely run two packages at a time without sharing mutable facts.
# Keep the public test target exhaustive while avoiding duplicate CI work.
ci-test: test-env
	@packages="$$( $(GO) list ./... | grep -v -e '/internal/platform/database$$' -e '/internal/shared/repository$$' )"; \
		test -n "$$packages"; \
		$(GO) run ./test/runner test -p=2 $$packages -count=1

worker-recovery-acceptance: test-env minio-test-env
	$(GO) run ./test/runner test -tags=integration -p=1 \
		./internal/platform/queue \
		./internal/modules/ingestion/application \
		./internal/modules/source/infrastructure/postgres \
		./internal/modules/ingestion/infrastructure/postgres \
		-run '^(TestWorkerKillAfterClaimReclaimsLeaseAndAppliesSideEffectOnce|TestIngestWorkerKillAfterMinIOWriteRecoversWithoutDuplicateSideEffects|TestEvidenceWorkerKillAfterObservationCommitRecoversOneDocumentJob|TestDocumentProjectionWorkerKillAfterVaultWriteRecoversOneArtifact)$$' \
		-count=1 -v

m4-fault-recovery-acceptance: test-env minio-test-env
	$(GO) run ./test/runner test -tags=integration -p=1 \
		./internal/modules/notification/transport/http \
		./internal/modules/notification/application \
		./internal/modules/notification/infrastructure/postgres \
		./internal/modules/knowledge/application \
		./internal/modules/knowledge/domain \
		./internal/modules/ingestion/application \
		./internal/modules/ingestion/infrastructure/postgres \
		./internal/modules/identity/infrastructure/redis \
		./internal/modules/operations/infrastructure/postgres \
		./internal/platform/queue \
		-run '^(TestWebSocketAuthenticatesInTheFirstFrameReplaysAndRecordsDelivery|TestEmailDeliveryReconcilesProviderReceiptWithoutSendingAgain|TestEmailDeliveryStopsWhenProviderReceiptCannotBeConfirmed|TestEmailDeliveryLeavesStartedFenceWhenProviderAcceptedButAttemptCommitFails|TestEmailDeliveryRepositoryFencesExpiredWorkersAndStopsUnknownSMTPReplay|TestEmailDeliveryRepositoryReusesDispatchKeyWithHigherFenceForIdempotentProvider|TestVaultRecoveryRestoresMissingFileFromRevisionSnapshot|TestVaultRecoveryStopsOnCurrentConflictWithoutReadingOlderCopies|TestStablePathRejectsTraversal|TestSourceDocumentGenerationReturnsHonestPartialResultWhenSearchProjectionFails|TestSourceDocumentGenerationRetriesAfterSearchProjectionRecoveryAndCompletes|TestVerificationStoreReturnsUnavailableDuringRealDisconnectAndRecoversExistingCode|TestProjectionRecoverySchedulesMissingVaultAndWorkerConvergesToZero|TestProjectionRecoveryRepositoryPreservesNotificationFactsAndUnknownAttempts|TestM4ProjectionRecoveryDrillRestoresIndependentCopyToZeroDifference|TestM4ProjectionRecoveryEvidenceWriterIsExclusiveAndPrivate|TestWorkerRecoversExpiredClaimsAndTerminatesExhaustedLease)$$' \
		-count=1 -v

source-resource-limit-acceptance: test-env
	$(GO) run ./test/runner test -p=1 \
		./internal/modules/source/domain \
		./internal/modules/source/application \
		./internal/modules/source/infrastructure/rss \
		./internal/modules/source/infrastructure/hackernews \
		./internal/modules/source/infrastructure/x \
		./internal/modules/source/infrastructure/postgres \
		-run '^(TestExternalRequestBudgetContractsRejectUnboundedOrAmbiguousFacts|TestRSSResourceLimitProfileFreezesEightFiniteDimensions|TestRSSResourceLimitsStopBeforeTheNextExternalOrEvidenceSideEffect|TestRSSResourceLimitsBoundConnectReadAndWallClock|TestRSSResourceLimitProfileRejectsMissingOrUnboundedDimensions|TestHackerNewsResourceLimitProfileFreezesEightFiniteDimensions|TestHackerNewsResourceLimitsStopBeforeNextExternalOrEvidenceSideEffect|TestHackerNewsResourceLimitsBoundConnectReadAndWallClock|TestHackerNewsResourceLimitProfileRejectsMissingOrUnboundedDimensions|TestXResourceLimitProfileFreezesEightFiniteDimensions|TestXResourceLimitsStopBeforeNextExternalOrEvidenceSideEffect|TestXResourceLimitOrderingValidatesInputAndCredentialBeforeBudgetOrNetwork|TestXResourceLimitsBoundConnectReadAndWallClock|TestXResourceLimitProfileRejectsMissingOrUnboundedDimensions|TestExternalRequestBudgetEnforcesUTCSourceProfileQuotaAtomically|TestExternalRequestBudgetEnforcesPerMinuteRateLimitAtomicallyWithoutConsumingDailyBudget|TestCollectionServiceFailureRetainsCursorAndPersistsRetryState|TestCollectionServicePersistsAuthenticationAndPermanentFailures)$$' \
		-count=1 -v

source-admission-matrix-acceptance: test-env
	$(GO) run ./test/runner test -p=1 \
		./internal/modules/source/domain \
		./internal/modules/source/application \
		./internal/modules/source/infrastructure \
		./internal/modules/source/infrastructure/credentialstore \
		./internal/modules/source/infrastructure/rss \
		./internal/modules/source/infrastructure/hackernews \
		./internal/modules/source/infrastructure/x \
		./internal/modules/source/infrastructure/postgres \
		-run '^(TestExternalRequestBudgetContractsRejectUnboundedOrAmbiguousFacts|TestCollectionAdmissionRequiresAnExplicitCurrentFetchAllow|TestCollectionAdmissionRejectsInvalidCapabilityBeforeRightsLookup|TestCollectionAdmissionChecksManagedCredentialStatusAfterRights|TestCollectionAdmissionChecksBudgetAndRateLimitAfterCredentialStatus|TestCollectionFetchRightsDenialStopsBeforeConnectorResolutionAndPersistsSanitizedAudit|TestCollectionHealthAdmissionDenialStopsBeforeConnectorResolutionAndPersistsSafeAudit|TestInstantSearchAdmissionDenialStopsBeforeConnectorResolution|TestXMetricRefreshAdmissionDenialStopsBeforeConnectorResolution|TestManagedCredentialStatusChecksMetadataAndKeyReadinessWithoutResolvingPlaintext|TestConnectorRegistryDefersManagedCredentialDecryptionUntilTheRequestBoundary|TestCollectionRequestBudgetStatusMapsEveryP0ProfileWithoutConsumingARequest|TestExternalRequestBudgetEnforcesUTCSourceProfileQuotaAtomically|TestExternalRequestBudgetEnforcesPerMinuteRateLimitAtomicallyWithoutConsumingDailyBudget|TestSSRFAcceptanceRejectsDirectAndObfuscatedDestinationsBeforeDial|TestSSRFAcceptanceRevalidatesRedirectDNSBeforeFollowingPrivateHop|TestConnectorHonorsContextTimeoutAndOfficialEndpoint|TestConnectorRejectsMissingCredentialBeforeNetwork|TestConnectorRejectsCrossHostRedirect|TestXResourceLimitOrderingValidatesInputAndCredentialBeforeBudgetOrNetwork|TestXEndpointPolicyRejectsUnsafeResolutionBeforeCredentialBudgetOrDial)$$' \
		-count=1 -v

source-management-audit-acceptance: test-env
	$(GO) run ./test/runner test -p=1 \
		./internal/modules/source/application \
		./internal/modules/operations/infrastructure/postgres \
		-run '^(TestSourceServiceCreatesAndRotatesEncryptedManagedCredential|TestSourceServiceAuditsBudgetConfigurationChangeWithoutPersistingConfiguration|TestManualCollectionUsesDurableCooldownAndActivePublishedTargets|TestCollectionControlListsRetriesAndPersistsSafeHealth|TestGovernanceAuditQueryCoversFiveSourceManagementCategoriesWithoutSyntheticSecrets|TestRetentionPreviewRunIsBoundedProtectedAndAudited)$$' \
		-count=1 -v

# This target is intentionally excluded from CI. It requires an explicitly
# authorized trusted environment and exercises the running API against real
# RSS, Hacker News, and X connections.
source-live-smoke:
	$(GO) run ./test/tools/source-live-smoke

request-boundary-acceptance: test-env
	$(GO) run ./test/runner test -p=1 \
		./internal/platform/http \
		-run '^(TestP0RequestBoundaryMatrixIsVersionedFiniteAndComplete|TestFixedRequestBoundaryFixtureMatchesProductionProfilesAndStableCodes|TestRequestBoundaryRateAndAttemptDimensionsStopAtLimitPlusOne|TestRequestBoundaryConcurrencyRejectsBeforeBusinessHandler|TestRequestBoundaryWallClockCancelsAndAuditsWithoutBusinessSideEffect|TestRequestBoundaryAuditIsDeduplicatedAndContainsOnlySafeIdentifiers|TestEveryP0RequestBoundaryClassExercisesAllFourThresholdDimensions)$$' \
		-count=1 -v
	$(GO) run ./test/runner test -p=1 \
		./internal/bootstrap \
		-run '^TestConfiguredAPILoginBoundaryRejectsLimitPlusOneAndPersistsSanitizedAudit$$' \
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

event-semantic-separation-acceptance: test-env
	$(GO) run ./test/runner test -p=1 \
		./internal/modules/event/domain \
		./internal/modules/event/transport/http \
		-run '^(TestRelevanceHeatClaimEvidenceAndEventEvidenceStateRemainIndependent|TestMicroEventResponseKeepsRelevanceHeatAndEvidenceStateSeparate)$$' \
		-count=1 -v
	$(GO) run ./test/runner test -tags=integration -p=1 \
		./internal/modules/report/infrastructure/postgres \
		-run '^TestReportPublicationRevalidatesCitationFactsInsteadOfAggregateEvidenceState$$' \
		-count=1 -v
	cd ../frontend && npm run test:unit -- dashboard-events-page.test.tsx dashboard-reports-page.test.tsx

notification-convergence-acceptance: test-env
	$(GO) run ./test/runner test -tags=integration -p=1 \
		./internal/modules/event/infrastructure/postgres \
		./internal/modules/notification/application \
		./internal/modules/notification/infrastructure/postgres \
		./internal/modules/notification/transport/http \
		-run '^(TestProductEventRefreshRiverReplaysMembersMetricsSnapshotsUpdatesAndNotifications|TestEmailDeliveryRepositoryRechecksCurrentChannelPreferenceBeforeClaim|TestUserNotificationRepositoryReplaysByUserRechecksMonitorAccessAndSeparatesDeliveryAttempts|TestServiceRecordsOnlyCurrentUsersMonotonicReadReceipt|TestWebSocketAuthenticatesInTheFirstFrameReplaysAndRecordsDelivery)$$' \
		-count=1 -v
	cd ../frontend && npm run test:unit -- notificationStore.test.ts RealtimeNotifications.test.tsx dashboard-notifications-page.test.tsx

report-publication-acceptance: test-env
	$(GO) run ./test/runner test -tags=integration -p=1 \
		./internal/modules/report/domain \
		./internal/modules/report/application \
		./internal/modules/report/infrastructure/postgres \
		./internal/modules/report/transport/http \
		-run '^(TestReportPublicationRequiresExactMicroEventSentenceCitations|TestReportRejectsUnsafeContentBeforeDraftPersistenceOrPublication|TestReportRevisionLifecycleAndInputSnapshotAreExplicit|TestServiceApprovalFreezesPendingRevisionAndRejectsRepeat|TestServiceApprovalRejectsInvalidEvidenceBeforeAnyWrite|TestServiceApprovalAuditsUnsafeContentWithoutPayloadOrBusinessWrite|TestServiceRequiresPendingRevisionAndExpectedVersionForApproval|TestServiceRegenerationCreatesNextDraftWithoutOverwritingApprovedRevision|TestMaliciousReportFixtureCannotCreateApprovedOrDownstreamFacts|TestReportRepositoryFreezesExactSentenceCitationsAndPublishedVersion|TestReportRevisionApprovalIsOptimisticAuditedAndRegenerationPreservesApprovedSnapshot|TestReportRevisionConcurrentApprovalHasOneWinner|TestReportRepositoryRejectsUnsafeDraftBeforeAnyWrite|TestReportPublicationRevalidatesCitationFactsInsteadOfAggregateEvidenceState|TestReportApprovalMapsInvalidEvidenceToStableConflict|TestReportApprovalMapsUnsafeContentWithoutReflectingPayload|TestReportResponseExposesExactSentenceCitations)$$' \
		-count=1 -v
	cd ../frontend && npm run test:unit -- content-security-policy.test.ts dashboard-reports-page.test.tsx

secret-rotation-acceptance: test-env
	$(GO) run ./test/runner test -p=1 \
		./internal/platform/config \
		./internal/modules/identity/infrastructure/security \
		./internal/modules/identity/infrastructure/redis \
		./internal/modules/source/infrastructure/credentialstore \
		./internal/bootstrap \
		-run '^(TestEnvironmentExampleDocumentsSecretRotationWindow|TestLoadUsesExplicitSecretRotationConfiguration|TestJWT.*Rotation.*|TestVerificationHMACRotation.*|TestCipherKeyringWritesCurrentVersionReadsPreviousAndRejectsItAfterRevocation|TestRotateBatch.*|TestSourceCredentialRotationCommand.*)$$' \
		-count=1 -v
	cd ../agent && uv run pytest

observability-cardinality-acceptance:
	$(GO) run ./test/runner test -p=1 \
		./internal/platform/logging \
		./internal/platform/observability \
		./internal/platform/http \
		-run '^(TestSafeCore.*|TestMetricsCollapseUntrustedValuesToBoundedLabels|TestAccessLogCarriesRequestAndTraceCorrelation)$$' \
		-count=1 -v

observability-alert-acceptance: test-env
	$(GO) run ./test/runner test -tags=integration -p=1 \
		./internal/modules/operations/application \
		./internal/modules/operations/infrastructure/postgres \
		./internal/modules/notification/infrastructure/postgres \
		-run '^(TestJobRepositoryRuntimeOverview.*|TestOverviewService.*|TestRepository.*UnknownDelivery.*)$$' \
		-count=1 -v

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

search-capacity-fixture: test-env
	sh test/tools/generate-search-capacity-fixture.sh

search-capacity-baseline: test-env
	$(GO) run ./test/tools/search-capacity-baseline

codex-capacity-baseline:
	$(GO) run ./test/tools/codex-capacity-baseline

recovery-evidence:
	$(GO) run ./test/tools/recovery-evidence

joint-recovery-acceptance: test-env minio-test-env
	$(GO) run ./test/tools/joint-recovery-drill

vulnerability:
	$(GO) run golang.org/x/vuln/cmd/govulncheck@v1.6.0 ./...

ci-static: openapi-check vet build architecture repository

ci-runtime: database-runtime schema ci-test

ci-vulnerability: vulnerability

ci: ci-static ci-runtime ci-vulnerability
