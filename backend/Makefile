GO := go
SWAG := $(GO) run github.com/swaggo/swag/cmd/swag
OPENAPI_EXCLUDE := internal/modules/agentaccess,internal/modules/alert,internal/modules/delivery/transport/http,internal/modules/event/transport/http/handler.go,internal/modules/knowledge/transport/http,internal/modules/report

.PHONY: test-env minio-test-env openapi openapi-check vet test worker-recovery-acceptance source-resource-limit-acceptance build architecture repository database-runtime schema capacity-fixture capacity-baseline collection-capacity-fixture collection-capacity-baseline codex-capacity-baseline recovery-evidence vulnerability ci

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

ci: openapi-check vet build architecture repository database-runtime schema test vulnerability
