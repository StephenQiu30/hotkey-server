---
layer: Plan
doc_no: "08"
audience:
  - Tech-Lead
  - Dev
  - QA
feature_area: "area:hotspot"
purpose: "实现热点评分、榜单 API、详情 API 和评分解释。"
canonical_path: "docs/plans/08-热点评分与榜单API实现计划.md"
status: draft
version: "1.0.0"
owner: "StephenQiu30"
inputs:
  - "docs/product/prd/08-热点评分与榜单APIPRD.md"
outputs:
  - "热点评分与榜单API实现任务"
triggers:
  - "评分权重或榜单 API 变化"
downstream:
  - "由 WORKFLOW.md 指定的 Symphony / Linear 流程接管"
---

# 08-热点评分与榜单API实现计划

## 1. 目标

计算热点相关度、热度、新鲜度、可信度和综合分，并提供列表和详情 API。

## 2. 文件清单

- 创建：`migrations/000008_hotspot_scores.up.sql`
- 创建：`internal/service/hotspot/scoring.go`
- 创建：`internal/transport/http/handlers/hotspot/`
- 创建：`internal/worker/handlers/scoring/`
- 修改：`internal/transport/http/router.go`

## 3. 任务拆解

1. 创建 `hotspot_scores`。
2. 实现评分权重和 explanation JSON。
3. 实现 `score_hotspots` worker。
4. 实现热点列表 API 和详情 API。
5. 添加 contract test 和排序测试。

## 4. TDD 与验证

- 多来源热点分数高于单来源低质量热点。
- 热点列表按 total_score 排序。
- 详情返回 source refs 和 explanation。

## 5. 执行顺序

1. `test:` scoring 和 handler 失败测试。
2. `impl:` migration、service、worker、handler。
3. `docs:` 更新 OpenAPI。

## 6. 回滚策略

移除热点 API 路由，回滚 score migration。

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
