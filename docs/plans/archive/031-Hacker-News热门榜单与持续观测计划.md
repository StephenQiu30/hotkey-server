---
layer: Plan
scope: shared
doc_no: "031"
title: Hacker-News热门榜单与持续观测计划
status: completed
version: v1.1
owner: HotKey Team
phase: P0
canonical_path: docs/plans/archive/031-Hacker-News热门榜单与持续观测计划.md
design: docs/design/archive/031-Hacker-News热门榜单与持续观测设计.md
prd: docs/prd/archive/031-Hacker-News热门榜单与持续观测.md
---

# Hacker-News热门榜单与持续观测计划

## 前置门禁

- [x] Design 已接受，PRD 已批准。
- [x] 对照 HN 官方 API 确认 `topstories`、`beststories` 与 item 字段契约。
- [x] 建立真实运行基线：后端就绪，但现有来源任务全部失败且 Content/Event 均为空。

## 变更范围

- `backend/internal/modules/source/domain/connection.go`
- `backend/internal/modules/source/infrastructure/hackernews/connector.go`
- `backend/internal/modules/source/transport/http/dto.go`
- `backend/internal/modules/ingestion/infrastructure/jobs/pipeline.go`
- `backend/internal/modules/ingestion/application/relevance.go`
- `backend/db/schema.sql`
- `frontend/src/components/dashboard/SourceConnectionDialog.tsx`
- 同模块测试、OpenAPI 生成物与 031 文档

## 执行步骤

1. [x] 先增加领域枚举、榜单重复观测、顺序、部分失败和 UI 提交的失败测试。
2. [x] 实现 `hacker_news_mode` 的领域默认、白名单、数据库和 DTO/OpenAPI 契约。
3. [x] 实现 Top/Best 列表读取、有界并发、稳定顺序、重复观测和模式健康探测。
4. [x] 在来源创建 UI 增加模式选择，新建 HN 默认热门榜单。
5. [x] 修复标准化分页遗漏与未知语言/地区误拒绝，并补充回归测试。
6. [x] 运行后端模块/Schema/OpenAPI 与前端单测、类型、构建回归。
7. [x] 用真实 HN 数据创建来源并触发多轮采集，核对 Content、metric snapshot、监控匹配、Event 与 Notification；完成浏览器验收。
8. [x] 写入同编号 Acceptance，归档 Design/PRD/Plan 并更新索引。

## 回滚

停止或归档新增热门来源即可停止重复观测。应用回滚后历史配置中的额外 JSON 键不会删除内容或指标快照；若需要恢复旧数据库契约，应先把 HN 来源模式改回 `new`，再恢复旧 schema 函数。全程不删除已有 Content、Event、CollectionRun 或 checkpoint。
