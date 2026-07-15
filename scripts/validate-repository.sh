#!/usr/bin/env sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
crud="$root/internal/shared/repository/crud.go"
model="$root/internal/platform/database/model/model.go"
errors=0

report() {
  printf '%s\n' "$1" >&2
  errors=1
}

test -f "$crud" || report "missing shared CRUD contract: internal/shared/repository/crud.go"
test -f "$model" || report "missing database record mapping: internal/platform/database/model/model.go"
for method in 'Create(' 'GetByID(' 'List(' 'Update(' 'Delete('; do
  grep -Fq "$method" "$crud" 2>/dev/null || report "CRUD contract is missing method: $method"
done

test -f "$root/db/schema.sql" || report "missing complete schema: db/schema.sql"
test ! -e "$root/db/schema" || report "split Schema directory must not return"
test ! -e "$root/db/migrations" || report "migration directory must not return"

test "$errors" -eq 0 || exit 1
printf '%s\n' 'Repository validation passed.'
