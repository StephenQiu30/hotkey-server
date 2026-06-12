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
grep -q "harness-quality-gate" WORKFLOW.md
grep -q "superpowers" WORKFLOW.md

test ! -d .agents
test ! -f skills-lock.json

git diff --check
