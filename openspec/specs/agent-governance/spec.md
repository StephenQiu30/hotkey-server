# Agent Governance

## Purpose

Define the accepted governance baseline for `hotkey-server`, including OpenSpec usage, project boundaries, and repository validation.

## Requirements

### Requirement: Claude Agent Assets Must Live Under .claude
The repository MUST keep agent roles and skills only under `.claude/agents/` and `.claude/skills/`.

#### Scenario: Contributor inspects agent configuration
- **GIVEN** a contributor clones the repository
- **WHEN** they look for agent workflow files
- **THEN** roles exist under `.claude/agents/`
- **AND** skills exist under `.claude/skills/`
- **AND** parallel trees such as `.agents/` or `.codex/skills/` are not part of the baseline

### Requirement: OpenSpec Baseline Must Exist
The repository MUST keep an OpenSpec baseline under `openspec/specs/` for long-lived governance requirements.

#### Scenario: Fresh clone inspects normative assets
- **GIVEN** a contributor clones the repository
- **WHEN** they review governance files
- **THEN** `openspec/config.yaml` exists
- **AND** at least one spec exists under `openspec/specs/`

### Requirement: Tests Must Stay Inside Accepted Boundary
The repository MUST NOT add compatibility-only tests, PRD/Plan count pairing tests, or OpenAPI/SQL string-matching contract tests unless explicitly required by an accepted spec.

#### Scenario: Compatibility fallback test is proposed
- **GIVEN** a change adds tests only to lock legacy fallback or file-content string matches
- **WHEN** that behavior is not required by accepted specs or core business rules
- **THEN** the change is out of scope for the governance baseline

### Requirement: Validation Must Check Governance Assets
Repository validation MUST fail when required governance files, harness skills, or OpenSpec baseline assets are missing.

#### Scenario: Required governance asset removed
- **GIVEN** `openspec/config.yaml`, `openspec/specs/agent-governance/spec.md`, or required `.claude/skills/harness-*` files are missing
- **WHEN** `make validate` runs
- **THEN** `scripts/validate-repository.sh` fails
