---
layer: PRD
doc_no: "03"
audience:
  - PM
  - Tech-Lead
  - Dev
  - QA
feature_area: "area:subscription"
purpose: "定义 AI 热点频道、用户频道订阅、用户关键词与日报偏好。"
canonical_path: "docs/product/prd/03-频道与订阅PRD.md"
status: draft
version: "1.0.0"
owner: "StephenQiu30"
inputs:
  - "README.md"
  - "WORKFLOW.md"
outputs:
  - "频道与订阅需求边界"
  - "频道与订阅TDD验收标准"
triggers:
  - "频道与订阅范围变更"
  - "对应 Linear issue 拆分或合并"
downstream:
  - "docs/plans/03-频道与订阅实现计划.md"
---

# 03-频道与订阅 PRD

## 1. 背景

日报需要支持频道订阅和用户个性化关键词，既能生成公开频道日报，也能生成用户私有日报。

## 2. 目标

定义 AI 频道、用户频道订阅、用户关键词和日报偏好。

## 3. 范围

- 频道管理数据模型。
- 用户订阅频道。
- 用户添加、启停、删除关键词。
- 用户日报发送时间覆盖全局默认。
- 默认频道 seed 数据。

## 4. 非目标

- 不实现热点聚合算法。
- 不实现 RSS 和邮件发送。
- 不做复杂权限模型。

## 5. 用户故事

- 作为用户，我可以订阅 AI 模型、AI 产品、AI 开源、AI 投融资频道。
- 作为用户，我可以添加自己的关键词影响私有日报。
- 作为管理员，我可以维护频道状态和默认关键词。

## 6. 数据与 API 边界

数据表：`channels`、`user_channel_subscriptions`、`user_keywords`、`system_settings`。

API：用户频道订阅、关键词 CRUD、管理员频道管理。

## 7. 后台任务影响

日报生成任务需要读取频道订阅和用户关键词决定日报范围。

## 8. 配置影响

- 默认时区：`Asia/Shanghai`。
- 管理员全局默认日报发送时间。

## 9. 错误与降级

- 频道禁用后不再生成公开频道日报。
- 用户无订阅时可返回空日报或默认推荐频道。

## 10. 安全与合规

用户只能修改自己的订阅和关键词；管理员才能修改系统频道。

## 11. 验收标准

- Given 用户订阅频道，When 查询订阅，Then 返回 enabled 状态。
- Given 用户添加关键词，When 生成私有日报范围，Then 包含该关键词。
- Given 管理员禁用频道，When scheduler 扫描，Then 不为该频道生成日报任务。

## 12. TDD 验收标准

- Plan 必须先写失败测试，再写最小实现，再运行测试通过。
- 涉及 HTTP API 的任务必须包含 handler/contract 测试。
- 涉及数据库的任务必须包含 migration 和 repository 测试。
- 涉及 Redis 的任务必须包含队列、幂等、重试或降级测试。
- 涉及 DashScope 或 SMTP 的任务必须使用 fake/mock 测试，不依赖真实外部服务通过基础测试。

## 13. Harness 执行要求

每个 Linear issue 必须包含：

```text
1. Read PRD: docs/product/prd/03-频道与订阅PRD.md
2. Read Plan: docs/plans/03-频道与订阅实现计划.md
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

