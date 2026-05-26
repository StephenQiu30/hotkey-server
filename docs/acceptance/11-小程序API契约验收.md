---
layer: Acceptance
doc_no: "11"
audience:
  - PM
  - Tech-Lead
  - Dev
  - QA
feature_area: "area:api"
purpose: "记录小程序 API 契约任务的实现边界、验证命令和验收结论。"
canonical_path: "docs/acceptance/11-小程序API契约验收.md"
status: accepted
version: "1.0.0"
owner: "StephenQiu30"
inputs:
  - docs/product/prd/11-小程序API契约PRD.md
  - docs/plans/11-小程序API契约实现计划.md
outputs:
  - 小程序 API 契约验收证据
---

# 11-小程序API契约验收

## 1. 实现范围

- OpenAPI 明确小程序端所需热点、事件证据、关键词、日报和刷新接口。
- OpenAPI 可 JSON 导出，可作为小程序客户端生成事实源。
- OpenAPI 增加 `BearerAuth` 鉴权方案。
- 统一错误响应补充 `400` 和 `401` 响应结构。

## 2. 小程序关键接口

- `GET /api/v1/hotspots`
- `GET /api/v1/hotspots/{id}`
- `GET /api/v1/events/{id}/evidence`
- `GET /api/v1/keywords/preferences`
- `GET /api/v1/reports/daily`
- `GET /api/v1/users/{id}/reports/daily`
- `POST /api/v1/refresh-queue`

## 3. 验收命令

```bash
go test ./...
git diff --check
find docs/product/prd docs/plans -maxdepth 1 -type f | sort -V
```

## 4. 验收结论

- 小程序所需接口字段已进入 OpenAPI。
- OpenAPI 可序列化为合法 JSON，支持客户端生成。
- 错误结构和鉴权要求已在 OpenAPI 中明确。
