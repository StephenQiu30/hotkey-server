---
layer: Acceptance
doc_no: "8"
audience:
  - PM
  - Tech-Lead
  - Dev
  - QA
feature_area: "area:ranking area:api"
purpose: "记录热点排序与详情 API 任务的实现边界、验证命令和验收结论。"
canonical_path: "docs/acceptance/8-热点排序与详情API验收.md"
status: accepted
version: "1.0.0"
owner: "StephenQiu30"
inputs:
  - docs/product/prd/8-热点排序与详情APIPRD.md
  - docs/plans/8-热点排序与详情API实现计划.md
outputs:
  - 热点排序与详情 API 验收证据
---

# 8-热点排序与详情API验收

## 1. 实现范围

- 新增热点服务，支持热点列表排序和详情展开。
- 列表支持关键词、地区、语言、最低可信度过滤。
- 列表支持 `heat`、`trust`、`relevance` 排序。
- 详情返回关联内容、证据摘要、相似度和风险标签。

## 2. API 影响

- `GET /api/v1/hotspots`
- `GET /api/v1/hotspots/{id}`
- `/openapi.json` 已包含热点列表与详情接口。

## 3. 验收命令

```bash
go test ./...
git diff --check
find docs/product/prd docs/plans -maxdepth 1 -type f | sort -V
```

## 4. 验收结论

- 支持关键词、地区、语言、可信度和热度排序。
- 详情返回关联内容、证据链摘要、相似度和风险标签。
- 当前实现先使用进程内仓储锁定领域和 OpenAPI 契约；后续数据库任务会接入真实热点计算和持久化读取。
