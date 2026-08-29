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
verified_revision: "f31f4f4525b5825119e201eda7206bf782853f89"
verification_date: "2026-08-29"
---

# HotKey 产品需求分析与总体架构验收

## 结论

本轮结论为 `failed`，含义是 001 整体尚未通过，而不是本轮自动化失败。当前已保存 P0 主故事现状、架构真实性、单一契约、降级边界、非向量检索/人工知识保护、日报列表游标、微事件 Evidence 列表游标、全文检索结果游标、运维任务列表游标、审计日志列表游标、内容/热点列表游标、Monitor 列表游标、来源连接列表游标、采集运行列表游标、采集条目工作列表游标、相关性匹配列表游标和微事件榜单游标十七组可复核证据；完整四角色 UAT、人工键盘矩阵、PostgreSQL/MinIO/Vault/River 联合恢复、全范围关键写入矩阵和全范围游标矩阵仍未完成，因此 Plan 保持 `in_progress`，PRD 保持 `approved`。

本文件只关闭证据完整的局部门禁，不将浏览器 Fixture、自动化测试或候选性能结果扩大解释为完整 P0 发布验收。

## 验证基线

| 项目 | 实际值 |
|---|---|
| 验证日期 | 2026-08-29（Asia/Shanghai） |
| 代码基线 | `f31f4f4525b5825119e201eda7206bf782853f89` |
| 本机环境 | macOS 26.6.2、Apple arm64、Go 1.26.5、Node.js 24.19.0、uv 0.11.32 |
| 项目运行 | 根 `docker-compose.yml`；Go Core、Python Agent、Web、PostgreSQL、Redis、MinIO 共 6 个服务均为 `healthy`；API `/readyz` 与 Web 首页均返回 HTTP 200 |
| 自动化环境 | 本机既有工具链与可丢弃测试库；GitHub Actions Ubuntu Runner 与隔离 PostgreSQL、Redis、MinIO、Fresh Compose Project |
| 远端证据 | [GitHub Actions 33226557991](https://github.com/StephenQiu30/hotkey-server/actions/runs/33226557991)：Backend static/test/vulnerability、Worker recovery、Frontend、Python Agent、Compose、Fresh-container browser 与最终 `All acceptance gates` 全部 `success` |

## P0 主故事现状矩阵

`EV-001-001` 保存的是当前实现和缺口，不声明 `AC-001-001` 已整体通过。

| 环节 | 当前代码/事实源 | API 与页面 | 可复核测试证据 | 当前结论 |
|---|---|---|---|---|
| 登录与身份 | `backend/internal/modules/identity/`、`users`、`auth_sessions` | `/api/v1/auth/login`、`/api/v1/auth/me`、Dashboard Shell | `TestFourRoleSessionLifecycleUsesCurrentRoleAndNeverResurrectsRevokedSessions`；Fresh-container 真实登录 | 已实现；完整四角色 UAT 未执行 |
| Monitor | `backend/internal/modules/monitor/`、`monitors` 与不可变配置版本 | `/api/v1/monitors`、`/dashboard/settings` | `make monitor-publication-acceptance`、`TestMonitorListCursorIsSignedBoundExpiringAndSnapshotStableAcrossConcurrentInsert` 及所有权负向测试 | 已实现；列表使用可见性绑定的短期签名高水位游标，并发新增不污染既有遍历；浏览器主故事当前使用固定 Monitor Fixture |
| 来源、采集与原始证据 | `backend/internal/modules/source/`、MinIO 对象、采集事实与 River Job | `/api/v1/sources`、`/api/v1/collection-runs`、`/api/v1/monitors/{id}/collect`、`/dashboard/sources` | `TestRSSHNPipelineRecovery`、四点 Worker Recovery、`TestCollectionRunListCursorIsSignedExpiringAndSnapshotStableAcrossConcurrentInsert`、`TestCapturedItemListCursorIsSignedBoundExpiringAndSnapshotStableAcrossConcurrentInsert`、对象写入失败零条目事实测试 | 已实现自动化链；来源连接、采集运行及 Source→Ingestion 采集条目工作列表使用短期签名高水位游标；本轮未执行真实 RSS/HN/X 授权冒烟 |
| 内容、事件与研判 | `backend/internal/modules/ingestion/`、`event/`、`intelligence/` 与根 `agent/` | `/api/v1/contents`、事件 API、`/dashboard/contents`、`/dashboard/events` | Agent 契约/降级、Evidence 白名单、事件语义与治理门禁 | 已实现自动化链；真实模型质量批准另由 003 管理 |
| 通知 | `notification_outbox_events`、`user_notifications`、`notification_read_receipts` | `/api/v1/notifications`、WebSocket、`/dashboard/notifications` | `make notification-convergence-acceptance`；浏览器 Fixture 精确产生 2 条报告通知 | 当前事实、重放和跨设备收敛已验证 |
| 日报与逐句 Evidence | `reports`、冻结 Revision、逐句 Citation 关系 | `/api/v1/reports`、`/dashboard/reports` | `make report-publication-acceptance`；`TestReportPublicationRevalidatesCitationFactsInsteadOfAggregateEvidenceState` | 每个事实句必须绑定允许 Evidence 的入口已验证；浏览器 Fixture 精确发布 1 份报告 |
| Vault 知识 | PostgreSQL Knowledge 事实、Vault 自动/人工区域与受保护 Revision | `/api/v1/knowledge/*`、`/dashboard/knowledge` | `TestUpdateVaultDocumentPreservesHumanRegionByteForByte`、`TestRecoverVaultDocumentUsesOnlyProtectedHumanRegionSources` | 自动区域可重建且人工区域逐字保护已验证；浏览器 Fixture 精确应用 1 个知识投影 |
| 全文检索 | `document_version_search_indexes` 与三个拥有模块的 PostgreSQL 词法查询 | `/api/v1/search`、`/dashboard/search` | `TestP0LexicalRecallUsesOnlyAuditablePostgresFTS`、三个 PostgreSQL 词法集成测试、`TestServiceCursorIsSignedBoundExpiringAndSnapshotStableAcrossOwners` | P0 不调用向量/Embedding/RAG；结果列表使用短期签名快照游标；浏览器 Fixture 精确检出 1 个知识结果 |
| 内容与热点列表 | `contents`、`content_metric_snapshots` 与 Ingestion Owner Repository | `/api/v1/contents`、`/api/v1/hotspots`、`/dashboard/contents` | `TestContentCursorIsSignedBoundExpiringAndSnapshotStableAcrossConcurrentChanges`、`TestContentListStatementUsesDatabaseOrderingForEveryHotspotSort` | 列表使用短期签名快照游标；热度指标变化不再改变已开始遍历的边界，并发新增不污染既有快照 |
| 相关性匹配列表 | `monitor_matches` 与 Ingestion Relevance Repository | `/api/v1/monitors/{id}/matches` | `TestRelevanceMatchListCursorKeepsFirstPageHighWaterAcrossConcurrentInsert`、`TestRelevanceCursorsRejectTamperingAndCrossQueryReuse` | 列表使用 Monitor/decision 绑定的短期签名高水位游标；首屏后的低分新增匹配不进入既有遍历 |
| 微事件榜单 | `micro_events`、版本化 Heat/Relevance/Evidence 快照与 Event Query Repository | `/api/v1/micro-events`、`/dashboard/events` | `TestMicroEventQueryRepositoryAppliesMultiDimensionalFiltersAndStableRelevanceCursor`、`TestMicroEventListCursorFreezesRankingAndRejectsChangedQuery` | `latest/heat/relevance` 榜单使用筛选、排序、时间和首屏最大事件 ID 绑定的短期签名游标；首屏后的低排名新增事件不进入既有遍历 |
| 运维任务 | `river_job` 安全元数据投影与 Operations Owner Repository | `/api/v1/operations/jobs`、`/dashboard/governance` | `TestJobRepositoryCursorIsSignedBoundExpiringAndSnapshotStable`、`TestJobListForwardsOpaqueSubjectBoundCursorAndReturnsNextCursor` | 管理员任务列表使用主体/筛选绑定的签名快照游标，连续遍历不跳过 `limit+1` 边界项 |
| 运维审计日志 | 追加写 `audit_logs` 与 Operations Governance Repository | `/api/v1/operations/audit-logs`、`/dashboard/governance` | `TestGovernanceAuditCursorIsSignedBoundExpiringAndStableAcrossConcurrentInsert`、`TestGovernanceAuditForwardsOpaqueSubjectBoundCursor` | 管理员审计列表使用主体/筛选绑定的短期签名游标；篡改、过期和跨上下文复用均受控拒绝 |

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
- 结果摘要：上述本机命令通过；远端运行 `33206329722` 的 Backend static、Backend test 与 Frontend 三个独立 Job 均为 `success`。

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
- CI 证据：提交 `3dcf6717c5d1e3f64f20d4276c99111c244d69a4` 为浏览器 Axe 扫描增加视觉状态稳定等待，`TestBrowserCIA11yAuditsWaitForVisualStateToSettle` 固化该门禁；远端运行 `33203034175` 的 Fresh-container browser、Backend test 和最终聚合门禁均为 `success`；
- 边界：本证据只关闭日报列表子项，不能替代其他 P0 列表的同排序值、并发新增、连续遍历、越权、篡改与过期统一矩阵。

### `EV-001-007`：微事件 Evidence 列表签名快照游标

- 映射：`CHK-001-G3-004`、`AC-001-010`；
- 结果：微事件 Evidence 列表子项通过，`CHK-001-G3-004` 仍不关闭；
- 代码证据：提交 `f30de87dc2d252e77f38ab6ae01f01968ab119ff` 将生产路由 `/api/v1/micro-events/{id}/evidence` 从裸整数游标改为短期有效的签名不透明快照游标，并绑定 `micro_event_id`；篡改、过期或跨事件复用统一拒绝为无效输入，OpenAPI 与生成客户端同步改为 `cursor`/`next_cursor` 字符串契约；
- 测试证据：`TestMicroEventEvidenceCursorIsSignedBoundExpiringAndSnapshotStable` 在 PostgreSQL 上验证非数字签名、篡改拒绝、事件绑定、过期拒绝，以及并发新增证据不污染已开始的快照遍历；`TestMicroEventEvidenceForwardsOpaqueCursor` 验证 HTTP 层只透传不透明游标，架构契约固定 `micro_event_evidence_list` 必须调用统一 Codec 的 `Seal/Open`；
- CI 证据：远端运行 `33203034175` 的 Backend static、Backend test、Frontend 和最终聚合门禁均为 `success`；
- 边界：本证据只关闭微事件 Evidence 列表子项；其他 P0 列表仍须分别证明稳定排序、资源/筛选绑定、连续遍历、越权、篡改和过期行为。

### `EV-001-008`：全文检索结果列表签名快照游标

- 映射：`CHK-001-G3-004`、`AC-001-010`；
- 结果：全文检索结果列表子项通过，`CHK-001-G3-004` 仍不关闭；
- 代码证据：提交 `ce8ba494b8493587103e18ed1b03daad6dd264ee` 为生产路由 `/api/v1/search` 增加短期有效的签名不透明快照游标，绑定当前用户、角色和完整筛选指纹；三个拥有模块按同一 `relevance/latest` 全局顺序执行 PostgreSQL keyset 查询，游标篡改、过期、跨用户、跨角色或跨筛选复用统一拒绝为无效输入，OpenAPI 与生成客户端同步增加 `cursor` 字符串契约；
- 测试证据：`TestServiceCursorIsSignedBoundExpiringAndSnapshotStableAcrossOwners` 验证跨 Content/Event/Knowledge 的连续遍历、筛选与主体绑定、篡改/过期拒绝及并发新增不污染快照；`TestSearchRouteForwardsOpaqueCursorAndReturnsNextCursor` 验证 HTTP 层只透传不透明游标；`TestMicroEventLexicalSearchUsesSnapshotKeysetOrdering` 验证实际 PostgreSQL 快照边界，三个 Owner 的既有词法集成测试验证查询回归；架构契约固定 `search_result_list` 必须调用统一 Codec 的 `Seal/Open`；
- CI 证据：远端运行 `33203034175` 的 8 个执行 Job 和最终聚合门禁全部为 `success`；
- 边界：本证据只关闭全文检索结果列表子项；其他 P0 列表仍须分别证明并列排序、资源/筛选绑定、连续遍历、越权、篡改和过期行为。

### `EV-001-009`：运维任务列表签名快照游标

- 映射：`CHK-001-G3-004`、`AC-001-010`；
- 结果：运维任务列表子项通过，`CHK-001-G3-004` 仍不关闭；
- 代码证据：提交 `f41e87d8df6d8893fddb644cc1e2bc636c521b6e` 将生产路由 `/api/v1/operations/jobs` 从裸整数游标改为短期有效的签名不透明快照游标，绑定当前管理员用户及 `kind/state` 筛选指纹，并以首次查询的 `river_job.id` 高水位冻结连续遍历范围；实现同时修复原 `limit+1` 额外行未返回却被越过造成的漏项，OpenAPI 与生成客户端同步改为 `cursor`/`next_cursor` 字符串契约；
- 测试证据：`TestJobRepositoryCursorIsSignedBoundExpiringAndSnapshotStable` 在 PostgreSQL 上验证三项固定快照以两页完整返回、并发新增不进入既有遍历、篡改/过期/跨用户/跨筛选游标受控拒绝；`TestJobListForwardsOpaqueSubjectBoundCursorAndReturnsNextCursor` 验证 HTTP 层绑定认证主体并只透传不透明游标；`TestP0UserListCursorsUseSignedExpiringCodec` 固定生产 Bootstrap 必须注入统一 Codec，且 `operations_job_list` 必须调用 `Seal/Open`；
- CI 证据：远端运行 `33206329722` 的 8 个执行 Job 和最终聚合门禁全部为 `success`，其中 Backend test 为 9 分 18 秒，Fresh-container browser 为 3 分 16 秒；
- 边界：本证据只关闭运维任务列表子项；其他 P0 列表仍须分别证明并列排序、资源/筛选绑定、连续遍历、越权、篡改和过期行为。

### `EV-001-010`：审计日志列表签名游标

- 映射：`CHK-001-G3-004`、`AC-001-010`；
- 结果：审计日志列表子项通过，`CHK-001-G3-004` 仍不关闭；
- 代码证据：提交 `9ad399b71c8aec79032a972382327b37d2bfa9d8` 将生产路由 `/api/v1/operations/audit-logs` 从裸整数游标改为短期有效的签名不透明游标，绑定当前管理员用户及 `action/resource_type/result` 筛选指纹；OpenAPI、生成客户端和治理页同步改为 `cursor`/`next_cursor` 字符串契约；
- 测试证据：`TestGovernanceAuditCursorIsSignedBoundExpiringAndStableAcrossConcurrentInsert` 在 PostgreSQL 上验证固定数据以两页完整返回、并发新增不回流到已越过区间，且篡改、过期、跨用户和跨筛选游标均受控拒绝；`TestGovernanceAuditForwardsOpaqueSubjectBoundCursor` 验证 HTTP 层绑定认证主体并只透传不透明游标；`TestP0UserListCursorsUseSignedExpiringCodec` 固定生产 Bootstrap 必须注入统一 Codec，且 `operations_audit_list` 必须调用 `Seal/Open`；
- CI 证据：远端运行 `33208979659` 的 8 个执行 Job 和最终聚合门禁全部为 `success`，其中 Backend test 为 13 分 25 秒，Fresh-container browser 为 3 分 5 秒；
- 边界：审计日志为按唯一追加 ID 降序的不可变事实，游标边界天然排除翻页期间新增的更高 ID；本证据只关闭该列表子项，不能替代其余 P0 列表的统一矩阵。

### `EV-001-011`：内容与热点列表签名快照游标

- 映射：`CHK-001-G3-004`、`AC-001-010`；
- 结果：内容与热点列表子项通过，`CHK-001-G3-004` 仍不关闭；
- 失败证据：`TestContentCursorIsSignedBoundExpiringAndSnapshotStableAcrossConcurrentChanges` 在修复前稳定复现第一页边界内容热度下降后第二页从两个原快照剩余项变为空页，证明旧实现回查边界行当前指标会漏项；架构契约同时因缺少 `content_list` 的统一 `Seal/Open` 而失败；
- 代码证据：提交 `1f2fe4398ca80eca5ce725c2dfc77fae7d0455df` 为生产路由 `/api/v1/contents` 与 `/api/v1/hotspots` 使用的 Ingestion 列表增加短期签名 `content_list` 快照游标，绑定完整筛选指纹、排序、数据库首屏时间、边界 ID 和实际时间/分数排序元组；后续页按首屏时间选择 `content_metric_snapshots`，排除首屏之后新增内容，不再回查可变边界行；既有 OpenAPI 继续使用不透明字符串，无契约漂移；
- 测试证据：同一 PostgreSQL 测试验证热度并列项按 ID 稳定遍历、并发新增不进入既有快照、边界指标变化不漏项，以及篡改、过期和跨排序复用均受控拒绝；原发布时间未知值与相关度排序集成测试继续通过；`TestP0UserListCursorsUseSignedExpiringCodec` 固定生产 Bootstrap 必须注入统一 Codec，且 `content_list` 必须调用 `Seal/Open`；
- CI 证据：远端运行 `33212372771` 的 8 个执行 Job 和最终聚合门禁全部为 `success`，其中 Backend test 为 9 分 32 秒，Fresh-container browser 为 4 分 7 秒；
- 边界：Notification 的 `after_id` 是跨离线周期持久重放位置而不是短期页面游标，按 004 契约保留；本证据只关闭内容与热点列表子项，不能替代其余 P0 列表的统一矩阵。

### `EV-001-012`：Monitor 列表签名高水位游标

- 映射：`CHK-001-G3-004`、`AC-001-010`；
- 结果：Monitor 列表子项通过，`CHK-001-G3-004` 仍不关闭；
- 失败证据：`TestMonitorListCursorIsSignedBoundExpiringAndSnapshotStableAcrossConcurrentInsert` 在修复前稳定复现第一页返回后新建的第 4 个 Monitor 错误进入第二页，证明旧 ID 游标没有固定一次遍历的新增边界；
- 代码证据：提交 `6ef7308842d1aebdc4907a29a8397b2e57071b73` 将 `/api/v1/monitors` 使用的 Repository 游标改为短期签名 `monitor_list` 结构，绑定 Viewer/Owner 可见性指纹、首次可见最大 ID 和上一页边界 ID，后续查询固定 `id <= snapshot_id`；提交 `6c0d32f2232d5f7db754b3569debdebde1a1906d` 进一步让普通与复合游标统一使用严格 URL-safe Base64 解码，拒绝同一签名字节的非规范文本别名；既有 OpenAPI 继续使用不透明字符串，无契约漂移；
- 测试证据：同一 PostgreSQL 测试覆盖并发新增排除、篡改、过期、跨 Owner、跨 PublishedOnly 视图和超大页拒绝，并连续运行 20 次通过；`TestCodecRejectsNonCanonicalSignatureEncoding` 对普通与复合游标保存确定性回归；`TestP0UserListCursorsUseSignedExpiringCodec` 固定生产 Bootstrap 注入统一 Codec，且 `monitor_list` 必须调用 `Seal/Open`；
- CI 证据：远端运行 `33215012565` 首次在 Linux 精确捕获非规范 Base64 别名导致的偶发测试失败，其余 7 个执行 Job 通过；严格解码修复后，远端运行 `33216292257` 的 8 个执行 Job 和最终聚合门禁全部为 `success`，其中 Backend test 为 10 分 14 秒，Fresh-container browser 为 3 分 9 秒；
- 边界：本证据冻结同一次 Monitor 遍历的新增 ID 高水位并绑定当前可见性查询形态，不扩大解释为可变状态的历史时间旅行快照；其他 P0 列表仍须分别完成统一矩阵。

### `EV-001-013`：来源连接列表签名高水位游标

- 映射：`CHK-001-G3-004`、`AC-001-010`；
- 结果：来源连接列表子项通过，`CHK-001-G3-004` 仍不关闭；
- 失败证据：`TestSourceConnectionListCursorIsSignedExpiringAndSnapshotStableAcrossConcurrentInsert` 在修复前稳定复现第一页返回 ID 1、2 后新建的 ID 4 错误进入第二页并与 ID 3 一起返回，证明旧签名 ID 游标没有固定一次遍历的新增边界；
- 代码证据：提交 `50a41a7dcae13d673a9dfb09bc33c50f882e4362` 将 `/api/v1/sources` 使用的 Repository 游标改为短期签名 `source_connection_list` 结构，保存固定过滤指纹、首次最大 ID 和上一页边界 ID，后续查询固定 `id <= snapshot_id`；既有 Public/Management 接口继续使用相同的共享团队数据集和不透明字符串契约，无 OpenAPI 漂移；
- 测试证据：同一 PostgreSQL 测试覆盖并发新增排除、篡改、过期和超大页拒绝，并连续运行 20 次通过；Source 应用、领域、连接器、PostgreSQL 与 HTTP 全模块通过；`TestP0UserListCursorsUseSignedExpiringCodec` 固定生产 Bootstrap 注入统一 Codec，且 `source_connection_list` 必须调用 `Seal/Open`；
- CI 证据：远端运行 `33218519351` 的 8 个执行 Job 和最终聚合门禁全部为 `success`，其中 Backend test 为 10 分 7 秒，Fresh-container browser 为 3 分 5 秒；
- 边界：来源连接是已认证团队成员共享集合，Public/Management 差异只发生在通过角色校验后的安全投影，因此游标无需虚构用户所有权范围；本证据只冻结同一次来源遍历的新增 ID 高水位，不能替代其余 P0 列表的统一矩阵。

### `EV-001-014`：采集运行列表签名高水位游标

- 映射：`TASK-001-S02-T06` → `SPEC-001-API-004` → `AC-001-010` → `CHK-001-G3-004`；
- 结果：采集运行列表子项通过，`CHK-001-G3-004` 仍不关闭；
- 失败证据：`TestCollectionRunListCursorIsSignedExpiringAndSnapshotStableAcrossConcurrentInsert` 在修复前稳定复现第一页返回 ID 1、2 后新建的 ID 4 错误进入第二页并与 ID 3 一起返回，证明旧签名 ID 游标没有固定一次遍历的新增边界；
- 代码证据：提交 `024baa6a44f4bda84f468884ee816db5ce43e0ce` 将 `/api/v1/collection-runs` 使用的 Repository 游标改为短期签名 `collection_run_list` 结构，保存固定过滤指纹、首次最大 ID 和上一页边界 ID，后续查询固定 `id <= snapshot_id`；既有 OpenAPI 继续使用不透明字符串，无契约漂移；
- 测试证据：同一 PostgreSQL 测试覆盖并发新增排除、篡改、过期和超大页拒绝，并连续运行 20 次通过；Source 应用、领域、连接器、PostgreSQL 与 HTTP 全模块通过；`TestP0UserListCursorsUseSignedExpiringCodec` 固定生产 Bootstrap 注入统一 Codec，且 `collection_run_list` 必须调用 `Seal/Open`；
- CI 证据：远端运行 `33220453646` 的 8 个执行 Job 和最终聚合门禁全部为 `success`，其中 Backend test 为 9 分 28 秒，Fresh-container browser 为 3 分 24 秒；
- 边界：采集运行是 Editor/Admin 角色校验后的共享运维集合，状态和目标字段可随任务执行演进；本证据冻结同一次遍历的成员范围和新增 ID 高水位，不扩大解释为可变字段的历史时间旅行快照，也不能替代其余 P0 列表的统一矩阵。

### `EV-001-015`：采集条目工作列表签名高水位游标

- 映射：`TASK-001-S02-T06` → `SPEC-001-API-004` → `AC-001-010` → `CHK-001-G3-004`；同时支撑 `TASK-002-S03-T02` → `SPEC-002-JOB-001`/`SPEC-002-OPS-001` → `AC-002-006` → `CHK-002-G4-004`；
- 结果：Source→Ingestion 采集条目工作列表子项通过，`CHK-001-G3-004` 仍不关闭；
- 失败证据：`TestCapturedItemListCursorIsSignedBoundExpiringAndSnapshotStableAcrossConcurrentInsert` 在修复前稳定复现第一页返回 ID 1、2 后新插入的 ID 4 错误进入第二页并与原快照 ID 3 一起返回，证明旧签名 ID 游标既没有冻结一次标准化遍历的新增边界，也没有绑定运行与失败重试筛选；
- 代码证据：提交 `6d69779b09bee7d92d040aad7cb3555c6d079a45` 将 `ListUnboundCaptured` 改为短期签名 `captured_item_list` 结构，绑定 `run_id`、`include_failed`、首次符合条件的最大条目 ID 和上一页边界 ID，后续查询固定 `id <= snapshot_id`；该接口是 Go Source 模块提供给 Ingestion Worker 的内部领域端口，领域 DTO、Schema 和 OpenAPI 均无变化；
- 测试证据：同一 PostgreSQL 测试覆盖并发新增排除、跨运行/跨筛选复用、篡改、过期和超大页拒绝，并连续运行 20 次通过；Source 全模块、`TestNormalizeHandlerDrainsEveryCapturedItemPageWithoutLegacyFanout` 和架构测试通过；`TestP0UserListCursorsUseSignedExpiringCodec` 固定生产 Bootstrap 注入统一 Codec，且 `captured_item_list` 必须调用 `Seal/Open`；
- CI 证据：远端运行 `33222427410` 的 8 个执行 Job 和最终聚合门禁全部为 `success`，其中 Backend test 为 10 分 32 秒，Fresh-container browser 为 3 分 6 秒；
- 边界：该列表是标准化 Worker 的有界工作遍历，不是用户可调用 API；快照排除首屏后新增的更高 ID，已成功绑定或明确失败的条目仍按工作状态有意退出默认结果集，不将本证据扩大解释为可变状态的历史时间旅行快照。

### `EV-001-016`：相关性匹配列表签名高水位游标

- 映射：`TASK-001-S02-T06` → `SPEC-001-API-004` → `AC-001-010` → `CHK-001-G3-004`；
- 结果：相关性匹配列表子项通过，`CHK-001-G3-004` 仍不关闭；
- 失败证据：`TestRelevanceMatchListCursorKeepsFirstPageHighWaterAcrossConcurrentInsert` 在修复前稳定复现首屏返回 90/80 分匹配后，新插入的 60 分匹配错误进入第二页并与原 70 分匹配一起返回，证明旧游标没有冻结一次遍历的新增成员范围；
- 代码证据：提交 `d287d813430cdddd8b8278ad8fe06b947fcf279d` 为 `relevance_match_list` 游标增加首次查询的最大匹配 ID，并让后续页只在 `match.id <= snapshot_id` 范围内按 `final_score,id` 继续；Monitor、decision 筛选绑定、签名有效期、OpenAPI 和前端生成契约保持不变；
- 测试证据：上述 PostgreSQL 测试证明两页只完整返回原始三项且排除并发低分新增；`TestRelevanceCursorsRejectTamperingAndCrossQueryReuse` 覆盖篡改、跨 Monitor 和跨 decision 复用拒绝，统一 Cursor Codec 测试覆盖过期拒绝，`TestP0UserListCursorsUseSignedExpiringCodec` 固定 `relevance_match_list` 必须调用 `Seal/Open`；Ingestion 全模块、前端与 Agent 回归均通过；
- CI 证据：远端运行 `33224826425` 的 8 个执行 Job 和最终聚合门禁全部为 `success`，其中 Backend test 为 9 分 2 秒，Fresh-container browser 为 3 分 30 秒；
- 边界：本证据冻结同一次遍历的新增匹配 ID 高水位，不扩大解释为对首屏后人工/AI 更新的分数、decision 或 Content 可见状态提供历史时间旅行快照；其他 P0 列表仍须分别完成统一矩阵。

### `EV-001-017`：微事件榜单签名高水位快照游标

- 映射：`TASK-001-S02-T06` → `SPEC-001-API-004` → `AC-001-010` → `CHK-001-G3-004`；
- 结果：微事件榜单子项通过，`CHK-001-G3-004` 仍不关闭；
- 失败证据：`TestMicroEventQueryRepositoryAppliesMultiDimensionalFiltersAndStableRelevanceCursor` 在修复前稳定复现首屏返回相关度 0.95 的事件后，首屏之后新增的 0.70 事件落入既有 `as_of` 时间边界，使第二页在返回原 0.80 事件时错误产生下一页游标；这证明仅冻结应用时间不能抵御应用与数据库时钟偏差，也没有冻结一次遍历的新增成员范围；
- 代码证据：提交 `f31f4f4525b5825119e201eda7206bf782853f89` 将 `micro_event_list` 游标升级为同时保存首屏时间与最大微事件 ID，后续页固定 `event.id <= snapshot_id`，并继续绑定完整筛选指纹、`latest/heat/relevance` 排序及实际排名元组；既有 OpenAPI 保持不透明字符串契约，无契约漂移；
- 测试证据：上述 PostgreSQL 测试证明旧游标两页只返回首屏时已有事件，首屏后的低排名新增只会出现在新查询；`TestMicroEventListCursorFreezesRankingAndRejectsChangedQuery` 固定游标携带高水位并拒绝篡改、跨排序和跨筛选复用，统一 Codec 测试覆盖过期拒绝，`TestP0UserListCursorsUseSignedExpiringCodec` 固定 `micro_event_list` 必须调用 `Seal/Open`；Event PostgreSQL 集成包、本机全量后端、前端与 Agent 门禁均通过；
- CI 证据：远端运行 `33226557991` 的 8 个执行 Job 和最终聚合门禁全部为 `success`；
- 边界：本证据冻结同一次榜单遍历的首屏时间、排名快照和新增事件 ID 高水位，不扩大解释为对首屏后的事件治理状态或成员关系变化提供完整历史时间旅行；其他 P0 列表仍须分别完成统一矩阵。

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
| `AC-001-010` | partial | 日报、微事件 Evidence、微事件榜单、全文检索结果、内容/热点、Monitor、来源连接、采集运行、采集条目工作、相关性匹配、运维任务与审计日志列表已覆盖适用的签名、过期、主体/筛选/资源绑定、篡改拒绝和并发新增期间稳定遍历；尚无全部 P0 列表的统一矩阵 |

## 实际命令与结果摘要

```text
backend: make ci
frontend: npm run openapi:check; npm run typecheck; npm run test:unit; npm audit --omit=dev --audit-level=high; npm run build
agent: uv run ruff format --check .; uv run ruff check .; uv run mypy src; uv run pytest; uv run pip-audit
repository: docker compose -f docker-compose.yml config --quiet; docker compose --env-file .env.prod.example -f docker-compose-prod.yml config --quiet（必填生产变量使用 CI 合成值注入）; git diff --check
specialized: make agent-degradation-acceptance; make agent-skill-contract-acceptance; make report-publication-acceptance; make m4-fault-recovery-acceptance; go run ./test/runner test ./internal/modules/search/... -count=1; go run ./test/runner test -tags=integration -p=1 ./internal/modules/event/infrastructure/postgres -run 'TestMicroEvent(QueryRepositoryAppliesMultiDimensionalFiltersAndStableRelevanceCursor|EvidenceCursorIsSignedBoundExpiringAndSnapshotStable|LexicalSearchUsesSnapshotKeysetOrdering)' -count=1; go run ./test/runner test -tags=integration -p=1 ./internal/modules/ingestion/infrastructure/postgres -run 'Test(ContentCursorIsSignedBoundExpiringAndSnapshotStableAcrossConcurrentChanges|ContentRepositoryListsOnlyActiveContentWithPublishedCursor|ContentRepositorySearchFiltersLatestMatchAndStableRelevanceCursor|RelevanceMatchListCursorKeepsFirstPageHighWaterAcrossConcurrentInsert)' -count=1; go run ./test/runner test ./internal/modules/ingestion/transport/http -run TestRelevanceCursorsRejectTamperingAndCrossQueryReuse -count=1; go run ./test/runner test -tags=integration -p=1 ./internal/modules/monitor/infrastructure/postgres -run TestMonitorListCursorIsSignedBoundExpiringAndSnapshotStableAcrossConcurrentInsert -count=20; go run ./test/runner test ./internal/modules/source/infrastructure/postgres -run TestSourceConnectionListCursorIsSignedExpiringAndSnapshotStableAcrossConcurrentInsert -count=20; go run ./test/runner test ./internal/modules/source/infrastructure/postgres -run TestCollectionRunListCursorIsSignedExpiringAndSnapshotStableAcrossConcurrentInsert -count=20; go run ./test/runner test ./internal/modules/source/infrastructure/postgres -run TestCapturedItemListCursorIsSignedBoundExpiringAndSnapshotStableAcrossConcurrentInsert -count=20; go run ./test/runner test ./internal/modules/ingestion/infrastructure/jobs -run TestNormalizeHandlerDrainsEveryCapturedItemPageWithoutLegacyFanout -count=1; go run ./test/runner test ./internal/shared/pagination -count=1; go run ./test/runner test -tags=integration -p=1 ./internal/modules/operations/infrastructure/postgres -run 'Test(JobRepositoryCursorIsSignedBoundExpiringAndSnapshotStable|GovernanceAuditCursorIsSignedBoundExpiringAndStableAcrossConcurrentInsert)' -count=1; go run ./test/runner test ./test/architecture -count=1
```

本机全量结果通过；前端 252 项测试通过，Python Agent 为 41 项测试通过且覆盖率 97.57%，生产依赖审计无高危漏洞；Go `govulncheck` 未发现当前调用链漏洞。远端运行 `33226557991` 的 8 个执行 Job 和最终聚合门禁全部通过。

## 未完成项与停止条件

- `CHK-001-G1-001`：等待完整四角色 UAT；
- `CHK-001-G2-001`：等待完整人工键盘/焦点矩阵，不以自动 Tab 与 WCAG 扫描替代；
- `CHK-001-G3-002`：等待同一隔离恢复副本上的联合恢复、对账和真实 RPO/RTO；
- `CHK-001-G3-003`：等待所有 P0 关键写入口的统一副作用、事实计数和追加审计矩阵；
- `CHK-001-G3-004`：日报、微事件 Evidence、微事件榜单、全文检索结果、内容/热点、Monitor、来源连接、采集运行、采集条目工作、相关性匹配、运维任务与审计日志列表子项已完成；等待其余 P0 列表的并列排序、并发新增、连续遍历和越权/篡改/过期游标矩阵。

以上任一项缺失时，本 Acceptance 不得改为 `passed`，001 Plan/PRD 不得改为 `completed/implemented`。

## 影响与回滚验证

- Schema、运行配置、部署拓扑和 OpenAPI：无变更；OpenAPI 的日报、微事件 Evidence、微事件榜单、全文检索、内容/热点、Monitor、来源连接、采集运行、相关性匹配、运维任务与审计日志 `cursor`/`next_cursor` 使用不透明字符串；采集条目工作列表只存在于 Go 模块内部；
- 运行影响：日报列表拒绝篡改、过期或跨筛选复用的游标，微事件 Evidence 列表拒绝篡改、过期或跨事件复用的游标，微事件榜单拒绝篡改、过期或跨排序/筛选复用并以首屏时间和最大事件 ID 稳定后续页，全文检索拒绝篡改、过期、跨主体或跨筛选复用的游标并冻结遍历快照，内容/热点列表拒绝篡改、过期或跨筛选/排序复用并以首屏指标快照稳定后续页，Monitor 列表拒绝篡改、过期或跨可见性复用并排除首屏后的新增 ID，来源连接与采集运行列表拒绝篡改和过期游标并排除首屏后的新增 ID，采集条目工作列表额外拒绝跨运行或跨失败重试筛选复用并排除首屏后的新增 ID，相关性匹配列表拒绝篡改、过期或跨 Monitor/decision 复用并排除首屏后的新增 ID，运维任务列表拒绝篡改、过期、跨管理员或跨筛选复用并修复分页漏项，审计日志列表拒绝篡改、过期、跨管理员或跨筛选复用；所有签名游标拒绝非规范 Base64 别名；项目继续由根 Compose 运行，测试继续使用本机锁定工具链和可丢弃测试服务；
- 回滚：若证据引用失效，回退本文件对应 EV 和 Plan 勾选即可，不删除业务事实、对象、Vault 内容或任务历史；架构契约会在引用的测试、命令或门禁状态漂移时失败；
- 已知限制：本文件固定的代码基线早于记录本文件的提交；记录提交由同一 CI 再验证，但不以无法实现的“提交自引用哈希”冒充证据。

## 验收人

- Owner：HotKey Team；
- 自动化执行：Codex；
- 整体结论：failed（部分门禁通过，001 尚未完成）。
