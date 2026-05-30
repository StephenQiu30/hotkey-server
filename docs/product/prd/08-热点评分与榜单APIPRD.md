---
layer: PRD
doc_no: "08"
audience:
  - PM
  - Tech-Lead
  - Dev
  - QA
feature_area: "area:hotspot"
purpose: "定义热点评分模型、榜单 API、详情 API 与评分解释。"
canonical_path: "docs/product/prd/08-热点评分与榜单APIPRD.md"
status: draft
version: "1.0.0"
owner: "StephenQiu30"
inputs:
  - "README.md"
  - "WORKFLOW.md"
outputs:
  - "热点评分与榜单API需求边界"
  - "热点评分与榜单APITDD验收标准"
triggers:
  - "热点评分与榜单API范围变更"
  - "对应 Linear issue 拆分或合并"
downstream:
  - "docs/plans/08-热点评分与榜单API实现计划.md"
---

# 08-热点评分与榜单API PRD

## 1. 背景

热点需要可解释排序，帮助用户理解为什么某事件进入榜单。

## 2. 目标

定义相关度、热度、新鲜度、可信度和综合分，并提供榜单与详情 API。

## 3. 范围

- 热点评分计算。
- 评分解释 JSON。
- 热点列表 API。
- 热点详情 API。
- 来源证据列表。

## 4. 非目标

- 不生成 AI 日报正文。
- 不做用户界面。
- 不做复杂传播路径仲裁。

## 5. 用户故事

- 作为用户，我可以看到当天 AI 热点榜。
- 作为用户，我可以打开热点详情查看来源证据。
- 作为管理员，我可以判断评分异常来自哪一项。

## 6. 数据与 API 边界

数据表：`hotspot_scores`。

API：`GET /api/v1/hotspots`、`GET /api/v1/hotspots/{id}`。

## 7. 后台任务影响

`score_hotspots` 读取热点簇和 source items，写入 hotspot_scores。

## 8. 配置影响

可配置评分权重：相关度、热度、新鲜度、可信度。

## 9. 错误与降级

缺少 embedding 时可以使用关键词和来源数量计算基础分。

## 10. 安全与合规

详情 API 只返回公开来源内容摘要和链接，不暴露内部 token。

## 11. 验收标准

- Given 热点簇有多来源内容，When 评分，Then total_score 大于单来源低质量内容。
- Given 请求热点列表，When 有评分数据，Then 按 total_score 排序。
- Given 请求详情，Then 返回 source refs 和 explanation。

## 12. TDD 验收标准

- Plan 必须先写失败测试，再写最小实现，再运行测试通过。
- 涉及 HTTP API 的任务必须包含 handler/contract 测试。
- 涉及数据库的任务必须包含 migration 和 repository 测试。
- 涉及 Redis 的任务必须包含队列、幂等、重试或降级测试。
- 涉及 DashScope 或 SMTP 的任务必须使用 fake/mock 测试，不依赖真实外部服务通过基础测试。

## 13. Harness 执行要求

每个 Linear issue 必须包含：

```text
1. Read PRD: docs/product/prd/08-热点评分与榜单APIPRD.md
2. Read Plan: docs/plans/08-热点评分与榜单API实现计划.md
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

