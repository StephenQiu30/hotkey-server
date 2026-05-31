---
layer: PRD
doc_no: "07"
audience:
  - PM
  - Tech-Lead
  - Dev
  - QA
feature_area: "area:embedding"
purpose: "定义 DashScope embedding、相似度计算与热点簇聚合。"
canonical_path: "docs/product/prd/07-Embedding与热点聚合PRD.md"
status: draft
version: "1.0.0"
owner: "StephenQiu30"
inputs:
  - "README.md"
  - "WORKFLOW.md"
outputs:
  - "Embedding与热点聚合需求边界"
  - "Embedding与热点聚合TDD验收标准"
triggers:
  - "Embedding与热点聚合范围变更"
  - "对应 Linear issue 拆分或合并"
downstream:
  - "docs/plans/07-Embedding与热点聚合实现计划.md"
---

# 07-Embedding与热点聚合 PRD

## 1. 背景

热点检测需要用 embedding 和关键词把相似内容聚合成热点簇。

## 2. 目标

使用 DashScope `text-embedding-v2` 生成向量，并基于相似度、关键词和时间窗口聚合热点。

## 3. 范围

- `item_embeddings` 存储。
- embedding 任务状态和错误。
- 相似度阈值配置。
- 热点簇和内容关联。

## 4. 非目标

- 不生成最终中文日报。
- 不做复杂跨语言事件图谱。
- 不做人工标注训练。

## 5. 用户故事

- 作为系统，我可以将多个来源报道的同一 AI 事件聚合到一个热点。
- 作为管理员，我可以看到 embedding 失败原因。
- 作为用户，我能看到去重聚合后的热点而不是重复链接。

## 6. 数据与 API 边界

数据表：`item_embeddings`、`hotspot_clusters`、`hotspot_items`。

管理员 API 可查看聚合结果和相关 source items。

## 7. 后台任务影响

`generate_embedding` 成功后触发或参与 `cluster_hotspots`。

## 8. 配置影响

- `HOTKEY_DASHSCOPE_API_KEY`
- embedding model 默认 `text-embedding-v2`
- similarity threshold

## 9. 错误与降级

DashScope 未配置时 embedding 任务标记 `failed_config`，系统可退回标题关键词规则聚合。

## 10. 安全与合规

发送给 DashScope 的内容应限制长度，不发送用户隐私字段。

## 11. 验收标准

- Given 新 source item，When embedding 任务执行，Then 保存 vector 和模型名。
- Given 两条相似内容，When 聚合，Then 关联到同一 hotspot cluster。
- Given DashScope 未配置，When 执行任务，Then 标记 `failed_config`。

## 12. TDD 验收标准

- Plan 必须先写失败测试，再写最小实现，再运行测试通过。
- 涉及 HTTP API 的任务必须包含 handler/contract 测试。
- 涉及数据库的任务必须包含 migration 和 repository 测试。
- 涉及 Redis 的任务必须包含队列、幂等、重试或降级测试。
- 涉及 DashScope 或 SMTP 的任务必须使用 fake/mock 测试，不依赖真实外部服务通过基础测试。

## 13. Harness 执行要求

每个 Linear issue 必须包含：

```text
1. Read PRD: docs/product/prd/07-Embedding与热点聚合PRD.md
2. Read Plan: docs/plans/07-Embedding与热点聚合实现计划.md
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

