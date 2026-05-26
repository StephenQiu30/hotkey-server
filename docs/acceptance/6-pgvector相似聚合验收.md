---
layer: Acceptance
doc_no: "6"
audience:
  - PM
  - Tech-Lead
  - Dev
  - QA
feature_area: "area:event"
purpose: "记录 pgvector 相似聚合任务的实现边界、验证命令和验收结论。"
canonical_path: "docs/acceptance/6-pgvector相似聚合验收.md"
status: accepted
version: "1.0.0"
owner: "StephenQiu30"
inputs:
  - docs/product/prd/6-pgvector相似聚合PRD.md
  - docs/plans/6-pgvector相似聚合实现计划.md
outputs:
  - pgvector 相似聚合验收证据
---

# 6-pgvector相似聚合验收

## 1. 实现范围

- 新增候选事件簇服务，支持把 SourceItem 候选内容归入事件簇。
- 向量可用时使用余弦相似度作为 pgvector 召回契约，达到阈值后按 `vector` 方式归簇。
- 向量不可用或维度不匹配时，可退回 hash/标题规则聚合，匹配方式为 `rule`。
- 首个候选内容创建新事件簇，匹配方式为 `seed`。
- API 响应展示 `matchMethod` 和 `similarity`。

## 2. API 影响

- `POST /api/v1/admin/event-candidates`
- `GET /api/v1/admin/event-clusters`
- `/openapi.json` 已包含候选事件簇接口。

## 3. 验收命令

```bash
go test ./...
git diff --check
find docs/product/prd docs/plans -maxdepth 1 -type f | sort -V
```

## 4. 验收结论

- 相似内容可聚合为事件簇，并展示相似度和匹配方式。
- pgvector 不可用时可退回规则聚合。
- 当前实现先使用进程内仓储锁定领域和 OpenAPI 契约；PostgreSQL、pgvector extension、真实 embedding 写入会在后续数据库任务中接入。
