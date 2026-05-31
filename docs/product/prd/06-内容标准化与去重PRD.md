---
layer: PRD
doc_no: "06"
audience:
  - PM
  - Tech-Lead
  - Dev
  - QA
feature_area: "area:content"
purpose: "定义跨来源内容标准化、canonical URL、hash 去重与采集记录。"
canonical_path: "docs/product/prd/06-内容标准化与去重PRD.md"
status: draft
version: "1.0.0"
owner: "StephenQiu30"
inputs:
  - "README.md"
  - "WORKFLOW.md"
outputs:
  - "内容标准化与去重需求边界"
  - "内容标准化与去重TDD验收标准"
triggers:
  - "内容标准化与去重范围变更"
  - "对应 Linear issue 拆分或合并"
downstream:
  - "docs/plans/06-内容标准化与去重实现计划.md"
---

# 06-内容标准化与去重 PRD

## 1. 背景

不同来源返回的内容格式不同，必须统一标准化并去重后才能参与 embedding、聚合和日报。

## 2. 目标

定义标准内容模型、canonical URL、content hash、语言和 metadata 处理。

## 3. 范围

- 标准化 `source_items`。
- URL/hash/title 去重。
- 保存原始 metadata。
- 记录 fetched_at、published_at、language。

## 4. 非目标

- 不实现复杂正文抽取质量优化。
- 不实现热点聚合。
- 不生成 AI 摘要。

## 5. 用户故事

- 作为系统，我可以把 RSS item 和公开页面 item 存成同一结构。
- 作为系统，我可以避免同一 URL 重复入库。
- 作为后续任务，我可以读取稳定 source item 生成 embedding。

## 6. 数据与 API 边界

数据表：`source_items`。

管理员 API 可查看 source item 和去重状态。

## 7. 后台任务影响

采集任务成功后写入 source_items，并为新增 item 入队 `generate_embedding`。

## 8. 配置影响

可配置正文最大长度、摘要预处理长度和默认语言。

## 9. 错误与降级

内容缺正文时允许仅标题和 URL 入库，但标记质量较低。

## 10. 安全与合规

只采集公开内容，保留原始 URL，便于追溯和删除。

## 11. 验收标准

- Given 同一 canonical URL，When 重复入库，Then 只保留一条 source item。
- Given 内容 hash 相同，When URL 不同，Then 标记重复或合并引用。
- Given 新 item 入库，Then 创建 embedding 任务。

## 12. TDD 验收标准

- Plan 必须先写失败测试，再写最小实现，再运行测试通过。
- 涉及 HTTP API 的任务必须包含 handler/contract 测试。
- 涉及数据库的任务必须包含 migration 和 repository 测试。
- 涉及 Redis 的任务必须包含队列、幂等、重试或降级测试。
- 涉及 DashScope 或 SMTP 的任务必须使用 fake/mock 测试，不依赖真实外部服务通过基础测试。

## 13. Harness 执行要求

每个 Linear issue 必须包含：

```text
1. Read PRD: docs/product/prd/06-内容标准化与去重PRD.md
2. Read Plan: docs/plans/06-内容标准化与去重实现计划.md
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

