#!/usr/bin/env bash
set -euo pipefail

required_files=(
  "README.md"
  "CLAUDE.md"
  "CLAUDE.local.md"
  "WORKFLOW.md"
  "openspec/config.yaml"
  "openspec/specs/agent-governance/spec.md"
  ".env.example"
  ".claude/skills/agent-browser/SKILL.md"
  ".claude/skills/openspec-new-change/SKILL.md"
  ".claude/skills/openspec-apply-change/SKILL.md"
  ".claude/skills/openspec-verify-change/SKILL.md"
  ".claude/skills/harness-local-server/SKILL.md"
  ".claude/skills/harness-playwright-evidence/SKILL.md"
  ".claude/skills/harness-linear-loop/SKILL.md"
  ".claude/skills/harness-quality-gate/SKILL.md"
  ".claude/skills/using-superpowers/SKILL.md"
  ".claude/skills/test-driven-development/SKILL.md"
  ".claude/skills/executing-plans/SKILL.md"
  ".claude/skills/verification-before-completion/SKILL.md"
  "scripts/vendor-superpowers-skills.sh"
  ".claude/skills/debug/SKILL.md"
  ".claude/skills/commit/SKILL.md"
  ".claude/skills/pull/SKILL.md"
  ".claude/skills/push/SKILL.md"
  ".claude/skills/land/SKILL.md"
  ".claude/skills/land/land_watch.py"
  ".claude/skills/linear/SKILL.md"
  ".github/pull_request_template.md"
  "docs/README.md"
  "docs/prd/README.md"
  "docs/plans/README.md"
  "docs/design/README.md"
  "docs/acceptance/README.md"
  "docs/operations/README.md"
  "Dockerfile"
  "docker-compose.yml"
  "Makefile"
  "cmd/hotkey/main.go"
  "scripts/validate-repository.sh"
  "scripts/validate-architecture-boundaries.sh"
)

echo "=== Required files ==="
for file in "${required_files[@]}"; do
  if [ ! -f "$file" ]; then
    echo "FAIL: missing $file"
    exit 1
  fi
done
echo "OK: all required files present"

echo ""
echo "=== WORKFLOW.md content ==="
grep -q "tracker:" WORKFLOW.md
grep -q "kind: linear" WORKFLOW.md
grep -q "project_slug" WORKFLOW.md
grep -q "## Claude Workpad" WORKFLOW.md
grep -q "command: claude" WORKFLOW.md
grep -q "Human Review" WORKFLOW.md
grep -q "harness-quality-gate" WORKFLOW.md
grep -q "superpowers" WORKFLOW.md
echo "OK: WORKFLOW.md contains required markers"

echo ""
echo "=== Anti-patterns ==="
test ! -d .agents
test ! -f skills-lock.json
echo "OK: no anti-patterns found"

echo ""
echo "=== Implementation files ==="
impl_files=(
  "cmd/hotkey/main.go"
  "internal/config/config.go"
  "internal/platform/http/router.go"
  "db/schema.sql"
)
for file in "${impl_files[@]}"; do
  if [ ! -f "$file" ]; then
    echo "FAIL: missing implementation file $file"
    exit 1
  fi
done
echo "OK: all implementation files present"

echo ""
echo "=== Schema validation ==="
if ! grep -q "create table" db/schema.sql; then
  echo "FAIL: db/schema.sql contains no CREATE TABLE statements"
  exit 1
fi
table_count=$(grep -c "create table" db/schema.sql)
echo "OK: db/schema.sql contains $table_count tables"

echo ""
echo "=== Go tests ==="
go test ./...
echo "OK: all Go tests pass"

echo ""
echo "=== Knowledge writeback roundtrip ==="
go test ./tests/integration -run TestKnowledgeWritebackRoundtrip -v
echo "OK: knowledge writeback roundtrip regression passed"

echo ""
echo "=== Go build ==="
go build ./...
echo "OK: Go code compiles"

echo ""
echo "=== Docker Compose ==="
if [ ! -f .env ]; then
  cp .env.example .env
fi
docker compose config >/dev/null
echo "OK: docker compose config valid"

echo ""
echo "=== Git diff check ==="
if git diff --check; then
  echo "OK: no whitespace errors"
else
  echo "FAIL: whitespace errors detected"
  exit 1
fi

echo ""
echo "=== Architecture boundary validation ==="
bash "$(dirname "$0")/validate-architecture-boundaries.sh"
echo "OK: architecture boundary validation passed"

echo ""
echo "=== Runtime API smoke ==="
bash "$(dirname "$0")/smoke-api.sh"
echo "OK: runtime API smoke passed"

echo ""
echo "=== All validations passed ==="
