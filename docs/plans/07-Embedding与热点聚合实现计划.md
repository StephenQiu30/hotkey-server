---
layer: Plan
doc_no: "07"
audience:
  - Tech-Lead
  - Dev
  - QA
feature_area: "area:embedding"
purpose: "实现 DashScope embedding、pgvector 存储和热点簇聚合。"
canonical_path: "docs/plans/07-Embedding与热点聚合实现计划.md"
status: draft
version: "1.0.0"
owner: "StephenQiu30"
inputs:
  - "docs/product/prd/07-Embedding与热点聚合PRD.md"
outputs:
  - "Embedding与热点聚合实现任务"
triggers:
  - "embedding 模型或聚合策略变化"
downstream:
  - "由 WORKFLOW.md 指定的 Symphony / Linear 流程接管"
---

# 07-Embedding与热点聚合实现计划

## 1. 目标

使用 DashScope `text-embedding-v2` 为内容生成向量，并按相似度和时间窗口聚合热点。

## 2. 文件清单

- 创建：`migrations/000007_embeddings_hotspots.up.sql`
- 创建：`internal/platform/dashscope/`
- 创建：`internal/domain/hotspot/`
- 创建：`internal/service/embedding/`
- 创建：`internal/service/hotspot/`
- 创建：`internal/repository/postgres/hotspotrepo/`
- 创建：`internal/worker/handlers/embed/`
- 创建：`internal/worker/handlers/cluster/`

## 3. 任务拆解

1. 创建 `item_embeddings`、`hotspot_clusters`、`hotspot_items`。
2. 实现 DashScope embedding client interface 和 mock。
3. 实现 embedding worker。
4. 实现相似度查询和聚合 service。
5. DashScope 未配置时标记 `failed_config`。

## 4. TDD 与验证

- mock DashScope 返回向量后保存 embedding。
- 两条相似内容聚合为同一 cluster。
- 未配置 API key 时任务失败但不 panic。

## 5. 执行顺序

1. `test:` dashscope mock、embedding service、cluster service 失败测试。
2. `impl:` migration、client、repo、service、worker。
3. `refactor:` provider interface。

## 6. 回滚策略

回滚 embedding/hotspot migration，停用 embed/cluster worker handler。

## 7. 验收命令

```bash
go test ./...
python3 -m unittest discover -s tests
```

## 8. Symphony / Linear 要求

任务状态、标签和流转规则完全以本仓库 `WORKFLOW.md` 和本地 Symphony 实现为准。Plan 不定义额外状态、不发明额外标签、不覆盖 Symphony 的状态机。

Linear issue 只承载任务内容：PRD 路径、Plan 路径、任务范围、禁止范围、TDD 验收命令和回写要求。Symphony 负责监听 active states、创建 workspace、运行 Codex，并按 `WORKFLOW.md` prompt 驱动执行。

## 9. 变更记录

| 日期 | 作者 | 版本 | 变更说明 |
| --- | --- | --- | --- |
| 2026-05-31 | StephenQiu30 | 1.0.0 | 初版 |
