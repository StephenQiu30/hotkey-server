---
layer: PRD
doc_no: "01"
audience:
  - PM
  - Tech-Lead
  - Dev
  - QA
feature_area: "area:governance"
purpose: "建立Feature PRD、Plan、Linear issue 与 Symphony harness 自动执行闭环。"
canonical_path: "docs/product/prd/01-项目治理与Symphony编排PRD.md"
status: draft
version: "1.0.0"
owner: "StephenQiu30"
inputs:
  - "README.md"
  - "WORKFLOW.md"
outputs:
  - "项目治理与Symphony编排需求边界"
  - "项目治理与Symphony编排TDD验收标准"
triggers:
  - "项目治理与Symphony编排范围变更"
  - "对应 Linear issue 拆分或合并"
downstream:
  - "docs/plans/01-项目治理与Symphony编排实现计划.md"
---

# 01-项目治理与Symphony编排 PRD

## 1. 背景

HotKey Server 已完成旧平台化代码清理和最小 Go server 骨架重启。后续功能必须按 feature 拆 PRD、Plan 和 Linear issue，并由本地 `Agents` 目录中运行的 Symphony 自动监听执行。

## 2. 目标

建立项目标准交付链路：Feature Idea -> PRD -> Plan -> Linear issue -> Symphony harness 执行 -> 测试 -> 提交/PR -> Linear 回写。

## 3. 范围

- 定义 PRD、Plan、Linear issue 的文档规范。
- 定义每个 issue 的 TDD 和 harness 执行要求。
- 定义 `WORKFLOW.md` 与 Symphony 固定规范的关系。
- 定义 feature 编号和执行顺序。

## 4. 非目标

- 不实现业务功能。
- 不修改 Symphony 调度规范。
- 不实现 Web 或小程序。

## 5. 用户故事

- 作为项目负责人，我希望每个 feature 都有独立 PRD 和 Plan，避免需求混杂。
- 作为开发 agent，我希望 Linear issue 明确引用 PRD/Plan 和验收命令，以便独立执行。
- 作为 reviewer，我希望每个任务都能追溯到需求、测试和提交。

## 6. 数据与 API 边界

本 PRD 不新增业务表和 HTTP API。它要求后续涉及数据或 API 的 PRD 必须声明 migration、OpenAPI 和 contract test。

## 7. 后台任务影响

本 PRD 不新增后台任务。它要求后续涉及 scheduler、Redis queue、worker handler 的 PRD 必须声明任务类型、payload、幂等 key、重试和失败状态。

## 8. 配置影响

- 继续使用根目录 `WORKFLOW.md` 作为 Symphony 工作流入口。
- Linear 项目通过 `$SYMPHONY_LINEAR_PROJECT_SLUG` 配置。
- Symphony 本体在本地 `Agents` 目录运行，不进入 HotKey server 代码。

## 9. 错误与降级

如果 Symphony 未运行，Linear issue 仍作为任务事实源，开发者可以手动执行 Plan，但必须回写同样的测试证据。

## 10. 安全与合规

Linear issue 不得包含明文 API key、SMTP 密码、DashScope key 或用户隐私数据。

## 11. 验收标准

- `docs/product/prd/` 下存在 12 个 feature PRD。
- 每个 PRD 都有 frontmatter、范围、非目标、TDD 验收和 harness 执行要求。
- 后续 Plan 必须能映射到 PRD。
- `WORKFLOW.md` 仍符合 Symphony 固定格式。

## 12. TDD 验收标准

- Plan 必须先写失败测试，再写最小实现，再运行测试通过。
- 涉及 HTTP API 的任务必须包含 handler/contract 测试。
- 涉及数据库的任务必须包含 migration 和 repository 测试。
- 涉及 Redis 的任务必须包含队列、幂等、重试或降级测试。
- 涉及 DashScope 或 SMTP 的任务必须使用 fake/mock 测试，不依赖真实外部服务通过基础测试。

## 13. Harness 执行要求

每个 Linear issue 必须包含：

```text
1. Read PRD: docs/product/prd/01-项目治理与Symphony编排PRD.md
2. Read Plan: docs/plans/01-项目治理与Symphony编排实现计划.md
3. Write failing test first
4. Run expected failing command
5. Implement minimal code
6. Run required verification
7. Update OpenAPI or migrations when needed
8. Commit with Chinese message
9. Report commands, results, risks, and changed files back to Linear
```

Symphony 在本地 `Agents` 目录监听 Linear issue，并在独立 workspace 中执行。HotKey 不重写 Symphony 规范，只在 `WORKFLOW.md` prompt 中约束执行行为。

## 14. PRD 自审清单

- 本 PRD 是否只覆盖一个 feature。
- 用户、管理员或系统任务的输入输出是否明确。
- 范围和非目标是否能阻止越界实现。
- 数据、API、任务和配置影响是否可拆成 Plan。
- 验收标准是否可测试、可自动化、可在 harness 中执行。
- 是否遵循 TDD，且不要求先写生产代码。

## 15. 变更记录

| 日期 | 作者 | 版本 | 变更说明 |
| --- | --- | --- | --- |
| 2026-05-31 | StephenQiu30 | 1.0.0 | 初版，按 server-only AI 热点检测与日报服务 feature 拆分创建 |

