#!/usr/bin/env bash
set -euo pipefail

required_files=(
  "README.md"
  "CLAUDE.md"
  "CLAUDE.local.md"
  "WORKFLOW.md"
  ".env.example"
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
  "docs/design/001-v1数据库设计.md"
  "docs/acceptance/README.md"
  "docs/operations/README.md"
  "db/schema.sql"
)

for file in "${required_files[@]}"; do
  test -f "$file"
done

grep -q "tracker:" WORKFLOW.md
grep -q "kind: linear" WORKFLOW.md
grep -q "project_slug" WORKFLOW.md
grep -q "## Claude Workpad" WORKFLOW.md
grep -q "command: claude" WORKFLOW.md
grep -q "Human Review" WORKFLOW.md
grep -q 'test:`、`docs:`、`impl:`、`feat:`、`chore:`、`refactor:`' CLAUDE.md
grep -q "test-first 提交顺序" CLAUDE.md
grep -q '`impl:` commit' CLAUDE.md
grep -q "需求到数据库设计门禁" CLAUDE.md
grep -q "harness-quality-gate" WORKFLOW.md
grep -q "superpowers" WORKFLOW.md
grep -q "CREATE EXTENSION IF NOT EXISTS vector" db/schema.sql
grep -q "CREATE TABLE users" db/schema.sql
grep -q "CREATE TABLE monitored_topics" db/schema.sql
grep -q "CREATE TABLE source_items" db/schema.sql
grep -q "CREATE TABLE hotspot_events" db/schema.sql
grep -q "CREATE TABLE report_subscriptions" db/schema.sql
grep -q "CREATE TABLE storage_objects" db/schema.sql
grep -q "CREATE TABLE deletion_requests" db/schema.sql
grep -q "hotkey-server/db/schema.sql" docs/design/001-v1数据库设计.md

test ! -d .agents
test ! -f skills-lock.json

git diff --check
