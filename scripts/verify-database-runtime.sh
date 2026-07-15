#!/usr/bin/env sh
set -eu

dsn=${HOTKEY_TEST_DSN:-}
if test -z "$dsn"; then
  printf '%s\n' 'HOTKEY_TEST_DSN is required' >&2
  exit 1
fi

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)

# Keep the capacity query away from the command fixture and the isolated Go
# test databases. URI-shaped DSNs are required so the disposable database can
# retain the caller's host, credentials, and connection options.
base_dsn=${dsn%%\?*}
query=
case "$dsn" in
  *\?*) query="?${dsn#*\?}" ;;
esac
case "$base_dsn" in
  postgres://*|postgresql://*) ;;
  *)
    printf '%s\n' 'HOTKEY_TEST_DSN must be a PostgreSQL URL for disposable capacity verification' >&2
    exit 1
    ;;
esac
capacity_database="hotkey_capacity_$$"
capacity_dsn="${base_dsn%/*}/${capacity_database}${query}"
cleanup_capacity() {
  dropdb --if-exists --force --maintenance-db="$dsn" "$capacity_database" >/dev/null 2>&1 || true
}
trap cleanup_capacity EXIT HUP INT TERM

# The fixture database is explicitly disposable. Recreating public verifies the
# empty-only initialization path and prevents test state from masking schema
# drift. Extensions are intentionally recreated by the embedded schema.
psql "$dsn" -v ON_ERROR_STOP=1 <<'SQL' >/dev/null
DROP SCHEMA public CASCADE;
CREATE SCHEMA public;
GRANT ALL ON SCHEMA public TO PUBLIC;
SQL

(
  cd "$root"
  HOTKEY_DATABASE_URL="$dsn" go run ./cmd/hotkey db init --empty-only --confirm-empty
  HOTKEY_DATABASE_URL="$dsn" go run ./cmd/hotkey db verify
  HOTKEY_TEST_DSN="$dsn" go test ./internal/platform/database ./internal/shared/repository -count=1
  createdb --maintenance-db="$dsn" --template=template0 "$capacity_database"
  HOTKEY_DATABASE_URL="$capacity_dsn" go run ./cmd/hotkey db init --empty-only --confirm-empty
  HOTKEY_TEST_DSN="$capacity_dsn" sh scripts/generate-capacity-fixture.sh
)

printf '%s\n' 'Database runtime verification passed.'
