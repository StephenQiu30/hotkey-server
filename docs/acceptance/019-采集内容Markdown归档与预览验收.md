---
layer: Acceptance
doc_no: "019"
audience: [Dev, QA, Ops]
feature_area: 内容归档与阅读
purpose: 验收授权 Feed 内容的 Markdown 归档与安全读取 API
canonical_path: docs/acceptance/019-采集内容Markdown归档与预览验收.md
status: accepted
conclusion: accepted_with_risk
version: v1.0
owner: HotKey Server Team
inputs:
  - docs/design/016-采集内容Markdown归档与预览设计.md
  - docs/prd/019-采集内容Markdown归档与预览.md
  - docs/plans/019-采集内容Markdown归档与预览计划.md
outputs:
  - PLAN-019 长期验收结论
triggers:
  - PLAN-019 开始实施
downstream:
  - docs/operations/README.md
---

# 采集内容 Markdown 归档与预览验收

## 实施前标准审核

| 项目 | 状态 | 证据 |
|---|---|---|
| 授权 Feed body 与历史无 body 边界 | passed | 独立 Plan Review APPROVED |
| Markdown 转换、网络非范围与安全渲染 | passed | 独立 Plan Review APPROVED |
| ready/not_captured、存储故障与完整性 | passed | 独立 Plan Review APPROVED |
| Result、稳定错误码与注解生成 OpenAPI | passed | 独立 Plan Review APPROVED |
| 并发、幂等、补偿、删除、对账与恢复 | passed | 独立 Plan Review APPROVED |

## RED 证据

| 提交 | 命令/失败信号 | 状态 |
|---|---|---|
| `043cbd4` | `go run ./test/runner test ./internal/modules/source/infrastructure/rss ./internal/modules/ingestion/infrastructure/markdown -run 'Test(ParseFeedPrefersContent\|Converter)' -count=1`；RSS2/RDF/Atom 均错误选择 description/summary，converter 包报 `undefined: NewConverter` | passed |
| `f8f7e0f` | `go run ./test/runner test ./internal/modules/ingestion/application ./internal/modules/ingestion/transport/http ./internal/modules/ingestion/infrastructure/minio -run 'Test(ArchiveMarkdown\|ContentDocument\|EvidenceStoreRead)' -count=1`；document model/use case/route、MIME 和受限读取均尚不存在而编译失败 | passed |

## GREEN 证据

| 类型 | 命令/证据 | 状态 |
|---|---|---|
| Feed 与转换 | `go run ./test/runner test ./internal/modules/source/infrastructure/rss ./internal/modules/ingestion/infrastructure/markdown -run 'Test(ParseFeedPrefersContent\|Converter)' -count=1` | passed |
| Server 定向 | `HOTKEY_TEST_DSN=<disposable-admin-dsn> go run ./test/runner test ./internal/modules/source/infrastructure/rss ./internal/modules/ingestion/... -count=1` | passed |
| Task 4 精确生命周期门禁 | `HOTKEY_TEST_DSN=<disposable-admin-dsn> go run ./test/runner test -race ./internal/modules/ingestion/application ./internal/modules/ingestion/infrastructure/minio -run 'Test(ContentDocumentConcurrentReads\|ArchiveMarkdownReplay\|ArchiveMarkdownCompensation\|ArchiveMarkdownOrphanReconciliation\|ContentDocumentStoreRecovery\|ContentDocumentDeleted\|ContentDocumentDeletePending\|ContentDocumentAssetSelection)' -count=1`；application 八项精确测试真实执行且通过，MinIO 包无匹配测试，`-race` 无数据竞争 | passed |
| Source 授权跨模块链路 | `HOTKEY_TEST_DSN=<disposable-admin-dsn> go run ./test/runner test -race ./internal/modules/ingestion/application -run 'TestSourceCapturePolicyBodyStorageFlowsToIngestionAsset' -count=1`；真实 `CollectionService` 按 Source config 生成并持久化 CapturedItem，`allow_body_storage=false/true` 分别断言 0/1 个 Markdown asset | passed |
| document HTTP 404 Result | `go run ./test/runner test ./internal/modules/ingestion/transport/http -run 'TestContentDocumentRouteAllowsAuthenticatedRolesAndReturnsSafeProjection' -count=1`；400/404/503 均断言统一 Result 错误码 | passed |
| Server 全量 | `HOTKEY_TEST_DSN=<disposable-admin-dsn> HOTKEY_TEST_REDIS_URL=<isolated-test-redis> make ci`，随后 `make clean`；OpenAPI、vet、数据库运行校验、全量测试、build、架构、仓库与 Schema 均通过 | passed |
| OpenAPI | `make openapi-validate` 通过；`GET /api/v1/contents/{id}/document` 由注解生成并进入 80 路由契约 | passed |
| Schema | `git diff --exit-code -- db/schema.sql`，本期无 Schema 差异 | passed |
| 独立复审 | 非主要实施者复核 8 项精确生命周期测试、Source CapturePolicy 跨模块链路、400/404/503 Result、Schema 零差异与工作区清洁，无 P0/P1/P2 | passed |

## 数据与外部依赖限制

当前开发数据中来源均未允许 body 归档，旧 Content 无可恢复正文。可丢弃 PostgreSQL fixture 已证明 `allow_body_storage=false` 不创建 asset，允许 body 的 fixture 才生成精确 Markdown MIME；历史 `text/plain` 通过 application 契约证明只能返回 not_captured。S3 协议 fixture 已覆盖 Content-Type、大小、SHA、篡改与读取上限；因未提供 `HOTKEY_TEST_MINIO_*`，真实外部 MinIO integration 本轮未执行，保留为 blocked 外部证据，不能伪记通过。

## 结论

`accepted_with_risk`：Server 范围内实现、OpenAPI、Schema、全量质量门禁与独立复审均通过，提交链为 `043cbd4`、`d2df192`、`f8f7e0f`、`54a7610`、`6d6da6f`、`2129615`、`f748ea3`、`eb0f604`。唯一保留风险是本轮没有可用的 `HOTKEY_TEST_MINIO_*`，因此真实外部 MinIO integration 未执行；S3 协议 fixture 已覆盖 MIME、大小、SHA、篡改与读取上限。Web 阅读器与浏览器打印由 `../hotkey-web` 自身验收，不作为 Server 结案阻塞项。

## 变更记录

| 版本 | 日期 | 变更 |
|---|---|---|
| v0.1 | 2026-07-18 | 创建实施前验收模板，结论 pending。 |
| v0.2 | 2026-07-18 | 分离 Web 页面验收，补齐 Server 生命周期、错误和 Schema 门禁。 |
| v0.3 | 2026-07-18 | 记录实施前标准经非主要编写者审核通过；实现证据仍 pending。 |
| v0.4 | 2026-07-18 | 记录真实 RED、定向与全量 GREEN、OpenAPI/Schema 门禁，以及真实 MinIO 未执行风险；等待独立复审。 |
| v0.5 | 2026-07-18 | 补齐 Plan Task 4 八项精确命名并发/重放/补偿/对账/恢复/删除/选择门禁，以及 Source CapturePolicy 跨模块持久化链路和 document 404 Result；结论继续 pending。 |
| v1.0 | 2026-07-18 | 独立实现复审 APPROVED；Server 验收为 accepted_with_risk，仅保留真实外部 MinIO integration 未执行风险。 |
