---
layer: Plan
doc_no: "10"
audience:
  - Tech-Lead
  - Dev
  - QA
feature_area: "area:rss"
purpose: "实现公开频道 RSS 和用户私有 RSS。"
canonical_path: "docs/plans/10-RSS订阅实现计划.md"
status: draft
version: "1.0.0"
owner: "StephenQiu30"
inputs:
  - "docs/product/prd/10-RSS订阅PRD.md"
outputs:
  - "RSS订阅实现任务"
triggers:
  - "RSS 输出或 token 策略变化"
downstream:
  - "由 WORKFLOW.md 指定的 Symphony / Linear 流程接管"
---

# 10-RSS订阅实现计划

## 1. 目标

提供公开频道 RSS 和用户私有 RSS token 输出，内容来自 daily_reports。

## 2. 文件清单

- 创建：`migrations/000010_rss_feeds.up.sql`
- 创建：`internal/service/rss/`
- 创建：`internal/repository/postgres/rssrepo/`
- 创建：`internal/transport/http/handlers/rss/`
- 修改：`internal/transport/http/router.go`

## 3. 任务拆解

1. 创建 `rss_feeds`。
2. 实现频道 RSS XML。
3. 实现用户私有 RSS token 生成、重置和禁用。
4. 实现私有 RSS XML。
5. 添加 XML contract test。

## 4. TDD 与验证

- 频道日报存在时 RSS 返回合法 XML。
- token 有效时返回用户日报。
- token 重置后旧 token 失效。

## 5. 执行顺序

1. `test:` RSS XML 和 token 失败测试。
2. `impl:` migration、repo、service、handler。
3. `docs:` 更新 OpenAPI 或 RSS endpoint 文档。

## 6. 回滚策略

回滚 rss_feeds migration，移除 RSS routes。

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
