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
verified_revision: "3d9acccbc136195ee1c26fa7d9cec69cef2d1740"
verification_date: "2026-08-29"
---

# HotKey 产品需求分析与总体架构验收

## 结论

本轮结论为 `failed`，含义是 001 整体尚未通过，而不是本轮自动化失败。当前已保存 P0 主故事现状、架构真实性、单一契约、降级边界、非向量检索/人工知识保护、全部 P0 公开列表/固定参考集边界、关键写入一致性，以及容量与联合恢复二十五组可复核证据；`AC-001-006`、`AC-001-009` 与 `AC-001-010` 已完成。完整四角色 UAT 和人工键盘矩阵仍未完成，因此 Plan 保持 `in_progress`，PRD 保持 `approved`。

本文件只关闭证据完整的局部门禁，不将浏览器 Fixture、自动化测试或候选性能结果扩大解释为完整 P0 发布验收。

## 验证基线

| 项目 | 实际值 |
|---|---|
| 验证日期 | 2026-08-29（Asia/Shanghai） |
| 代码基线 | `3d9acccbc136195ee1c26fa7d9cec69cef2d1740` |
| 本机环境 | macOS 26.6.2、Apple arm64、Go 1.26.5、Node.js 24.19.0、uv 0.11.32 |
| 项目运行 | 根 `docker-compose.yml`；Go Core、Python Agent、Web、PostgreSQL、Redis、MinIO 共 6 个服务均为 `healthy`；API `/readyz` 与 Web 首页均返回 HTTP 200 |
| 自动化环境 | 本机既有工具链与可丢弃测试库；GitHub Actions Ubuntu Runner 与隔离 PostgreSQL、Redis、MinIO、Fresh Compose Project |
| 远端证据 | [GitHub Actions 33248525810](https://github.com/StephenQiu30/hotkey-server/actions/runs/33248525810)：Backend static/test/vulnerability、Worker recovery、Frontend、Python Agent、Compose 与 Fresh-container browser 共 8 个 Job 及最终汇总全部 `success`；Worker Job 实际执行联合恢复 |

## P0 主故事现状矩阵

`EV-001-001` 保存的是当前实现和缺口，不声明 `AC-001-001` 已整体通过。

| 环节 | 当前代码/事实源 | API 与页面 | 可复核测试证据 | 当前结论 |
|---|---|---|---|---|
| 登录与身份 | `backend/internal/modules/identity/`、`users`、`auth_sessions` | `/api/v1/auth/login`、`/api/v1/auth/me`、Dashboard Shell | `TestFourRoleSessionLifecycleUsesCurrentRoleAndNeverResurrectsRevokedSessions`；Fresh-container 真实登录 | 已实现；完整四角色 UAT 未执行 |
| 用户管理列表 | `users` 与 Identity User Repository | `/api/v1/users`、`/dashboard/users` | `TestUserRepositoryListCursorIsSignedFilterBoundExpiringAndSnapshotStable`、`TestListUsersPassesPaginationAndFiltersAndReturnsSafePage`、`dashboard-users-page.test.tsx` | 管理员列表使用角色/状态/搜索绑定的短期签名高水位游标；服务端筛选替代前端全量加载和切片，并发注册不进入既有遍历 |
| Monitor | `backend/internal/modules/monitor/`、`monitors`、不可变配置版本与 Source 所有的扫描事实 | `/api/v1/monitors`、`/api/v1/monitors/{id}/versions`、`/api/v1/monitors/{id}/scans`、`/dashboard/settings` | `make monitor-publication-acceptance`、`TestMonitorListCursorIsSignedBoundExpiringAndSnapshotStableAcrossConcurrentInsert`、`TestMonitorConfigHistoryCursorIsSignedViewBoundExpiringAndSnapshotStable`、`TestMonitorScanCursorIsSignedResourceBoundExpiringAndSnapshotStable` 及所有权负向测试 | Monitor、配置历史与扫描历史均使用短期签名快照游标；Versions 绑定 Monitor/草稿可见性，Scans 每页重新授权并绑定 Monitor；并发新增不污染既有遍历；浏览器主故事当前使用固定 Monitor Fixture |
| 来源、采集与原始证据 | `backend/internal/modules/source/`、MinIO 对象、采集事实与 River Job | `/api/v1/sources`、`/api/v1/collection-runs`、`/api/v1/monitors/{id}/collect`、`/dashboard/sources` | `TestRSSHNPipelineRecovery`、四点 Worker Recovery、`TestCollectionRunListCursorIsSignedExpiringAndSnapshotStableAcrossConcurrentInsert`、`TestCapturedItemListCursorIsSignedBoundExpiringAndSnapshotStableAcrossConcurrentInsert`、对象写入失败零条目事实测试 | 已实现自动化链；来源连接、采集运行及 Source→Ingestion 采集条目工作列表使用短期签名高水位游标；本轮未执行真实 RSS/HN/X 授权冒烟 |
| 内容、事件与研判 | `backend/internal/modules/ingestion/`、`event/`、`intelligence/` 与根 `agent/` | `/api/v1/contents`、事件 API、`/dashboard/contents`、`/dashboard/events` | Agent 契约/降级、Evidence 白名单、事件语义与治理门禁 | 已实现自动化链；真实模型质量批准另由 003 管理 |
| 通知 | `notification_outbox_events`、`user_notifications`、`notification_read_receipts` | `/api/v1/notifications`、WebSocket、`/dashboard/notifications` | `make notification-convergence-acceptance`；浏览器 Fixture 精确产生 2 条报告通知 | 当前事实、重放和跨设备收敛已验证 |
| 日报与逐句 Evidence | `reports`、冻结 Revision、逐句 Citation 关系 | `/api/v1/reports`、`/dashboard/reports` | `make report-publication-acceptance`；`TestReportPublicationRevalidatesCitationFactsInsteadOfAggregateEvidenceState` | 每个事实句必须绑定允许 Evidence 的入口已验证；浏览器 Fixture 精确发布 1 份报告 |
| Vault 知识 | PostgreSQL Knowledge 事实、Vault 自动/人工区域与受保护 Revision | `/api/v1/knowledge/*`、`/dashboard/knowledge` | `TestUpdateVaultDocumentPreservesHumanRegionByteForByte`、`TestRecoverVaultDocumentUsesOnlyProtectedHumanRegionSources`、`TestKnowledgeListCursorsAreSignedBoundExpiringAndSnapshotStable` | 自动区域可重建且人工区域逐字保护已验证；Documents/Proposals 使用独立短期签名快照游标；浏览器 Fixture 精确应用 1 个知识投影 |
| 全文检索 | `document_version_search_indexes` 与三个拥有模块的 PostgreSQL 词法查询 | `/api/v1/search`、`/dashboard/search` | `TestP0LexicalRecallUsesOnlyAuditablePostgresFTS`、三个 PostgreSQL 词法集成测试、`TestServiceCursorIsSignedBoundExpiringAndSnapshotStableAcrossOwners` | P0 不调用向量/Embedding/RAG；结果列表使用短期签名快照游标；浏览器 Fixture 精确检出 1 个知识结果 |
| 内容与热点列表 | `contents`、`content_metric_snapshots` 与 Ingestion Owner Repository | `/api/v1/contents`、`/api/v1/hotspots`、`/dashboard/contents` | `TestContentCursorIsSignedBoundExpiringAndSnapshotStableAcrossConcurrentChanges`、`TestContentListStatementUsesDatabaseOrderingForEveryHotspotSort` | 列表使用短期签名快照游标；热度指标变化不再改变已开始遍历的边界，并发新增不污染既有快照 |
| Document Match 列表 | `document_match_decisions`、追加写 Override 与 Ingestion Document Match Repository | `/api/v1/monitors/{id}/document-matches` | `TestDocumentMatchListCursorIsSignedBoundExpiringAndStableAcrossConcurrentInsert`、Document Match HTTP 角色边界测试 | 列表按不可变 Decision ID 倒序，游标绑定 Monitor 与 effective decision 筛选；并发新增不进入既有遍历，篡改、过期和跨查询复用受控拒绝 |
| 相关性匹配列表 | `monitor_matches` 与 Ingestion Relevance Repository | `/api/v1/monitors/{id}/matches` | `TestRelevanceMatchListCursorKeepsFirstPageHighWaterAcrossConcurrentInsert`、`TestRelevanceCursorsRejectTamperingAndCrossQueryReuse` | 列表使用 Monitor/decision 绑定的短期签名高水位游标；首屏后的低分新增匹配不进入既有遍历 |
| 反馈建议列表 | `monitor_feedback_suggestions` 与 Ingestion Relevance Repository | `/api/v1/monitors/{id}/feedback/suggestions` | `TestRelevanceSuggestionListCursorUsesImmutableOrderAcrossConcurrentUpdate`、`TestRelevanceCursorsRejectTamperingAndCrossQueryReuse` | 列表改用不可变 ID 倒序和 Monitor/status 绑定的 v2 短期签名游标；未读建议支持数更新不再造成跨页漏项 |
| Rights 历史列表 | `source_rights_policies`、`source_rights_decision_batches` 与 Source Rights Repository | `/api/v1/source-endpoints/{id}/rights-policies`、`/api/v1/source-endpoints/{id}/rights-decision-batches` | `TestRightsManagementProjectionListCursorsAreSignedBoundExpiringAndStable`、Rights HTTP Admin 边界测试 | 两条列表按不可变 ID 倒序，游标绑定 Source Endpoint 与列表类型；并发新增、跨端点/跨类型复用、篡改、过期及超限页均有矩阵覆盖 |
| 微事件榜单 | `micro_events`、版本化 Heat/Relevance/Evidence 快照与 Event Query Repository | `/api/v1/micro-events`、`/dashboard/events` | `TestMicroEventQueryRepositoryAppliesMultiDimensionalFiltersAndStableRelevanceCursor`、`TestMicroEventListCursorFreezesRankingAndRejectsChangedQuery` | `latest/heat/relevance` 榜单使用筛选、排序、时间和首屏最大事件 ID 绑定的短期签名游标；首屏后的低排名新增事件不进入既有遍历 |
| 运维任务 | `river_job` 安全元数据投影与 Operations Owner Repository | `/api/v1/operations/jobs`、`/dashboard/governance` | `TestJobRepositoryCursorIsSignedBoundExpiringAndSnapshotStable`、`TestJobListForwardsOpaqueSubjectBoundCursorAndReturnsNextCursor` | 管理员任务列表使用主体/筛选绑定的签名快照游标，连续遍历不跳过 `limit+1` 边界项 |
| 运维审计日志 | 追加写 `audit_logs` 与 Operations Governance Repository | `/api/v1/operations/audit-logs`、`/dashboard/governance` | `TestGovernanceAuditCursorIsSignedBoundExpiringAndStableAcrossConcurrentInsert`、`TestGovernanceAuditForwardsOpaqueSubjectBoundCursor` | 管理员审计列表使用主体/筛选绑定的短期签名游标；篡改、过期和跨上下文复用均受控拒绝 |
| AI Model Profiles | `ai_model_profiles` 与 Intelligence Owner Repository | `/api/v1/ai/model-profiles` | `TestModelProfileListCursorIsSignedExpiringAndSnapshotStable`、`TestModelProfileRoutesEnforceAdminControlPlaneAndRedactCredentials` | 管理员列表使用短期签名 ID 快照游标；并发新增只进入新查询，凭据不进入 DTO，篡改、过期和超限页受控拒绝 |
| 固定参考集 | Operations Retention Domain/Schema 与 Source Preset Catalog | `/api/v1/operations/retention-policies`、`/api/v1/source-presets` | `TestRetentionPolicySchemaRejectsAnEighthDataClass`、`TestCatalogHasFixedTwelveItemMaximumAndKeepsConnectionDetailsServerSide`、HTTP 契约测试 | Retention Policies 由领域与数据库共同固定为 7 类并拒绝第 8 类；Source Presets 由服务端代码固定为 12 项、返回防御性副本且无游标遍历入口 |

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

### `EV-001-018`：反馈建议列表不可变排序游标

- 映射：`TASK-001-S02-T06` → `SPEC-001-API-004` → `AC-001-010` → `CHK-001-G3-004`；
- 结果：反馈建议列表子项通过，`CHK-001-G3-004` 仍不关闭；
- 失败证据：`TestRelevanceSuggestionListCursorUsesImmutableOrderAcrossConcurrentUpdate` 在修复前稳定复现首屏返回较新的 gamma/beta 建议后，尚未读取的 alpha 建议因支持数更新而刷新 `updated_at` 并跳到游标上方，第二页变为空，证明可变更新时间不能作为连续遍历排序键；
- 代码证据：提交 `118c2667617ab051fc5df0f11b9ec64977cb1507` 将 `relevance_suggestion_list` 的数据库顺序改为不可变唯一 ID 倒序，并将签名游标升级为 v2，只保存边界 ID 且继续绑定 Monitor 和 status；旧 v1 可变排序游标、篡改、跨 Monitor 或跨 status 复用均受控拒绝，既有 OpenAPI 继续使用不透明字符串，无契约漂移；
- 测试证据：上述 PostgreSQL 集成测试证明未读建议并发更新支持数后仍在第二页精确返回；`TestRelevanceCursorsRejectTamperingAndCrossQueryReuse` 固定 v2 正常解码、篡改、跨 Monitor、跨 status 和 v1 游标拒绝；Ingestion 全模块、本机 `make ci`、前端 252 项单测与构建、Agent 41 项测试与审计均通过；
- CI 证据：远端运行 [33228203247](https://github.com/StephenQiu30/hotkey-server/actions/runs/33228203247) 的 8 个 Job 全部为 `success`；
- 边界：本证据保证同一次反馈建议遍历不会因支持数或审核导致的 `updated_at` 变化漏掉尚未读取的记录；状态变更后建议有意退出当前 status 过滤结果，不扩大解释为可变状态的历史时间旅行快照，其他 P0 列表仍须分别完成统一矩阵。

### `EV-001-019`：Document Match 与 Rights 历史列表游标矩阵

- 映射：`TASK-001-S02-T06` → `SPEC-001-API-004` → `AC-001-010` → `CHK-001-G3-004`；同时支撑 002 的来源 Rights 事实边界；
- 结果：Document Match、Rights Policy 与 Rights Decision Batch 三个列表子项通过，`CHK-001-G3-004` 仍不关闭；
- 实现证据：当前生产 `DocumentMatchRepository` 按不可变 `document_match_decisions.id DESC` 遍历，签名游标键为 `document_match_decision_id` 并绑定 Monitor 与 effective decision 筛选；`RightsManagementRepository` 按不可变 Policy/Decision Batch ID 倒序遍历，签名游标绑定 Source Endpoint 及 `policies`/`decision-batches` 类型；生产 Bootstrap 为两者注入统一短期 Codec。本轮提交 `8d33b2812acf0d97eda48dd715e782b82928e074` 将这些既有正确行为固定为架构防回退契约，没有新增 Schema、OpenAPI 或运行时分支；
- 测试证据：`TestDocumentMatchListCursorIsSignedBoundExpiringAndStableAcrossConcurrentInsert` 验证两页精确遍历、并发新增只进入新查询、签名形态、篡改、过期、跨 Monitor、跨 decision 筛选及超限页拒绝；`TestRightsManagementProjectionListCursorsAreSignedBoundExpiringAndStable` 对 Policy 与 Decision Batch 分别验证两页精确遍历、并发新增、篡改、过期、跨 Source Endpoint、跨列表类型及超限页拒绝；既有 HTTP 测试继续证明 Viewer/Editor 不能读取 Admin Rights 历史，Document Match 读取只向已认证角色开放；`TestP0UserListCursorsUseSignedExpiringCodec` 固定生产组合必须使用统一 Codec；
- CI 证据：远端运行 [33230114986](https://github.com/StephenQiu30/hotkey-server/actions/runs/33230114986) 的 8 个 Job 全部为 `success`；
- 边界：该证据登记时的 OpenAPI 全量 GET 盘点还发现 Users、Knowledge Documents/Proposals、Monitor Scans/Versions 等未分页公开列表，以及 AI Model Profiles、Retention Policies、Source Presets 等可能属于有界参考集但尚无最大基数契约；这些列表必须逐项补稳定游标或证明固定有界且不属于连续遍历，故本证据当时不能关闭全范围矩阵。

### `EV-001-020`：用户管理列表筛选与签名高水位游标

- 映射：`TASK-001-S02-T06` → `SPEC-001-API-004` → `AC-001-010` → `CHK-001-G3-004`；
- 结果：Users 列表子项通过，`CHK-001-G3-004` 仍不关闭；
- 失败证据：修复前的 Repository/Application/HTTP/架构契约测试因缺少 `UserPage`、`UserListQuery`、共享游标 Codec 注入和游标契约而稳定失败；前端测试同时固定搜索和翻页必须发送服务端参数，禁止继续用一次性全量加载后的本地筛选与数组切片冒充分页；
- 实现证据：提交 `117a21af8ded6511c92af68b06f9cfcad524c724` 将生产路由 `/api/v1/users` 改为服务端 email/display name 搜索、角色和 active/disabled/deleted 生命周期筛选，以首次查询匹配集合的最大用户 ID 冻结升序遍历范围，并通过生产 Bootstrap 注入共享短期 Codec；`identity_user_list` 游标只保存筛选摘要、快照高水位和边界 ID，不暴露搜索原文，OpenAPI、生成客户端和用户管理页同步改为 `items`/`next_cursor` 不透明字符串契约；
- 测试证据：`TestUserRepositoryListCursorIsSignedFilterBoundExpiringAndSnapshotStable` 在 PostgreSQL 上验证两页精确遍历、并发注册只进入新查询、搜索/角色/状态筛选、篡改、过期、跨筛选复用和页大小上限拒绝；`TestListUsersPassesPaginationAndFiltersAndReturnsSafePage` 与 `TestListUsersRejectsInvalidQueryBeforeService` 固定 HTTP 参数、安全投影和写前拒绝，`TestListUsersMapsInvalidCursorToStableValidationError` 固定脱敏错误映射；前端 `dashboard-users-page.test.tsx` 验证真实不透明游标前后翻页、服务端筛选、生命周期操作刷新和失败重试；
- CI 证据：远端运行 [33232453453](https://github.com/StephenQiu30/hotkey-server/actions/runs/33232453453) 的 8 个 Job 全部为 `success`；
- 边界：用户管理路由每次请求都重新执行 Admin 授权，当前单 Workspace 的管理员共享同一可见集合，因此游标无需虚构用户所有权；本证据冻结并发新增 ID，不承诺对翻页期间角色、状态或软删除变化提供历史时间旅行。该证据登记时，Knowledge Documents/Proposals、Monitor Scans/Versions 等其余公开列表，以及三个尚未证明固定有界的参考集仍须逐项完成矩阵。

### `EV-001-021`：Knowledge Documents/Proposals 双列表签名快照游标

- 映射：`TASK-001-S02-T06` → `SPEC-001-API-004` → `AC-001-010` → `CHK-001-G3-004`；同时保持 004 的 PostgreSQL Knowledge 事实与 Vault 投影边界；
- 结果：Knowledge Documents 与 Proposals 两个公开列表子项通过，`CHK-001-G3-004` 仍不关闭；
- 失败证据：修复前 `/api/v1/knowledge/documents` 返回全部非归档文档，`/api/v1/knowledge/proposals` 只返回最新 100 条且无法继续遍历；新增 Repository/HTTP/前端测试分别因缺少分页领域对象、共享 Codec 注入、页对象响应和双列表游标状态而稳定失败；
- 实现证据：提交 `2b03b67e86bb2f3fe7332604d8ff610593774659` 为 Documents 使用不可变 `id ASC` 与首次查询最大非归档 ID，为 Proposals 使用 `created_at DESC, id DESC` 与首次查询匹配 status 的最大 ID；`knowledge_document_list` 与 `knowledge_proposal_list` 分别签名，Proposal 游标绑定 status，过期、篡改、跨筛选、跨列表和超限页均映射为脱敏校验错误。OpenAPI 与生成客户端统一返回 `items`/`next_cursor`，知识页为两个列表维护互不干扰的前后翻页状态；内部 Vault Reconciler 继续使用未改变的 `ListDocuments` 全集读取，不把公开分页错误扩散到对账边界；
- 测试证据：`TestKnowledgeListCursorsAreSignedBoundExpiringAndSnapshotStable` 在 PostgreSQL 上验证两个列表逐页精确返回、并发新增不进入既有遍历、归档文档排除、Proposal status 绑定、篡改、过期、跨用途与页大小上限拒绝；`TestKnowledgeListRoutesForwardOpaqueCursorsAndRejectInvalidQueries` 固定 HTTP 层只透传不透明游标并在访问 Repository 前拒绝非法 status/limit；`dashboard-knowledge-page.test.tsx` 验证 Documents/Proposals 独立翻页和当前页刷新；`TestP0UserListCursorsUseSignedExpiringCodec` 固定生产 Bootstrap 注入统一 Codec；
- CI 证据：远端运行 [33234032155](https://github.com/StephenQiu30/hotkey-server/actions/runs/33234032155) 的 8 个必需 Job 与 `All acceptance gates` 汇总全部为 `success`；本机后端 `make ci`、前端 254 项测试/生产构建、Agent 41 项测试/审计、开发与生产 Compose 配置以及 6 个现有健康服务复验均通过；
- 边界：每次请求仍重新执行 Editor/Admin 授权，共享治理角色读取同一可见集合；文档归档或 Proposal 状态变更会使记录退出当前可变视图，本证据不伪造数据库历史时间旅行。该证据登记时仍剩余 Monitor Scans/Versions，以及 AI Model Profiles、Retention Policies、Source Presets 的固定有界性须逐项完成。

### `EV-001-022`：Monitor Scans/Versions 资源与可见性绑定快照游标

- 映射：`TASK-001-S02-T06` → `SPEC-001-API-004` → `AC-001-010` → `CHK-001-G3-004`；同时保持 Monitor 拥有配置与关联、Source 拥有采集事实的模块边界；
- 结果：Monitor Scans 与配置 Versions 两个公开历史列表子项通过，`CHK-001-G3-004` 仍不关闭；
- 失败证据：修复前两个接口都只返回首段数组且不能继续遍历；Scans 仅验证登录态，没有在每次请求按目标 Monitor 重新授权；新增 PostgreSQL/Application/HTTP/架构测试先因缺少分页领域对象、共享 Codec、资源/可见性绑定和页对象响应而失败，扫描测试还捕获了生成下一页游标时过早改写当前页边界的实现缺陷；
- 实现证据：提交 `021b90500a945e4d9babf0750caecd9ed81aec14` 为 `monitor_scan_list` 使用扫描组最大 Collection Run ID 倒序边界及首次查询 Run ID 高水位，并将游标签名绑定 Monitor；为 `monitor_config_history` 使用不可变 `revision DESC,id DESC`，绑定 Monitor 与 `include_drafts` 视图，并用首次查询 ID/时间快照阻止并发新增或翻页期间才发布的草稿进入旧的只读遍历。Source 扫描 Application 每页调用 Monitor `AuthorizeRead`，Viewer/非所有者 Analyst 只能读取 active/paused 且已发布的 Monitor，协作者保留既有草稿历史权限；
- 测试证据：`TestMonitorScanCursorIsSignedResourceBoundExpiringAndSnapshotStable` 与 `TestMonitorConfigHistoryCursorIsSignedViewBoundExpiringAndSnapshotStable` 在 PostgreSQL 上验证连续翻页、并发新增只进入新查询、篡改、过期、跨 Monitor、跨草稿可见性和页大小上限拒绝；`TestAnalystCanManageOnlyAnOwnedMonitor` 固定草稿/已发布读取边界，Source 三来源聚合测试证明每次扫描请求重新授权；HTTP 测试固定不透明游标透传与 `next_cursor`，`TestP0UserListCursorsUseSignedExpiringCodec` 固定生产 Bootstrap 为两个用途注入统一 Codec；设置页只请求实际展示的最新一条扫描，未凭空增加版本历史 UI；
- CI 证据：远端运行 [33235504594](https://github.com/StephenQiu30/hotkey-server/actions/runs/33235504594) 的 8 个必需 Job 与 `All acceptance gates` 汇总全部为 `success`；本机后端 `make ci`、前端 254 项测试/生产构建、开发与生产 Compose 配置以及 6 个现有健康服务复验均通过；
- 边界：Scans 冻结一次遍历开始后的新增 Run，不承诺对已存在 Run 的运行状态变化提供历史时间旅行；Versions 的协作者视图包含草稿、只读视图仅含首次查询时已发布的版本，配置状态只能沿既有状态机演进。剩余 AI Model Profiles、Retention Policies、Source Presets 必须分别补分页或证明固定有界参考集。

### `EV-001-023`：AI Model Profiles 与固定参考集边界闭环

- 映射：`TASK-001-S02-T06` → `SPEC-001-API-004` → `AC-001-010` → `CHK-001-G3-004`；
- 结果：AI Model Profiles、Retention Policies 与 Source Presets 三个剩余边界通过，`AC-001-010` 和 `CHK-001-G3-004` 关闭；
- 失败证据：修复前 AI Model Profiles 管理接口一次返回全部记录，缺少页大小、快照与共享生产 Codec；Retention Domain/Schema 接受任意第 8 个 `data_class`；新增测试先分别因缺少分页领域对象/构造器、任意数据类仍通过校验、数据库未拒绝第 8 类和 Canonical Catalog 未记录第三个 CHECK 而失败。Source Presets 的既有 12 项实现已通过初始固定目录测试，因此没有伪造红灯；本轮只把精确 ID、12 项上限、防御性副本和 HTTP 无遍历契约固化为回归门禁；
- 实现证据：提交 `5f06061d1bd6f2d571f7710b0ab35ff53bbab566` 为 `/api/v1/ai/model-profiles` 增加 `limit`、`cursor` 与 `next_cursor`，以不可变 `id ASC` 和首次查询最大 ID 冻结遍历范围；`ai_model_profile_list` 使用统一短期签名 Codec，生产 Bootstrap 注入正式密钥，过期、篡改和超限页映射为脱敏校验错误，OpenAPI 与生成客户端同步。Retention Policies 在领域白名单与 Canonical PostgreSQL Schema 中固定为 7 类，既有数据库通过命名 CHECK 收敛且第 8 类写入失败；Source Presets 继续由 Go 服务端固定为 12 项，不提供游标或客户端连接细节；
- 测试证据：`TestModelProfileListCursorIsSignedExpiringAndSnapshotStable` 验证两页连续遍历、并发新增只进入新查询、签名、篡改、过期和页大小上限；`TestModelProfileRoutesEnforceAdminControlPlaneAndRedactCredentials` 固定 Admin 授权、参数透传、脱敏 DTO 与非法参数写前拒绝；`TestRetentionPolicyValidationAllowsOnlyFixedSevenDataClasses`、`TestRetentionPolicySchemaRejectsAnEighthDataClass` 和 `TestRetentionPoliciesAreAFixedSevenItemReferenceCatalog` 固定领域、真实 PostgreSQL 与 Canonical Catalog 三层契约；`TestCatalogHasFixedTwelveItemMaximumAndKeepsConnectionDetailsServerSide` 与 `TestSourcePresetCatalogIsServedByTheBackend` 固定精确 12 项、防御性副本、服务端解析及无 `next_cursor` 的有界响应；`TestP0UserListCursorsUseSignedExpiringCodec` 固定 AI 生产 Codec 接线；
- CI 证据：远端运行 [33237476339](https://github.com/StephenQiu30/hotkey-server/actions/runs/33237476339) 的 8 个必需 Job 与 `All acceptance gates` 汇总全部为 `success`；本机后端 `make ci`、前端 254 项测试/生产构建、Agent 41 项测试与审计、开发/生产 Compose 配置以及 6 个既有健康服务的 API/Web 运行态复验均通过；
- 完整范围：`EV-001-006` 至 `EV-001-023` 已逐项登记日报、微事件 Evidence/榜单、全文检索、内容/热点、Monitor/Scans/Versions、Document Match、来源连接、Rights Policy/Decision Batch、采集运行/条目、相关性匹配、反馈建议、运维任务/审计、Users、Knowledge Documents/Proposals、AI Model Profiles、Retention Policies 与 Source Presets；
- 边界：AI 管理列表包含软删除记录以保持成员集合稳定，冻结并发新增但不伪造翻页期间字段更新的历史时间旅行；Retention 与 Presets 是小型固定参考集，不适用用户主体、并列排序或连续游标 Fixture，其有界性由精确成员、上限、无遍历入口和数据库拒绝额外成员共同证明。

### `EV-001-024`：关键写操作一致性与不可覆盖审计闭环

- 映射：`TASK-001-S02-T05` → `SPEC-001-API-003` → `AC-001-009` → `CHK-001-G3-003`；
- 结果：以 Rights Policy/Decision Batch 作为 `AC-001-009` 定义的正式 P0 关键写操作 Fixture，未认证、越权、无效输入、固定幂等键重复/冲突、旧版本和 8 路并发矩阵全部通过，`AC-001-009` 与 `CHK-001-G3-003` 关闭；
- 失败证据：新增 Schema 契约与真实 PostgreSQL 验收首次运行时，认证、授权、幂等、旧版本、并发和审计计数均已通过，但 `audit_logs` 的 `UPDATE` 与 `DELETE` 仍成功，测试以“缺少 `audit_logs_append_only`”和“audit update/delete succeeded”稳定失败；未伪造其余既有正确行为的红灯；
- 实现证据：提交 `7b12cf0a539e918945d714ae41b42cd4a222bf7a` 在唯一 `backend/db/schema.sql` 中增加 `audit_logs_append_only` 触发器，使用 SQLSTATE `23514` 拒绝更新或删除通用 Operations 审计事实；没有新增迁移目录、服务、端口或运行依赖，既有成功审计仍与业务事实同事务写入，拒绝/冲突审计仍通过独立事务追加；
- 测试证据：`TestCriticalRightsWriteMatrixPreservesFactsAndAppendsSanitizedAudit` 在可丢弃 PostgreSQL 中证明无效输入和 Viewer 越权时 Policy/Batch/Decision 事实计数不变，Policy 与 Decision 首次提交各只写一次、相同请求重放复用原结果、同键异载荷冲突不新增事实、旧 Policy 版本不写 Decision，8 路同键并发只产生 1 个首次结果和 1 组 Batch/Decision；成功、拒绝、幂等冲突和版本冲突形成精确 8 行净化审计，敏感哨兵为 0，随后直接 `UPDATE/DELETE` 均被数据库拒绝且审计矩阵不变。`TestRightsManagementMutationRejectsUnauthenticatedBeforeService` 与四角色路由矩阵证明未认证/Viewer 请求不进入业务服务；`TestRightsManagementTransportMapsStableRepositoryFailures` 固定过期版本与写冲突返回 HTTP 409/稳定冲突码；`TestRightsManagementSchemaPersistsIdempotencyActorAndDecisionBatchFacts` 固定 Schema 追加写契约；
- 本机证据：相关 Rights/Audit 六包定向测试通过；后端 `make ci` 的 OpenAPI、vet、build、架构、仓库、空库/非空库 Schema、数据库运行时和全量测试均通过，首次 `govulncheck` 下载漏洞索引遇到一次网络 `EOF`，仅重试未完成的漏洞门禁后确认 0 个当前调用链漏洞；前端 254 项测试/生产构建、Agent 41 项测试/97.57% 覆盖率/依赖审计、开发与生产 Compose 配置以及 6 个既有健康服务复验均通过；
- CI 证据：远端运行 [33240077568](https://github.com/StephenQiu30/hotkey-server/actions/runs/33240077568) 的 8 个必需 Job 与 `All acceptance gates` 汇总全部为 `success`；
- 边界：该 Fixture 按 `AC-001-009` 的单一关键写操作定义选择最严格的 Rights 管理边界，并由其他模块已有专项写入测试共同防回归；它不替代 `AC-001-002` 的完整四角色人工 UAT，也不把读操作或无需资源版本的事件误报为乐观锁写入。

### `EV-001-025`：固定容量基线与联合恢复零差异闭环

- 映射：`TASK-001-S03-T01` → `SPEC-001-OBS-001` → `AC-001-006` → `CHK-001-G3-002`；
- 结果：隔离环境中的固定容量基线和 PostgreSQL/MinIO/Vault/River 联合恢复均已实际执行并保存机器可读报告，`AC-001-006` 与 `CHK-001-G3-002` 关闭；这里通过的是候选测试与恢复演练，不把本机数字扩大为生产 SLA；
- 失败证据：收紧 `TestS03CapacityAndRecoveryEvidenceStayMeasuredAndReproducible` 后，仓库先因缺少真实联合恢复工具、Make 入口、CI 步骤、报告文件和 Plan/Acceptance 映射而稳定失败；既有恢复 verifier 只能检查人工编写的 manifest，不能证明三类存储真的恢复或 River 事实真的对账；
- 实现证据：提交 `2d56bec451246b6a04a8cb9991b384b3059bf88b` 增加 `backend/test/tools/joint-recovery-drill`、`make joint-recovery-acceptance` 及 Worker CI 步骤；提交 `3d9acccbc136195ee1c26fa7d9cec69cef2d1740` 修正 CI 证据输出上下文。演练以随机源/恢复数据库、版本化源/恢复 Bucket 和临时 Vault 目录构造同一隔离恢复副本，使用真实 `pg_dump`/`pg_restore`，实际复制 MinIO 对象正文/元数据和 Vault 文件，并在完成后清理临时数据库、Bucket 和目录；生产出口被禁用；
- 容量证据：[`contents-keyset-capacity-macos-arm64-3d9acccb.json`](evidence/001/contents-keyset-capacity-macos-arm64-3d9acccb.json) 固定 macOS 26.6.2、Apple M5 10 CPU/24 GiB、PostgreSQL 18.4 loopback、100,000 行 Fixture、并发 20、预热 20、样本 1,000、warm cache 和 nearest-rank-ceiling 算法；实测 P50 453 µs、P95 1597 µs、P99 61923 µs、错误 0。该报告只覆盖 `contents` keyset page 查询，不代表 API、全文检索、网络或生产负载 SLA；
- 恢复证据：[`joint-recovery-macos-arm64-3d9acccb.json`](evidence/001/joint-recovery-macos-arm64-3d9acccb.json) 记录真实 RPO 165 ms、RTO 762 ms；`postgres_facts` 2/2、`minio_evidence` 2/2（版本 2/2）、`vault_all_files` 2/2、`vault_manual_regions` 2/2、`river_jobs_attempts` 2/2 均为期望/实际数量相等且 SHA-256 相等，`differences=[]`。RPO 取备份围栏至事故截点，RTO 取演练开始至 PostgreSQL、MinIO 和 Vault 全部可读；
- 测试与 CI 证据：本机联合恢复、容量基线、架构契约和后端 `make ci` 全部通过；远端运行 [33248525810](https://github.com/StephenQiu30/hotkey-server/actions/runs/33248525810) 的 `Joint PostgreSQL MinIO Vault and River recovery acceptance` 步骤、8 个必需 Job 与 `All acceptance gates` 汇总全部为 `success`；
- 边界：报告排除生产流量、外部连接器、通知投递和 Redis 短期状态；它证明可重复的隔离候选基线和同一恢复副本零未解释差异，生产同构环境、正式备份介质和现场恢复演习仍属于发布运维责任，RPO 165 ms 与 RTO 762 ms 不构成 SLA 承诺。

### `EV-001-026`：简单 Monitor 创建与编辑进入正式 Compiled Profile 发布链

- 结果：关闭了设置页把只有 legacy 配置、没有 ready Compiled Profile 的 Monitor 显示为“已创建并启用”的实现缺口；新建和编辑现在都必须完成配置草稿 CAS、意图草稿、版本绑定预览、成功状态确认和精确版本发布后，页面才提示成功；本证据推进 `AC-001-001` 的 Monitor→采集入口，但不替代真实来源驱动的完整 P0 浏览器/UAT；
- 失败证据：先收紧 `dashboard-settings-page.test.tsx`，要求一次简单创建实际调用 Draft、Intent、Preview Status 和 Publish 契约；修复前测试稳定显示 `putMonitorsIdDraft` 调用次数为 0，证明旧页面只调用 `POST /api/v1/monitors`，与调度器“无 ready Compiled Profile 不准入”的门禁断开；
- 实现证据：设置页继续保留名称、监控词、来源、周期和邮件提醒这组简单产品字段，但内部通过既有生成客户端建立或替换配置草稿，用强 ETag 与 Idempotency-Key 建立意图和预览 Run，轮询到 `succeeded` 后从版本历史读取精确 draft version 再发布；编辑复用相同链路，已有失败草稿通过当前 draft/resource version 继续 CAS，不回退到 legacy 匹配；
- 失败关闭：预览返回 `failed`、`invalidated`、未知状态或超时时停止发布并保留可重试 UI；专项测试明确断言失败预览的 `postMonitorsIdPublish` 调用次数为 0；
- 本机证据：`dashboard-settings-page.test.tsx` 13 项通过，前端 TypeScript、全量单测、生产依赖审计和 Production Build 均通过；
- 边界：创建接口在兼容层仍先产生一次 legacy published Monitor，页面随即创建 replacement draft 并完成正式发布；调度器继续拒绝该短暂中间态，绝不回退读取 legacy rules。真实 RSS/HN/X 来源、四角色人工 UAT、事件以后链路仍按对应未关闭门禁执行。

## AC 结果

| AC | 结果 | 说明 |
|---|---|---|
| `AC-001-001` | partial | 已保存完整现状矩阵和报告/Vault 硬断言入口；见 `EV-001-026`，简单 Monitor 新建/编辑已进入正式 Compiled Profile 发布链；真实来源驱动的完整 P0 浏览器/UAT 故事仍未完成 |
| `AC-001-002` | partial | 自动化四角色、所有权和会话撤销已覆盖；完整四角色 UAT 未完成 |
| `AC-001-003` | passed | 见 `EV-001-002` |
| `AC-001-004` | partial | 五态、移动端、自动 WCAG 与 Tab 焦点证据存在；完整人工键盘矩阵未完成 |
| `AC-001-005` | passed | 见 `EV-001-003` |
| `AC-001-006` | passed | 见 `EV-001-025`；固定容量样本及同一隔离副本的 PostgreSQL/MinIO/Vault/River 联合恢复、五类资产对账和真实 RPO/RTO 测量已完成，结果仍是候选基线而非 SLA |
| `AC-001-007` | passed | 见 `EV-001-004` |
| `AC-001-008` | passed | 见 `EV-001-005` |
| `AC-001-009` | passed | 见 `EV-001-024`；正式 Rights 关键写 Fixture 已闭合写前认证/授权/校验、重复与冲突幂等、旧版本、8 路并发、事实计数、稳定错误码及不可覆盖净化审计 |
| `AC-001-010` | passed | 见 `EV-001-006` 至 `EV-001-023`；所有 P0 公开列表均完成适用的稳定快照游标矩阵，Retention Policies 与 Source Presets 分别完成 7 类和 12 项固定有界参考集证明 |

## 实际命令与结果摘要

```text
backend: make ci
frontend: npm ci; npm run openapi:check; npm run typecheck; npm run test:unit; npm audit --omit=dev --audit-level=high; npm run build
agent: uv sync --all-extras --locked; uv run ruff format --check .; uv run ruff check .; uv run mypy src; uv run pytest; uv run pip-audit
repository: docker compose -f docker-compose.yml config --quiet; docker compose --env-file .env.prod -f docker-compose-prod.yml config --quiet（必填生产变量使用合成值注入且不改写文件）; git diff --check
specialized: make agent-degradation-acceptance; make agent-skill-contract-acceptance; make report-publication-acceptance; make m4-fault-recovery-acceptance; go run ./test/runner test ./internal/modules/search/... -count=1; go run ./test/runner test -tags=integration -p=1 ./internal/modules/event/infrastructure/postgres -run 'TestMicroEvent(QueryRepositoryAppliesMultiDimensionalFiltersAndStableRelevanceCursor|EvidenceCursorIsSignedBoundExpiringAndSnapshotStable|LexicalSearchUsesSnapshotKeysetOrdering)' -count=1; go run ./test/runner test -tags=integration -p=1 ./internal/modules/ingestion/infrastructure/postgres -run 'Test(ContentCursorIsSignedBoundExpiringAndSnapshotStableAcrossConcurrentChanges|ContentRepositoryListsOnlyActiveContentWithPublishedCursor|ContentRepositorySearchFiltersLatestMatchAndStableRelevanceCursor|RelevanceMatchListCursorKeepsFirstPageHighWaterAcrossConcurrentInsert|RelevanceSuggestionListCursorUsesImmutableOrderAcrossConcurrentUpdate|DocumentMatchListCursorIsSignedBoundExpiringAndStableAcrossConcurrentInsert)' -count=1; go run ./test/runner test ./internal/modules/ingestion/transport/http -run TestRelevanceCursorsRejectTamperingAndCrossQueryReuse -count=1; go run ./test/runner test -tags=integration -p=1 ./internal/modules/monitor/infrastructure/postgres -run TestMonitorListCursorIsSignedBoundExpiringAndSnapshotStableAcrossConcurrentInsert -count=20; go run ./test/runner test ./internal/modules/source/infrastructure/postgres -run 'Test(SourceConnectionListCursorIsSignedExpiringAndSnapshotStableAcrossConcurrentInsert|CollectionRunListCursorIsSignedExpiringAndSnapshotStableAcrossConcurrentInsert|CapturedItemListCursorIsSignedBoundExpiringAndSnapshotStableAcrossConcurrentInsert|RightsManagementProjectionListCursorsAreSignedBoundExpiringAndStable)' -count=1; go run ./test/runner test ./internal/modules/ingestion/infrastructure/jobs -run TestNormalizeHandlerDrainsEveryCapturedItemPageWithoutLegacyFanout -count=1; go run ./test/runner test ./internal/shared/pagination -count=1; go run ./test/runner test -tags=integration -p=1 ./internal/modules/operations/infrastructure/postgres -run 'Test(JobRepositoryCursorIsSignedBoundExpiringAndSnapshotStable|GovernanceAuditCursorIsSignedBoundExpiringAndStableAcrossConcurrentInsert)' -count=1; go run ./test/runner test ./test/architecture -count=1
identity cursor: go run ./test/runner test -p=1 ./internal/modules/identity/... ./internal/bootstrap ./test/architecture -count=1
knowledge cursor: go run ./test/runner test -tags=integration -p=1 ./internal/modules/knowledge/infrastructure/postgres ./internal/modules/knowledge/transport/http ./test/architecture -run 'Test(KnowledgeListCursors|KnowledgeListRoutes|P0UserListCursors)' -count=1
monitor history cursors: go run ./test/runner test -tags=integration -p=1 ./internal/modules/monitor/infrastructure/postgres ./internal/modules/monitor/application ./internal/modules/source/application ./internal/modules/monitor/transport/http ./internal/modules/source/transport/http ./test/architecture -run 'Test(MonitorScanCursor|MonitorConfigHistoryCursor|MonitorScanReaderKeeps|RepositoryListsConfigurationHistory|MonitorHistory|MonitorScan|Scans|AnalystCanManageOnlyAnOwnedMonitor|P0UserListCursors)' -count=1
reference boundaries: go run ./test/runner test -tags=integration -p=1 ./internal/modules/intelligence/infrastructure/postgres ./internal/modules/intelligence/transport/http ./internal/modules/operations/domain ./internal/modules/operations/infrastructure/postgres ./internal/platform/database ./internal/modules/source/preset ./internal/modules/source/transport/http ./test/architecture -run 'Test(ModelProfileListCursor|ModelProfileRoutesEnforce|RetentionPolicyValidation|RetentionPoliciesAre|RetentionPolicySchemaRejects|CatalogHasFixed|SourcePresetCatalog|P0UserListCursors|OpenAPI)' -count=1
critical write: go run ./test/runner test -p=1 ./internal/platform/database ./internal/bootstrap ./internal/modules/source/application ./internal/modules/source/infrastructure/postgres ./internal/modules/source/transport/http ./internal/modules/operations/infrastructure/postgres -run '(RightsManagement|RightsActor|RightsRead|RightsAudit|CriticalRights|AuditWriter)' -count=1
capacity: HOTKEY_CAPACITY_DATASET_SIZE=100000 HOTKEY_CAPACITY_CONCURRENCY=20 HOTKEY_CAPACITY_WARMUPS=20 HOTKEY_CAPACITY_SAMPLES=1000 HOTKEY_CAPACITY_CACHE_STATE=warm HOTKEY_CAPACITY_PERCENTILE_ALGORITHM=nearest-rank-ceiling HOTKEY_CAPACITY_ENVIRONMENT=macos-26.6.2-local-postgresql-18.4-isolated HOTKEY_CAPACITY_HARDWARE='Apple M5; 10 CPU; 24 GiB RAM; internal APFS SSD; PostgreSQL 18.4 loopback' HOTKEY_CAPACITY_GIT_REVISION=3d9acccbc136195ee1c26fa7d9cec69cef2d1740 HOTKEY_CAPACITY_OUTPUT=../docs/acceptance/evidence/001/contents-keyset-capacity-macos-arm64-3d9acccb.json make capacity-fixture capacity-baseline
joint recovery: HOTKEY_RECOVERY_TEST_DSN='postgresql test DSN' HOTKEY_RECOVERY_MINIO_ENDPOINT='isolated MinIO endpoint' HOTKEY_RECOVERY_MINIO_ACCESS_KEY='test-only key' HOTKEY_RECOVERY_MINIO_SECRET_KEY='test-only secret' HOTKEY_RECOVERY_GIT_REVISION=3d9acccbc136195ee1c26fa7d9cec69cef2d1740 HOTKEY_RECOVERY_ENVIRONMENT=macos-26.6.2-local-postgresql-18.4-minio-isolated HOTKEY_RECOVERY_HARDWARE='Apple M5; 10 CPU; 24 GiB RAM; internal APFS SSD; PostgreSQL 18.4 and MinIO loopback' HOTKEY_RECOVERY_PRODUCTION_EGRESS_DISABLED=true HOTKEY_RECOVERY_OUTPUT=../docs/acceptance/evidence/001/joint-recovery-macos-arm64-3d9acccb.json make joint-recovery-acceptance
```

本机全量结果通过；前端 272 项测试通过，Python Agent 为 41 项测试通过且覆盖率 97.57%，生产依赖审计无高危漏洞；Go `govulncheck` 未发现当前调用链漏洞。远端运行 `33248525810` 的 8 个必需 Job 与最终汇总全部通过，其中 Worker 实际完成联合恢复。

## 未完成项与停止条件

- `CHK-001-G1-001`：等待完整四角色 UAT；
- `CHK-001-G2-001`：等待完整人工键盘/焦点矩阵，不以自动 Tab 与 WCAG 扫描替代；

以上任一项缺失时，本 Acceptance 不得改为 `passed`，001 Plan/PRD 不得改为 `completed/implemented`。

## 影响与回滚验证

- Schema、OpenAPI、生产 API 和启动拓扑均未变化；新增影响仅限后端测试工具、Make/CI 恢复门禁和两份净化后的验收报告。项目继续由根 Compose 运行，测试使用本机锁定工具链和可丢弃隔离恢复资产；
- 运行影响：联合恢复门禁只创建随机测试数据库、版本化测试 Bucket 和临时 Vault 目录，禁止生产出口并在成功/失败后清理；容量报告只读取固定测试库；不会停止或重启本机既有 6 个 Compose 服务；
- 回滚：若工具或证据失效，可回退联合恢复工具、Make/CI 步骤、本文件对应 EV 和 Plan 勾选；不得用回滚删除业务事实、对象、Vault 内容或任务历史，架构契约会在引用的测试、命令或门禁状态漂移时失败；
- 已知限制：本文件固定的代码基线早于记录本文件的提交；记录提交由同一 CI 再验证，但不以无法实现的“提交自引用哈希”冒充证据。

## 验收人

- Owner：HotKey Team；
- 自动化执行：Codex；
- 整体结论：failed（部分门禁通过，001 尚未完成）。
