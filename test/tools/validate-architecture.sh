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

for module in github.com/segmentio/kafka-go github.com/tmc/langchaingo; do
  if grep -Fq "$module" "$root/go.mod"; then
    report "forbidden legacy dependency: $module"
  fi
done

domain_matches=$(find "$root/internal/modules" -type f -name '*.go' -path '*/domain/*' -exec grep -nE 'github.com/gin-gonic/gin|gorm.io/gorm|riverqueue/river|minio/minio-go' {} + 2>/dev/null || true)
if test -n "$domain_matches"; then
  report "domain code imports infrastructure package"
fi

if ! (cd "$root" && go test ./test/architecture -run TestArchitectureValidationRejectsDirectGinResponsesInModuleTransport -count=1); then
  report "direct Gin response output is forbidden in module transport; use internal/platform/http Result helpers and Wrap"
fi

test "$errors" -eq 0 || exit 1
printf '%s\n' 'Architecture validation passed.'
