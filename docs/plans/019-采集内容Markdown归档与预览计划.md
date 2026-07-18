---
layer: Plan
doc_no: "019"
audience: [Dev, QA, Ops]
feature_area: 内容归档与阅读
purpose: 以测试优先方式实施 Markdown 归档与安全 document API
canonical_path: docs/plans/019-采集内容Markdown归档与预览计划.md
status: accepted
execution_status: ready
review_status: approved
version: v1.0
owner: HotKey Server Team
inputs:
  - docs/design/016-采集内容Markdown归档与预览设计.md
  - docs/prd/019-采集内容Markdown归档与预览.md
outputs:
  - Server Markdown 投影和 document API
triggers:
  - PRD-019 accepted 且 ready
downstream:
  - docs/acceptance/019-采集内容Markdown归档与预览验收.md
depends_on: [PLAN-006, PLAN-007, PLAN-017]
---

# 采集内容 Markdown 归档与预览执行计划

## 1. 开工门禁

Design-016 与 PRD-019 必须 accepted，本 Plan 必须 accepted/approved/ready，并由非主要编写者逐项审核范围、文件、错误、安全、空态和验收。

## 2. 文件边界

创建：

- `test/_suite/internal/modules/ingestion/application/document_test.go`
- `internal/modules/ingestion/infrastructure/markdown/converter.go`
- `test/_suite/internal/modules/ingestion/infrastructure/markdown/converter_test.go`

修改：

- `go.mod`、`go.sum`
- `internal/modules/source/infrastructure/rss/feed.go`
- `internal/modules/ingestion/domain/content.go`、`ports.go`
- `internal/modules/ingestion/application/normalizer.go`、`service.go`、`query.go`
- `internal/modules/ingestion/infrastructure/minio/store.go`
- `internal/modules/ingestion/transport/http/dto.go`、`handler.go`、`routes.go`
- `internal/bootstrap/app.go`
- `test/_suite/internal/modules/source/infrastructure/rss/feed_test.go`
- `test/_suite/internal/modules/ingestion/application/normalizer_test.go`、`query_test.go`
- `test/_suite/internal/modules/ingestion/transport/http/handler_test.go`
- `test/_suite/internal/modules/ingestion/infrastructure/minio/store_test.go`
- `test/_suite/internal/modules/ingestion/infrastructure/minio/store_integration_test.go`
- `test/_suite/internal/modules/ingestion/infrastructure/postgres/repository_integration_test.go`
- `docs/openapi/docs.go`、`docs/openapi/swagger.json`
- 四层文档索引与本文档链

`db/schema.sql` 不修改；如实施发现必须增表或扩展 asset type，立即停工并回到 Design Review。

## 3. 测试优先步骤

### Task 1：冻结 Feed 与 Markdown 转换边界

- RED 命令：`go run ./test/runner test ./internal/modules/source/infrastructure/rss ./internal/modules/ingestion/infrastructure/markdown -run 'Test(ParseFeedPrefersContent|Converter)' -count=1`；预期失败为 RSS2/RDF/Atom content 未优先或 converter 包尚不存在。
- GREEN：引入固定 v2.5.2；适配器移除 script/iframe/form/远程图片，保留 CJK、表格、列表、代码和安全链接，不发起网络请求。
- GREEN 命令：重跑上述精确命令；通过信号为相关包 `ok`，且 RSS2/RDF/Atom fixture、GFM table、允许协议和危险节点用例全部通过。
- 提交：`test:` 后 `impl:`，不夹带 API/UI。

### Task 2：归档写入与生命周期

- RED 命令：`go run ./test/runner test ./internal/modules/ingestion/application ./internal/modules/ingestion/infrastructure/postgres -run 'Test(ArchiveMarkdown|MarkdownAsset|BodyStoragePolicy)' -count=1`；预期失败为 Markdown 未写入、MIME 仍为 text/plain、重放产生重复 asset，或未授权 body 被归档。
- GREEN：转换成功后以 `asset_type=text`/`text/markdown; charset=utf-8` 写入确定性对象；同一 SHA 重放幂等，多版本按 `captured_at DESC, id DESC` 选最新可用 Markdown asset。对象写成但 DB 失败必须补偿，delete_pending/deleted 不可读，孤儿继续由既有对账清理。
- GREEN 命令：重跑上述精确命令；通过信号为相关包 `ok`，且 `allow_body_storage=false` 不写对象/asset、`true` 才归档、重放只保留一个 asset、事务失败补偿用例全部通过。

### Task 3：document 读取与 HTTP 契约

- RED 命令：`go run ./test/runner test ./internal/modules/ingestion/application ./internal/modules/ingestion/transport/http ./internal/modules/ingestion/infrastructure/minio -run 'Test(ContentDocument|EvidenceStoreRead)' -count=1`；预期失败为 document 方法/路由和受限读取契约尚不存在。
- GREEN：EvidenceStore 只读 Repository 选出的 available Markdown asset，限制最大字节，同时校验 MinIO 元数据、DB asset 和读回内容的 SHA/大小/Content-Type。viewer/editor/admin 都可读；400/404/503 按 Design-016 映射；选择测试覆盖 available + text + 精确 Markdown MIME + `captured_at DESC, id DESC`，历史 text/plain 只能得到 not_captured。
- GREEN 命令：重跑上述精确命令；通过信号为相关包 `ok`，且 ready/not_captured、选择顺序、历史 MIME 隔离、三角色权限、大小和完整性错误映射全部通过。
- 运行 `make openapi`，通过 OpenAPI 契约和差异检查。

### Task 4：生命周期与恢复回归

- RED 命令：`go run ./test/runner test -race ./internal/modules/ingestion/application ./internal/modules/ingestion/infrastructure/minio -run 'Test(ContentDocumentConcurrentReads|ArchiveMarkdownReplay|ArchiveMarkdownCompensation|ArchiveMarkdownOrphanReconciliation|ContentDocumentStoreRecovery|ContentDocumentDeleted|ContentDocumentDeletePending|ContentDocumentAssetSelection)' -count=1`；预期失败为新契约/测试不存在。
- GREEN：并发读一致，重放无重复 asset，DB 失败后无引用对象被补偿，删除或 delete_pending 不可读，MinIO 从不可用恢复后同一 asset 恢复 ready。
- GREEN 命令：重跑上述精确 `-race` 命令；通过信号为两个包 `ok`、无 race，且孤儿对象只由既有对账删除、available Markdown 选择和恢复用例全部通过。

### Task 5：全量门禁与独立验收

- Server：`test -n "$HOTKEY_TEST_DSN" && make ci && make clean`；环境缺失时显式失败，不伪造数据库证据。
- 真实 MinIO：`test -n "$HOTKEY_TEST_MINIO_ENDPOINT" && test -n "$HOTKEY_TEST_MINIO_ACCESS_KEY" && test -n "$HOTKEY_TEST_MINIO_SECRET_KEY" && test -n "$HOTKEY_TEST_MINIO_BUCKET" && go run ./test/runner test -tags=integration ./internal/modules/ingestion/infrastructure/minio -run 'TestStoreIntegration' -count=1`；通过信号为 integration 包 `ok`。条件缺失时标记 blocked 风险，不伪造通过。
- 非主要实施者对每项验收标记 passed/failed/blocked，只有无失败项才结案。

## 4. 回滚与残余风险

每个任务独立提交，本 Plan 不提交或回滚 Web 文件。回滚 document API 前要求下游 Web 先回滚其调用；回滚 Markdown 产生前确认已有 `text/markdown` asset 仍可按 text 证据完成删除/孤儿对账，不得改回历史读取兼容分支。旧数据无 body 与外部 MinIO 未配置必须如实记录。

## 5. 变更记录

| 版本 | 日期 | 变更 |
|---|---|---|
| v0.1 | 2026-07-18 | 建立文件级、红绿灯和独立复审计划，等待审核。 |
| v0.2 | 2026-07-18 | 按 Plan Review 修正跨仓相对路径，补齐可执行红灯、错误码、来源开关以及并发/幂等/删除/恢复验收。 |
| v0.3 | 2026-07-18 | 拆分跨仓范围，冻结 Markdown asset 选择规则、适配器文件、读写完整性与全生命周期验收。 |
| v0.4 | 2026-07-18 | 补齐逐 Task GREEN、授权策略、资产选择、孤儿对账和真实 MinIO integration 门禁。 |
| v0.5 | 2026-07-18 | 修正 runner 定向命令为 materialize 后的生产包路径。 |
| v1.0 | 2026-07-18 | 七项 Plan Review 门禁经非主要编写者复核通过，进入 ready。 |
