---
layer: PRD
doc_no: "04"
audience:
  - PM
  - Tech-Lead
  - Dev
  - QA
feature_area: "area:source"
purpose: "定义 RSS、官方 API、公开页面来源及合规采集策略。"
canonical_path: "docs/product/prd/04-来源与采集策略PRD.md"
status: draft
version: "1.0.0"
owner: "StephenQiu30"
inputs:
  - "README.md"
  - "WORKFLOW.md"
outputs:
  - "来源与采集策略需求边界"
  - "来源与采集策略TDD验收标准"
triggers:
  - "来源与采集策略范围变更"
  - "对应 Linear issue 拆分或合并"
downstream:
  - "docs/plans/04-来源与采集策略实现计划.md"
---

# 04-来源与采集策略 PRD

## 1. 背景

AI 热点检测依赖 RSS、公开站点、官方 API 和公开页面轻量抓取。不同平台必须独立配置合规策略、限速和启停。

## 2. 目标

建立来源模型、采集策略和适配器边界，第一版优先支持 RSS 和通用公开页面适配器。

## 3. 范围

- 来源类型：`rss`、`official_api`、`public_page`。
- 鉴权方式：`none`、`api_key`、`oauth`。
- 来源启停、抓取频率、限速、合规说明、失败状态。
- 来源与频道关联。
- 采集运行记录。

## 4. 非目标

- 不做绕过授权、模拟登录或私域采集。
- 不保证第一版覆盖所有社交平台。
- 不把平台专属逻辑写死到 scheduler。

## 5. 用户故事

- 作为管理员，我可以配置 AI 新闻 RSS 来源并关联频道。
- 作为管理员，我可以配置公开页面来源和限速。
- 作为系统，我可以记录每次采集成功、失败和抓取数量。

## 6. 数据与 API 边界

数据表：`sources`、`source_channel_links`、`collection_runs`。

API：管理员来源 CRUD、启停、测试采集、查看运行记录。

## 7. 后台任务影响

`scheduler` 每小时扫描 enabled sources，入队 `collect_source`。

## 8. 配置影响

平台 API key 通过环境变量或安全配置引用，不在 sources 表存明文 secret。

## 9. 错误与降级

- 单个来源失败只记录 `collection_runs`，不影响其他来源。
- 连续失败可自动暂停来源或标记 degraded。

## 10. 安全与合规

每个来源必须填写 `compliance_note`，公开页面抓取必须配置限速。

## 11. 验收标准

- Given enabled RSS source，When collect_source 执行，Then 记录 collection_run。
- Given disabled source，When scheduler 扫描，Then 不入队采集任务。
- Given public_page source 无合规说明，When 创建，Then 返回校验错误。

## 12. TDD 验收标准

- Plan 必须先写失败测试，再写最小实现，再运行测试通过。
- 涉及 HTTP API 的任务必须包含 handler/contract 测试。
- 涉及数据库的任务必须包含 migration 和 repository 测试。
- 涉及 Redis 的任务必须包含队列、幂等、重试或降级测试。
- 涉及 DashScope 或 SMTP 的任务必须使用 fake/mock 测试，不依赖真实外部服务通过基础测试。

## 13. Harness 执行要求

每个 Linear issue 必须包含：

```text
1. Read PRD: docs/product/prd/04-来源与采集策略PRD.md
2. Read Plan: docs/plans/04-来源与采集策略实现计划.md
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

