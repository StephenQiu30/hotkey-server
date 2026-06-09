---
layer: Plan
doc_no: "19"
audience:
  - Tech-Lead
  - Dev
  - QA
feature_area: "area:hotspot"
purpose: "让热点簇结果能被评分并通过稳定的列表与详情 API 对外提供。"
canonical_path: "docs/plans/19-热点评分与榜单API主线接通实现计划.md"
status: draft
version: "1.0.0"
owner: "StephenQiu30"
inputs:
  - "docs/design/002-热点平台能力补齐与上线接通设计.md"
  - "docs/plans/18-Embedding热点聚类管线接通实现计划.md"
  - "docs/product/prd/08-热点评分与榜单APIPRD.md"
  - "docs/plans/08-热点评分与榜单API实现计划.md"
outputs:
  - "热点评分与榜单 API 主线接通实现任务"
triggers:
  - "评分规则或热点 API 契约变化"
downstream:
  - "docs/plans/20-日报邮件输出管线实现计划.md"
---

# 19-热点评分与榜单API主线接通实现计划

## 1. 目标

让热点簇结果能被评分并通过稳定的列表与详情 API 对外提供。

## 2. 文件清单

- 修改：`internal/service/hotspot/`
- 修改：`internal/transport/http/handlers/hotspot/`
- 修改：`internal/repository/postgres/scorerepo/`
- 修改：`internal/transport/http/router.go`

## 3. 任务拆解

1. 为评分排序、详情字段和分页过滤写失败测试。
2. 将聚类结果接入评分落库。
3. 补齐热点列表和详情接口字段。
4. 更新必要的 OpenAPI 和 contract test。
5. 运行验证并回写结果。

## 4. TDD 与验证

- 热点读取接口稳定可用。
- 评分解释和来源证据可以被前端消费。
- API 行为由 handler 或 contract test 明确保护。

## 5. 执行顺序

1. `test:` 评分排序、详情字段、分页过滤失败测试。
2. `impl:` 评分落库、handler 与路由接线。
3. `docs:` 更新 OpenAPI 契约（如有字段变更）。

## 6. 回滚策略

回滚评分落库与 API 接线，保留聚类结果；前端仍可通过旧 mock 或降级响应访问。

## 7. 验收命令

```bash
go test ./internal/service/hotspot/...
go test ./internal/transport/http/...
gofmt -w cmd internal
go test ./...
python3 -m unittest discover -s tests
```

## 8. Symphony / Linear 要求

任务状态、标签和流转规则完全以本仓库 `WORKFLOW.md` 和本地 Symphony 实现为准。Plan 不定义额外状态、不发明额外标签、不覆盖 Symphony 的状态机。

Linear issue 只承载任务内容：PRD 路径、Plan 路径、任务范围、禁止范围、TDD 验收命令和回写要求。Symphony 负责监听 active states、创建 workspace、运行 Codex，并按 `WORKFLOW.md` prompt 驱动执行。

## 9. 变更记录

| 日期 | 作者 | 版本 | 变更说明 |
| --- | --- | --- | --- |
| 2026-06-09 | StephenQiu30 | 1.0.0 | 按文档规范重写，编号调整为 19 |
