#!/usr/bin/env sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
errors=0

report() {
  printf '%s\n' "$1" >&2
  errors=1
}

for path in internal/bootstrap internal/platform internal/shared internal/modules; do
  test -d "$root/$path" || report "missing required directory: $path"
done
test -f "$root/db/schema.sql" || report "missing complete schema: db/schema.sql"

for path in db/schema db/migrations internal/controller internal/service internal/repository internal/model internal/queue internal/worker internal/fxapp; do
  test ! -e "$root/$path" || report "forbidden legacy path: $path"
done

auto_migrate_matches=$(find "$root/cmd" "$root/internal" -name '*.go' -type f ! -name '*_test.go' -exec grep -n 'AutoMigrate(' {} + 2>/dev/null || true)
if test -n "$auto_migrate_matches"; then
  report "GORM AutoMigrate is forbidden; db/schema.sql is the only structure source"
fi

for module in github.com/segmentio/kafka-go; do
  if grep -Fq "$module" "$root/go.mod"; then
    report "forbidden legacy dependency: $module"
  fi
done

domain_matches=$(find "$root/internal/modules" -type f -name '*.go' -path '*/domain/*' -exec grep -nE 'github.com/gin-gonic/gin|gorm.io/gorm|riverqueue/river|minio/minio-go' {} + 2>/dev/null || true)
if test -n "$domain_matches"; then
  report "domain code imports infrastructure package"
fi

numbered_implementation_files=$(find "$root/internal" "$root/test/_suite" -type f -iname '*032*' -print 2>/dev/null || true)
if test -n "$numbered_implementation_files"; then
  report "implementation files must use capability semantics instead of plan numbers"
fi

numbered_implementation_symbols=$(grep -Rni '032' "$root/db/schema.sql" "$root/internal" "$root/test/_suite" 2>/dev/null \
  | grep -v 'CanonicalUpgradeTarget = "032"' || true)
if test -n "$numbered_implementation_symbols"; then
  report "implementation symbols and fixtures must use capability semantics instead of plan numbers"
fi

semantic_domain_layer_leaks=$(grep -nE '^type [[:alnum:]_]+(DTO|Row|Record|Manifest|Command|Query|Result) struct' \
  "$root/internal/modules/source/domain/raw_response.go" \
  "$root/internal/modules/source/domain/raw_archive.go" \
  "$root/internal/modules/ingestion/domain/document.go" \
  "$root/internal/modules/ingestion/domain/document_version.go" \
  "$root/internal/modules/ingestion/domain/derived_artifact.go" \
  "$root/internal/modules/knowledge/domain/projection.go" \
  "$root/internal/modules/monitor/domain/intent.go" \
  "$root/internal/modules/monitor/domain/intent_version.go" \
  "$root/internal/modules/monitor/domain/expansion.go" \
  "$root/internal/modules/monitor/domain/intent_run.go" 2>/dev/null || true)
if test -n "$semantic_domain_layer_leaks"; then
  report "new Domain files must contain only entities and value objects, not DTO or persistence/transport shapes"
fi

source_evidence_application_contract_domain_leaks=$(awk '
  /^type [A-Z][A-Za-z0-9_]* (struct|interface) \{/ { inside_public_contract=1 }
  inside_public_contract && /domain\./ { print FILENAME ":" FNR ":" $0 }
  inside_public_contract && /^}/ { inside_public_contract=0 }
' \
  "$root/internal/modules/source/application/raw_evidence_dto.go" \
  "$root/internal/modules/source/application/raw_archive.go" \
  "$root/internal/modules/source/application/raw_evidence_collection.go" \
  "$root/internal/modules/source/application/raw_evidence_rights.go" \
  "$root/internal/modules/source/application/evidence_selection.go" \
  "$root/internal/modules/source/application/source_document_scheduling.go" 2>/dev/null || true)
if test -n "$source_evidence_application_contract_domain_leaks"; then
  report "Source raw-evidence Application public contracts must be POJOs without Domain entity or value-object fields"
fi

architecture_tests='Test(ArchitectureValidationRejectsDirectGinResponsesInModuleTransport|AnalystRoleGapRemainsExplicitAcrossPublishedContracts|P0RuntimeRejectsForbiddenDistributedInfrastructure|ForbiddenInfrastructureDetectorCatchesErroneousIntroductions|DocumentationLifecycleStatusesStayConsistent|M4BusinessFlowCapacityUsesTheFreshStackAndKeepsApprovalExplicit|G5RCCandidateAssessmentGateAggregatesWithoutForgingReleaseApproval|G5AvailabilityRehearsalUsesAnIsolatedSingleHostContract|G5PlannedRunbookDryRunStaysNonActivatingAndRunsInCI|G5ReleaseManifestBindsBuiltImagesSBOMLocksContractsAndScanGates|BackendTestEnvironmentGuardRejectsFormalDatastores|001PartialAcceptanceRecordsVerifiedBaselineAndHonestGaps|002PartialAcceptanceRecordsPassedEvidenceAndHonestReleaseGaps|003And004PartialAcceptancesRecordHonestRemainingGates)$'
if ! (cd "$root" && go run ./test/runner test ./test/architecture -run "$architecture_tests" -count=1); then
  report "executable architecture contracts failed; review Result boundaries, role truth, and forbidden P0 infrastructure"
fi

test "$errors" -eq 0 || exit 1
printf '%s\n' 'Architecture validation passed.'
