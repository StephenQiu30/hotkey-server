---
layer: Acceptance
doc_no: "01"
audience:
  - Tech-Lead
  - Dev
  - QA
feature_area: "area:governance"
purpose: "记录 HotKey Server 项目治理与 Symphony 编排的验收证据。"
canonical_path: "docs/acceptance/01-项目治理与Symphony编排验收.md"
status: draft
version: "1.0.0"
owner: "StephenQiu30"
inputs:
  - "docs/product/prd/01-项目治理与Symphony编排PRD.md"
  - "docs/plans/01-项目治理与Symphony编排实现计划.md"
outputs:
  - "Symphony 编排验收记录"
---

# 01-项目治理与Symphony编排验收

## Linear project

- Project: HotKey Server AI 热点日报
- Team: STE
- Initial orchestration issue: STE-37
- Scope: only `hotkey-server`; do not modify `hotkey-web` or `hotkey-miniapp`.

## Symphony 启动要求

Symphony 读取根目录 `WORKFLOW.md`，并通过环境变量注入项目级配置：

- `SYMPHONY_LINEAR_PROJECT_SLUG`: Linear project slug used by the tracker.
- `SYMPHONY_WORKSPACE_ROOT`: isolated workspace root for issue execution.
- `HOTKEY_SERVER_REPOSITORY_URL`: clone source for `hotkey-server` workspaces.

Startup must keep project-specific values in the environment rather than hardcoding them in `WORKFLOW.md`.

## Issue 编排

Each issue must follow the project workflow:

1. Symphony watches active states from `WORKFLOW.md`.
2. A `Todo` issue is moved to `In Progress` before implementation starts.
3. The agent creates or reuses exactly one `## Codex Workpad` comment.
4. The workpad records plan, progress, validation evidence, confusions, and handoff notes.
5. The agent syncs `origin/main`, reads the referenced PRD and Plan, writes a failing test first, implements the minimum change, then validates.
6. The agent creates or updates a PR, adds the GitHub `symphony` label, links it to the issue, and moves the issue to `Human Review` only after validation and PR feedback sweep pass.

The workflow must not define extra Linear labels, custom statuses, or a separate state machine that conflicts with Symphony.

## Blocker 编排

Use the blocked-access escape hatch only for missing required non-GitHub auth or tools that cannot be resolved in-session. The workpad must record:

- what is missing,
- why it blocks acceptance or validation,
- the exact human action required to unblock execution.

GitHub access is not a default blocker; the agent must attempt documented fallback strategies before treating publish or PR work as blocked.

## 验证证据

Required commands for this governance slice:

```bash
make test
python3 -m unittest discover -s tests
```

Repository handoff also requires:

```bash
gofmt -w cmd internal
go test ./...
```

The governance tests must fail if a PRD under `docs/product/prd/` does not have a matching Plan under `docs/plans/`, or if this acceptance document loses the Linear/Symphony orchestration evidence above.
