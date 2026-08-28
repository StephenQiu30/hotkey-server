---
layer: Acceptance
scope: shared
doc_no: "001"
title: HotKey 产品需求分析与总体架构验收
status: failed
version: v1.0
owner: HotKey Team
canonical_path: docs/acceptance/001-HotKey产品需求分析与总体架构验收.md
design: docs/design/001-HotKey产品需求分析与总体架构设计.md
prd: docs/prd/001-HotKey产品需求分析与总体架构.md
plan: docs/plans/001-HotKey产品需求分析与总体架构计划.md
verified_revision: "f30de87dc2d252e77f38ab6ae01f01968ab119ff"
verification_date: "2026-08-29"
---

# HotKey 产品需求分析与总体架构验收

## 结论

本轮结论为 `failed`，含义是 001 整体尚未通过，而不是本轮自动化失败。当前已保存 P0 主故事现状、架构真实性、单一契约、降级边界、非向量检索/人工知识保护、日报列表游标和微事件 Evidence 列表游标七组可复核证据；完整四角色 UAT、人工键盘矩阵、PostgreSQL/MinIO/Vault/River 联合恢复、全范围关键写入矩阵和全范围游标矩阵仍未完成，因此 Plan 保持 `in_progress`，PRD 保持 `approved`。

本文件只关闭证据完整的局部门禁，不将浏览器 Fixture、自动化测试或候选性能结果扩大解释为完整 P0 发布验收。

## 验证基线

| 项目 | 实际值 |
|---|---|
| 验证日期 | 2026-08-29（Asia/Shanghai） |
| 代码基线 | `f30de87dc2d252e77f38ab6ae01f01968ab119ff` |
| 本机环境 | macOS 26.6.2、Apple arm64、Go 1.26.5、Node.js 24.19.0、uv 0.11.32 |
| 项目运行 | 根 `docker-compose.yml`；Go Core、Python Agent、Web、PostgreSQL、Redis、MinIO 共 6 个服务均为 `healthy`；API `/healthz` 与 Web 首页均返回 HTTP 200 |
| 自动化环境 | 本机既有工具链与可丢弃测试库；GitHub Actions Ubuntu Runner 与隔离 PostgreSQL、Redis、MinIO、Fresh Compose Project |
| 远端证据 | [GitHub Actions 33199596739](https://github.com/StephenQiu30/hotkey-server/actions/runs/33199596739)：Backend static/test/vulnerability、Worker recovery、Frontend、Python Agent、Compose、Fresh-container browser 与最终 `All acceptance gates` 全部 `success` |

## P0 主故事现状矩阵

`EV-001-001` 保存的是当前实现和缺口，不声明 `AC-001-001` 已整体通过。

| 环节 | 当前代码/事实源 | API 与页面 | 可复核测试证据 | 当前结论 |
|---|---|---|---|---|
| 登录与身份 | `backend/internal/modules/identity/`、`users`、`auth_sessions` | `/api/v1/auth/login`、`/api/v1/auth/me`、Dashboard Shell | `TestFourRoleSessionLifecycleUsesCurrentRoleAndNeverResurrectsRevokedSessions`；Fresh-container 真实登录 | 已实现；完整四角色 UAT 未执行 |
| Monitor | `backend/internal/modules/monitor/`、`monitors` 与不可变配置版本 | `/api/v1/monitors`、`/dashboard/settings` | `make monitor-publication-acceptance` 及所有权负向测试 | 已实现；浏览器主故事当前使用固定 Monitor Fixture |
| 来源、采集与原始证据 | `backend/internal/modules/source/`、MinIO 对象、采集事实与 River Job | `/api/v1/sources`、`/api/v1/monitors/{id}/collect`、`/dashboard/sources` | `TestRSSHNPipelineRecovery`、四点 Worker Recovery、对象写入失败零条目事实测试 | 已实现自动化链；本轮未执行真实 RSS/HN/X 授权冒烟 |
| 内容、事件与研判 | `backend/internal/modules/ingestion/`、`event/`、`intelligence/` 与根 `agent/` | `/api/v1/contents`、事件 API、`/dashboard/contents`、`/dashboard/events` | Agent 契约/降级、Evidence 白名单、事件语义与治理门禁 | 已实现自动化链；真实模型质量批准另由 003 管理 |
| 通知 | `notification_outbox_events`、`user_notifications`、`notification_read_receipts` | `/api/v1/notifications`、WebSocket、`/dashboard/notifications` | `make notification-convergence-acceptance`；浏览器 Fixture 精确产生 2 条报告通知 | 当前事实、重放和跨设备收敛已验证 |
| 日报与逐句 Evidence | `reports`、冻结 Revision、逐句 Citation 关系 | `/api/v1/reports`、`/dashboard/reports` | `make report-publication-acceptance`；`TestReportPublicationRevalidatesCitationFactsInsteadOfAggregateEvidenceState` | 每个事实句必须绑定允许 Evidence 的入口已验证；浏览器 Fixture 精确发布 1 份报告 |
| Vault 知识 | PostgreSQL Knowledge 事实、Vault 自动/人工区域与受保护 Revision | `/api/v1/knowledge/*`、`/dashboard/knowledge` | `TestUpdateVaultDocumentPreservesHumanRegionByteForByte`、`TestRecoverVaultDocumentUsesOnlyProtectedHumanRegionSources` | 自动区域可重建且人工区域逐字保护已验证；浏览器 Fixture 精确应用 1 个知识投影 |
| 全文检索 | `document_version_search_indexes` 与三个拥有模块的 PostgreSQL 词法查询 | `/api/v1/search`、`/dashboard/search` | `TestP0LexicalRecallUsesOnlyAuditablePostgresFTS`、三个 PostgreSQL 词法集成测试 | P0 不调用向量/Embedding/RAG；浏览器 Fixture 精确检出 1 个知识结果 |

## 证据登记

### `EV-001-001`：现状与主故事保护入口

- 映射：`CHK-001-G0-001`、`AC-001-001`；
- 结果：通过“现状基线保存”，不代表 `AC-001-001` 整体通过；
- 证据：上表将代码、Schema 事实、OpenAPI 路径、页面和测试入口逐段关联；日报逐句 Evidence 由 `make report-publication-acceptance` 固化，Vault 人工区域由 `TestUpdateVaultDocumentPreservesHumanRegionByteForByte` 与 `TestRecoverVaultDocumentUsesOnlyProtectedHumanRegionSources` 固化；
- 边界：Fresh-container 浏览器故事从固定 Monitor/事实 Fixture 开始，只证明通知→报告→Vault→搜索链与数据库计数，不替代真实来源从 Monitor 创建到采集的完整 UAT。

### `EV-001-002`：架构与文档状态真实性

- 映射：`CHK-001-G1-002`、`AC-001-003`；
- 结果：通过；
- 证据：`TestP0RuntimeRejectsForbiddenDistributedInfrastructure` 证明当前启动图未引入禁用基础设施；`TestForbiddenInfrastructureDetectorCatchesErroneousIntroductions` 用 Kafka、Temporal、第二 Python 后端和 `migrations/` 反例证明门禁能失败；`TestPythonAgentIsTheOnlyApprovedAnalysisService` 证明根 `agent/` 无宿主端口、业务存储依赖和业务凭据；`TestDocumentationLifecycleStatusesStayConsistent` 证明 Design/PRD/Plan 状态与索引一致，且 `planned` Plan 不能冒充已完成 G4–G6；
- 当前/目标/验收分离：代码、唯一 Schema 与生成 OpenAPI 记录当前事实；accepted Design、approved PRD 和 in-progress Plan 记录目标/执行状态；本 Acceptance 以 `failed` 保留未完成项。

### `EV-001-003`：Result、Schema、OpenAPI 与生成客户端单一事实源

- 映射：`CHK-001-G3-001`、`AC-001-005`；
- 结果：通过；
- 证据：`make ci-static` 重新生成并比较 `backend/openapi/docs.go` 与 `docs/openapi/swagger.json`，执行统一 Result/Transport、唯一 Schema、仓库与架构检查；`make ci-runtime` 在可丢弃 PostgreSQL/Redis 上执行数据库运行时、Schema 空库/幂等/兼容检查和后端全量串行测试；前端 `npm run openapi:check` 证明生成客户端无漂移；
- 结果摘要：上述本机命令通过；远端运行 `33199596739` 的 Backend static、Backend test 与 Frontend 三个独立 Job 均为 `success`。

### `EV-001-004`：降级与 Agent 建议边界

- 映射：`CHK-001-G4-001`、`AC-001-007`；
- 结果：通过；
- 命令：`make agent-degradation-acceptance`、`make agent-skill-contract-acceptance`、`make m4-fault-recovery-acceptance`；
- 事实计数/状态断言：三来源 Fixture 为 2 个成功、1 个限流失败、1 个保留并继续下游的候选，聚合状态为 `partial_success`；MinIO 写失败时条目事实为 0；Agent 的 5 类运行失败均保持 `degraded/pending_analysis` 且 Claim 写入为 0；4 类持久运行失败均只保存失败 Run 和空结构结果；伪造 Evidence 经 2 次有界尝试后不进入结果，后续有效修复创建第 2 个独立 Run；Vault 不可写时保留 PostgreSQL Artifact 身份，修复后复用同一 Artifact 完成；
- 人工边界：可解析但越权建议在 Go Application/Domain 的 Schema、Evidence 白名单、权限和状态机校验前停止；Python Agent 没有 PostgreSQL/Redis/MinIO/Vault/来源/用户凭据，不能直接写业务事实、权限或最终状态。

### `EV-001-005`：词法检索与人工知识保护

- 映射：`CHK-001-G4-002`、`AC-001-008`；
- 结果：通过；
- 搜索证据：`TestP0LexicalRecallUsesOnlyAuditablePostgresFTS` 与 `TestP0FTSProjectionAndSchemaRemainRebuildableAndIndexed` 检查 P0 查询和投影只使用 PostgreSQL FTS/`pg_trgm`/权限重检，禁止向量表、距离运算、Provider 和 RAG；拥有模块 PostgreSQL 集成测试与浏览器 `/api/v1/search` 验证可见结果；
- 人工区域证据：`TestUpdateVaultDocumentPreservesHumanRegionByteForByte`、`TestRecoverVaultDocumentUsesOnlyProtectedHumanRegionSources`、`TestVaultRecoveryRestoresMissingFileFromRevisionSnapshot` 和冲突停止测试证明自动区域可重建，人工区域只能从当前 Vault、受保护 Knowledge Revision 或批准备份恢复；
- 迁移边界：`TestExistingEmbeddingPathRemainsAnExplicitMigrationInventory` 保留旧 Provider/Embedding/pgvector 为受控迁移清单，未在缺少替换、灰度和回滚证据时删除。

### `EV-001-006`：日报列表签名游标与浏览器门禁稳定性

- 映射：`CHK-001-G3-004`、`AC-001-010`；
- 结果：日报列表子项通过，`CHK-001-G3-004` 仍不关闭；
- 代码证据：提交 `654ac3ac106e5dd1cd77a29878dff0da1414c1cc` 将日报列表游标改为短期有效的签名不透明字符串，并绑定 `type`、`status` 筛选；篡改、过期或跨筛选复用统一拒绝为无效输入，OpenAPI 与生成客户端同步从整数契约改为字符串契约；
- 测试证据：`TestReportListCursorIsSignedBoundExpiringAndStableAcrossConcurrentInsert` 在 PostgreSQL 上验证非数字签名、篡改拒绝、筛选绑定、过期拒绝，以及并发新增期间连续遍历无重复、无越权泄漏且不遗漏原快照结果；HTTP 契约测试证明游标按不透明字符串透传；
- CI 证据：提交 `3dcf6717c5d1e3f64f20d4276c99111c244d69a4` 为浏览器 Axe 扫描增加视觉状态稳定等待，`TestBrowserCIA11yAuditsWaitForVisualStateToSettle` 固化该门禁；远端运行 `33199596739` 的 Fresh-container browser、Backend test 和最终聚合门禁均为 `success`；
- 边界：本证据只关闭日报列表子项，不能替代其他 P0 列表的同排序值、并发新增、连续遍历、越权、篡改与过期统一矩阵。

### `EV-001-007`：微事件 Evidence 列表签名快照游标

- 映射：`CHK-001-G3-004`、`AC-001-010`；
- 结果：微事件 Evidence 列表子项通过，`CHK-001-G3-004` 仍不关闭；
- 代码证据：提交 `f30de87dc2d252e77f38ab6ae01f01968ab119ff` 将生产路由 `/api/v1/micro-events/{id}/evidence` 从裸整数游标改为短期有效的签名不透明快照游标，并绑定 `micro_event_id`；篡改、过期或跨事件复用统一拒绝为无效输入，OpenAPI 与生成客户端同步改为 `cursor`/`next_cursor` 字符串契约；
- 测试证据：`TestMicroEventEvidenceCursorIsSignedBoundExpiringAndSnapshotStable` 在 PostgreSQL 上验证非数字签名、篡改拒绝、事件绑定、过期拒绝，以及并发新增证据不污染已开始的快照遍历；`TestMicroEventEvidenceForwardsOpaqueCursor` 验证 HTTP 层只透传不透明游标，架构契约固定 `micro_event_evidence_list` 必须调用统一 Codec 的 `Seal/Open`；
- CI 证据：远端运行 `33199596739` 的 Backend static、Backend test、Frontend 和最终聚合门禁均为 `success`，其中 Backend test 为 9 分 40 秒；
- 边界：本证据只关闭微事件 Evidence 列表子项；其他 P0 列表仍须分别证明稳定排序、资源/筛选绑定、连续遍历、越权、篡改和过期行为。

## AC 结果

| AC | 结果 | 说明 |
|---|---|---|
| `AC-001-001` | partial | 已保存完整现状矩阵和报告/Vault 硬断言入口；真实来源驱动的完整 P0 浏览器/UAT 故事未完成 |
| `AC-001-002` | partial | 自动化四角色、所有权和会话撤销已覆盖；完整四角色 UAT 未完成 |
| `AC-001-003` | passed | 见 `EV-001-002` |
| `AC-001-004` | partial | 五态、移动端、自动 WCAG 与 Tab 焦点证据存在；完整人工键盘矩阵未完成 |
| `AC-001-005` | passed | 见 `EV-001-003` |
| `AC-001-006` | failed | 未执行同一隔离副本的 PostgreSQL/MinIO/Vault/River 联合恢复与真实 RPO/RTO 测量 |
| `AC-001-007` | passed | 见 `EV-001-004` |
| `AC-001-008` | passed | 见 `EV-001-005` |
| `AC-001-009` | partial | 多个关键写入已有授权、幂等、版本、冲突和审计测试，但尚无全 P0 写入口统一矩阵 |
| `AC-001-010` | partial | 日报与微事件 Evidence 列表已覆盖签名、过期、筛选/资源绑定、篡改拒绝和并发新增期间快照遍历；尚无全部 P0 列表的统一矩阵 |

## 实际命令与结果摘要

```text
backend: make ci
frontend: npm run openapi:check; npm run typecheck; npm run test:unit; npm audit --omit=dev --audit-level=high; npm run build
agent: uv run ruff format --check .; uv run ruff check .; uv run mypy src; uv run pytest; uv run pip-audit
repository: docker compose -f docker-compose.yml config --quiet; docker compose --env-file .env.prod -f docker-compose-prod.yml config --quiet; git diff --check
specialized: make agent-degradation-acceptance; make agent-skill-contract-acceptance; make report-publication-acceptance; make m4-fault-recovery-acceptance; go run ./test/runner test -tags=integration -p=1 ./internal/modules/event/infrastructure/postgres -run TestMicroEventEvidenceCursorIsSignedBoundExpiringAndSnapshotStable -count=1; go run ./test/runner test ./test/architecture -count=1
```

本机全量结果通过；Python Agent 为 41 项测试通过且覆盖率 97.57%，生产依赖审计无高危漏洞；Go `govulncheck` 未发现当前调用链漏洞。远端运行 `33199596739` 的 8 个执行 Job 和最终聚合门禁全部通过，其中 Backend test 为 9 分 40 秒，Fresh-container browser 为 3 分 12 秒。记录期间并行出现但未纳入本验收提交的前端工作区改动不计入该基线。

## 未完成项与停止条件

- `CHK-001-G1-001`：等待完整四角色 UAT；
- `CHK-001-G2-001`：等待完整人工键盘/焦点矩阵，不以自动 Tab 与 WCAG 扫描替代；
- `CHK-001-G3-002`：等待同一隔离恢复副本上的联合恢复、对账和真实 RPO/RTO；
- `CHK-001-G3-003`：等待所有 P0 关键写入口的统一副作用、事实计数和追加审计矩阵；
- `CHK-001-G3-004`：日报与微事件 Evidence 列表子项已完成；等待其余 P0 列表的并列排序、并发新增、连续遍历和越权/篡改/过期游标矩阵。

以上任一项缺失时，本 Acceptance 不得改为 `passed`，001 Plan/PRD 不得改为 `completed/implemented`。

## 影响与回滚验证

- Schema、运行配置和部署拓扑：无变更；OpenAPI 的日报与微事件 Evidence `cursor`/`next_cursor` 使用不透明字符串；
- 运行影响：日报列表拒绝篡改、过期或跨筛选复用的游标，微事件 Evidence 列表拒绝篡改、过期或跨事件复用的游标；项目继续由根 Compose 运行，测试继续使用本机锁定工具链和可丢弃测试服务；
- 回滚：若证据引用失效，回退本文件对应 EV 和 Plan 勾选即可，不删除业务事实、对象、Vault 内容或任务历史；架构契约会在引用的测试、命令或门禁状态漂移时失败；
- 已知限制：本文件固定的代码基线早于记录本文件的提交；记录提交由同一 CI 再验证，但不以无法实现的“提交自引用哈希”冒充证据。

## 验收人

- Owner：HotKey Team；
- 自动化执行：Codex；
- 整体结论：failed（部分门禁通过，001 尚未完成）。
