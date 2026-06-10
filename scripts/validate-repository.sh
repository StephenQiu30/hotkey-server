#!/usr/bin/env bash
set -euo pipefail

required_files=(
  "README.md"
  "CLAUDE.md"
  "CLAUDE.local.md"
  "AGENTS.md"
  "WORKFLOW.md"
  "go.mod"
  "Dockerfile"
  "docker-compose.yml"
  "docker-compose.prod.yml"
  ".env.example"
  ".env.prod.example"
  "openspec/config.yaml"
  "openspec/specs/agent-governance/spec.md"
  "scripts/vendor-superpowers-skills.sh"
  ".claude/agents/pm.md"
  ".claude/agents/explorer.md"
  ".claude/agents/builder.md"
  ".claude/agents/tester.md"
  ".claude/agents/reporter.md"
  ".claude/skills/harness-local-server/SKILL.md"
  ".claude/skills/harness-playwright-evidence/SKILL.md"
  ".claude/skills/harness-linear-loop/SKILL.md"
  ".claude/skills/harness-quality-gate/SKILL.md"
  ".claude/skills/using-superpowers/SKILL.md"
  ".claude/skills/test-driven-development/SKILL.md"
  ".claude/skills/verification-before-completion/SKILL.md"
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
grep -q "openspec/specs/" CLAUDE.md
grep -q "当前项目边界" CLAUDE.md
grep -q "兼容性" CLAUDE.md
grep -q ".claude/skills/" CLAUDE.local.md
grep -q ".claude/skills/land/SKILL.md" WORKFLOW.md

test ! -d .agents
test ! -d .codex
test ! -f skills-lock.json

test ! -d migrations
test ! -d server
test ! -d sql
test ! -d packages
test ! -d deploy
test ! -f package.json
test ! -f package-lock.json

git diff --check

python3 -m unittest tests/test_workflow_contract.py
