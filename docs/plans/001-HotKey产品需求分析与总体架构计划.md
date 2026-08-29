---
layer: Plan
scope: shared
doc_no: "001"
title: HotKey 产品需求分析与总体架构计划
status: in_progress
version: v1.0
owner: HotKey Team
canonical_path: docs/plans/001-HotKey产品需求分析与总体架构计划.md
design: docs/design/001-HotKey产品需求分析与总体架构设计.md
prd: docs/prd/001-HotKey产品需求分析与总体架构.md
---

# HotKey 产品需求分析与总体架构计划

## 当前实现与差距

### 当前实现

- 仓库已经是 Go 模块化单体、Next.js Web 工作台和根 Compose 编排，不是绿地项目；
- `backend/cmd/hotkey` 提供单二进制入口，`all`、`api`、`worker` 角色共享模块、配置和领域事实；
- PostgreSQL 是业务事实源，`backend/db/schema.sql` 是唯一 Schema；Redis、MinIO 和本地 Vault 分别承担短期协同、原始证据和知识投影；
- PostgreSQL `river_job` 与 Worker 提供持久任务、重试、幂等和恢复；
- HTTP 业务接口统一返回 `code`、`message`、`data`，成功码为 `0`；发布契约由 Swaggo 生成到后端运行时代码和 `docs/openapi/swagger.json`；
- 前端已经使用 Next.js App Router、生成 OpenAPI 客户端、Zustand 和现有 UI 组合组件；
- 身份、Monitor、来源、采集、内容、事件、智能、通知、报告、知识和运维模块已有大量实现与测试。

### 主要差距

- 新基线需要把“已实现事实”“目标行为”“候选能力”分开，不能把交付包中的分布式目标写成当前能力；
- 五个新 PRD 的 P0/P1、角色边界、候选容量和 24 周承诺需要统一冻结；
- 现有实现仍包含多 Provider、Go Codex CLI Adapter、Embedding 与向量路径，而新目标要求根目录 Python Agent 和无向量全文检索；该差距是受控替换项目，不是文档清理时可直接删除的遗留物；
- 四角色服务端契约、Analyst 资源所有权与兼容迁移已落地，并由真实 PostgreSQL/JWT 会话矩阵证明每次请求重读当前角色、禁用即时撤销且重新启用不复活旧会话；仍需完整四角色 UAT，不能用自动化证据冒充人工验收；
- 当前 Schema、OpenAPI、River、Compose 和测试入口必须成为后续四个计划的硬门禁；
- 候选性能指标尚需固定测试数据集、环境、采样方法和证据格式，不能提前宣称为已满足 SLO。

## 启动条件与阶段门禁

S01 现状审计和 S02 评审准备允许在 Design=`proposed`、PRD=`draft` 时启动，它们正是形成 G0–G2 决策证据的工作，不以预先批准自身为前提。阶段门禁如下：

1. 启动 S01 前只需确认评审 Owner、决策记录位置、非生产验证环境和当前仓库读取权限；不得借现状审计修改产品行为；
2. 进入 S03 工程实施前，001 Design 必须为 `accepted`、PRD 必须为 `approved`，十条 AC 文义稳定；未批准项保持 `planned`，不能以审计结果代替批准；
3. 交付包只作为需求输入，不作为删除代码、改变架构或扩大范围的执行指令；
4. 当前仓库的后端、前端、Compose、Schema 和 OpenAPI 基线命令必须在 S01 至少运行一次并记录结果；
5. G1/G2 必须批准且约束根目录 `agent/` Python 分析服务，同时明确排除第二业务后端、Kafka、Temporal、其他微服务、Elasticsearch、Keycloak、`migrations/` 与新向量/RAG 能力；
6. 任何既有能力移除都有后继方案、双轨验证、回滚入口和单独验收，不以“目标不需要”为由直接删除。

## 阶段、依赖与出口

| 阶段 | 周次 | 依赖 | 主要工作 | 阶段出口 |
|---|---:|---|---|---|
| S01 现状审计 | W1 | 无 | 盘点运行角色、模块、Schema、OpenAPI、River、前端页面、Compose、测试和当前能力 | G0：当前/目标/差距矩阵可复核 |
| S02 范围与架构冻结 | W1–W2 | S01 | 冻结 P0/P1、角色、事实源、依赖方向、禁用架构、降级和候选指标 | G1–G2：Design accepted、PRD approved |
| S03 工程门禁固化 | W2 | S02 | 用架构测试、契约测试、Schema 检查和仓库检查保护基线 | G3：002–005 可按共同规则开工 |
| S04 追踪与发布基线 | W2，持续更新 | S02–S03 | 建立 AC→TASK→SPEC→CHK→Acceptance 映射和里程碑出口 | 每个 P0 AC 无孤儿映射 |

002 依赖本计划完成 M0；003 依赖 001 与 002；004 依赖 001–003 的稳定事实；005 贯穿所有计划并在 W19–W24 收敛发布证据。

## 预计变更文件清单

以下仅表示实施时可能检查或修改的路径，不授权本计划立即改代码：

```text
AGENTS.md
README.md
docs/README.md
docs/TEMPLATE.md
docs/design/001-HotKey产品需求分析与总体架构设计.md
docs/prd/001-HotKey产品需求分析与总体架构.md
docs/plans/001-HotKey产品需求分析与总体架构计划.md
backend/cmd/hotkey/main.go
backend/internal/bootstrap/
backend/internal/platform/http/
backend/internal/platform/queue/
backend/internal/platform/database/
backend/db/schema.sql
agent/pyproject.toml
agent/src/hotkey_agent/
agent/tests/
agent/Dockerfile
backend/openapi/docs.go
backend/test/architecture/
backend/test/tools/validate-architecture.sh
backend/test/tools/validate-repository.sh
backend/test/tools/verify-schema.sh
docs/openapi/swagger.json
frontend/src/app/
frontend/src/services/hotkey/hotkey-server/
frontend/test/unit/lib/repository-layout.test.ts
frontend/test/unit/lib/openapi-generation.test.ts
docker-compose.yml
docker-compose-prod.yml
.github/workflows/ci.yml
```

## 测试先行任务

### S01：建立可复测现状

1. `TASK-001-S01-T01`：先运行仓库、架构、Schema、OpenAPI、前端和 Compose 基线，记录任何失败及其是否为既有问题；逐项标注代码/契约当前事实、Design/PRD/Plan 目标状态和 Acceptance 已验证状态，不得把目标或失败基线误写成已实现结果。映射 `AC-001-001`、`AC-001-003`、`AC-001-005`。
2. `TASK-001-S01-T02`：为单二进制角色、依赖方向、唯一 Schema、生成 OpenAPI、唯一文档树及“只有 Acceptance 能证明完成”的状态真实性补充或收紧失败测试，再做最小规则修复。映射 `AC-001-003`、`AC-001-005`。
3. `TASK-001-S01-T03`：对身份角色、页面菜单、服务端 RBAC、资源所有权和直接 API 调用建立四角色夹具；Analyst 只能管理自有 Monitor、扫描与反馈，审核和管理动作必须负向拒绝。映射 `AC-001-002`、`AC-001-004`。

### S02：冻结产品和技术范围

1. `TASK-001-S02-T01`：把完整 P0 用户故事拆为登录、Monitor、来源、证据、事件、通知、日报、Vault 和全文检索的端到端测试故事，并为每一步定义事实源和失败状态；日报逐句 ClaimEvidence 覆盖及 Vault 自动区域更新不覆盖人工区域必须是硬断言。映射 `AC-001-001`。
2. `TASK-001-S02-T02`：增加仅允许根目录 `agent/` Python 分析服务，并禁止其持有业务存储凭据/公网端口，同时禁止第二业务后端、Kafka、Temporal、其他微服务、Elasticsearch、Keycloak 和 `migrations/` 的仓库结构或依赖断言；只有测试能够捕获错误引入后才固化规则。映射 `AC-001-003`。
3. `TASK-001-S02-T03`：为统一 Result、无数据 `data: null`、错误脱敏和生成 OpenAPI 建立契约测试，禁止新增 `request_id` 等未批准顶层字段。映射 `AC-001-005`。
4. `TASK-001-S02-T04`：为正常、空、加载、错误、权限不足和移动端状态建立前端测试矩阵，并覆盖全键盘操作、焦点顺序/可见焦点、语义化标签、对比度和 `prefers-reduced-motion`；公共组件变化先保存失败测试。映射 `AC-001-004`。
5. `TASK-001-S02-T05`：为关键写操作建立未认证、越权、无效输入、固定幂等键重复/重放、过期资源版本和并发冲突 Fixture；断言拒绝发生在副作用前、重复请求复用结果且不重复写事实、过期版本返回稳定冲突，成功/拒绝/冲突均追加净化审计。映射 `AC-001-009`。
6. `TASK-001-S02-T06`：为每个 P0 列表建立不可变排序字段+唯一 ID 的游标契约，使用并列排序、并发新增、连续翻页、无效/过期/越权游标和页大小边界 Fixture，断言已越过区间不回流、同一遍历无重复或漏项且错误脱敏。映射 `AC-001-010`。

### S03：固化非功能与降级边界

1. `TASK-001-S03-T01`：为候选容量和时延定义固定数据规模、并发、硬件、预热、采样次数、P95 算法和输出格式；同时在隔离环境从一致备份恢复 PostgreSQL、MinIO、Vault 与持久任务，完成事实/Hash/人工区域/任务对账并测量真实 RPO/RTO；先建立可重复基准和恢复证据，不先宣称达标。映射 `AC-001-006`。
2. `TASK-001-S03-T02`：建立 Python Agent 不可用、单来源失败、Redis 短暂不可用、MinIO 写失败和 Vault 不可写的故障矩阵；用可解析但越权的 Agent 建议证明分析服务只能返回建议，必须经过 Go Application/Domain 校验和授权人工治理，无法直写业务事实、权限或最终状态。映射 `AC-001-007`。
3. `TASK-001-S03-T03`：为 PostgreSQL FTS 路径补充无向量/无 RAG 的架构检查与搜索契约测试；现有 Embedding 路径仅标记为待迁移，不在此任务删除。映射 `AC-001-008`。

### S04：追踪和评审

1. `TASK-001-S04-T01`：逐项校验五份 PRD 的 FR/NFR→AC→Plan TASK/SPEC/CHK 覆盖率，P0 覆盖率必须为 100%。映射 `AC-001-001` 至 `AC-001-010`。
2. `TASK-001-S04-T02`：评审预计变更路径、数据/契约顺序、灰度和回滚；无法明确所有者的风险进入阻塞清单而不是静默开工。映射 `AC-001-003`、`AC-001-007`。

## 实施规格（SPEC）

| SPEC ID | 规格条款 | 上游 AC |
|---|---|---|
| `SPEC-001-API-001` | 所有业务 JSON 响应仅含 `code`、`message`、`data`；成功 `code=0`，无数据为 `data:null`；错误由全局处理器脱敏。 | `AC-001-005` |
| `SPEC-001-API-002` | OpenAPI 只由 Swaggo 注解生成到 `backend/openapi/docs.go` 与 `docs/openapi/swagger.json`，前端不得手写接口路径或 DTO。 | `AC-001-005` |
| `SPEC-001-API-003` | 关键写入必须在 Application 边界依次认证、授权、校验，并使用业务幂等键或资源版本前置条件；重复提交不重复副作用，过期版本返回稳定冲突，成功/拒绝/冲突追加净化且不可覆盖的审计事实。 | `AC-001-009` |
| `SPEC-001-API-004` | P0 列表使用不可变排序字段与唯一 ID 构成稳定游标和顺序，页大小有界；并发新增不回流到已越过区间，无效、过期或越权游标返回稳定脱敏错误。 | `AC-001-010` |
| `SPEC-001-DATA-001` | `backend/db/schema.sql` 是唯一数据库结构事实源；新增结构必须向前兼容并通过空库、非空库和兼容性检查，不创建迁移目录。 | `AC-001-003`、`AC-001-005` |
| `SPEC-001-JOB-001` | 异步工作使用 PostgreSQL River 表和 Go Worker；任务参数只传稳定 ID/版本/幂等键，处理时重读事实。 | `AC-001-003`、`AC-001-007` |
| `SPEC-001-AGENT-001` | 根目录 `agent/` 使用 Python 实现无状态数据分析服务；Go Worker 通过内部认证的版本化 HTTP 契约提交有界上下文，Agent 不持有 PostgreSQL/Redis/MinIO/Vault/来源/用户凭据，不直接写业务事实。 | `AC-001-003`、`AC-001-007` |
| `SPEC-001-UI-001` | Next.js 页面同时覆盖桌面/移动与正常、空、加载、错误、权限不足状态，使用生成客户端和现有设计令牌；关键流程必须支持键盘、可见焦点、语义化标签、合理对比度及 `prefers-reduced-motion`。 | `AC-001-004` |
| `SPEC-001-SEC-001` | 身份沿用现有 JWT、会话和服务端 RBAC；来源只允许官方 API、RSS、Atom 或授权 Feed；Python Agent 仅返回结构化建议，经 Go Application/Domain 校验及授权人工治理后才能影响当前视图，不能直写事实、权限或最终状态；错误与日志不得泄露凭据。 | `AC-001-002`、`AC-001-007` |
| `SPEC-001-OBS-001` | 候选容量和时延只有在环境、数据、命令、原始结果和统计方式齐全时才可作为验收证据；候选 RPO/RTO 只有在隔离环境联合恢复 PostgreSQL、MinIO、Vault 和持久任务并完成对账后才能成为承诺。 | `AC-001-006` |
| `SPEC-001-OPS-001` | P0 运行拓扑保持 Go Core、内部 Python Agent、PostgreSQL、Redis、MinIO、Vault 与根 Compose；Agent 不发布宿主机端口，禁用基础设施不进入启动图。 | `AC-001-003` |
| `SPEC-001-OPS-002` | 任何既有 Provider、Embedding 或向量结构的退出必须先完成替换验收、数据影响审计、灰度和回滚演练，删除另行评审。 | `AC-001-007`、`AC-001-008` |
| `SPEC-001-OPS-003` | P0 主故事必须沿正式模块边界完成登录、Monitor、采集、证据、事件、通知、日报、Vault 和检索；每一步状态、事实身份和 Evidence 引用均可追踪，每个事实性报告句有允许的 ClaimEvidence，自动知识更新不得覆盖人工区域。代码/契约记录当前事实，Design/PRD/Plan 记录目标与执行状态，只有 Acceptance 记录已验证完成。 | `AC-001-001`、`AC-001-003`、`AC-001-008` |

## 执行检查清单（CHECKLIST）

- [x] `CHK-001-G0-001`（`AC-001-001`）：已保存当前 P0 主故事的代码、Schema、API、页面、测试与真实缺口矩阵，并确认日报逐句 Evidence 与 Vault 人工区域保护的专项入口；证据：001 Acceptance `EV-001-001`。该基线完成不代表真实来源驱动的 `AC-001-001` 整体 UAT 已通过。
- [ ] `CHK-001-G1-001`（`AC-001-002`）：四角色权限矩阵已由产品与后端共同评审，Analyst 迁移语义、所有权和审核边界显式；当前工程证据覆盖 Schema/Domain/HTTP/OpenAPI/生成客户端、自有/他人 Monitor 负向测试及 `TestFourRoleSessionLifecycleUsesCurrentRoleAndNeverResurrectsRevokedSessions` 的角色/会话生命周期矩阵，仍需完整四角色 UAT 后勾选。
- [x] `CHK-001-G1-002`（`AC-001-003`）：Design 明确拒绝禁用基础设施，反例架构测试可捕获 Kafka、Temporal、第二 Python 后端和迁移目录误引入；代码/契约现状、Design/PRD/Plan 目标状态与 failed/partial Acceptance 未相互冒充；证据：001 Acceptance `EV-001-002`。
- [ ] `CHK-001-G2-001`（`AC-001-004`）：PRD 覆盖工作台五态、移动端、权限不足、键盘、可见焦点、语义标签、对比度和 `prefers-reduced-motion`；预期证据：页面状态、自动可访问性和人工键盘测试矩阵。
- [x] `CHK-001-G3-001`（`AC-001-005`）：Result、OpenAPI、唯一 Schema 和生成客户端的单一事实源无冲突；证据：本机 `make ci`、前端 `openapi:check`、远端全绿门禁及 001 Acceptance `EV-001-003`。
- [ ] `CHK-001-G3-002`（`AC-001-006`）：容量/性能测试和 PostgreSQL/MinIO/Vault/持久任务联合恢复的环境、数据、命令、统计法、对账与 RPO/RTO 测量均已冻结；预期证据：基准报告、恢复时间线和零未解释差异报告模板。
- [ ] `CHK-001-G3-003`（`AC-001-009`）：关键写操作的未认证/越权、幂等重放、旧版本和并发冲突矩阵在副作用、事实计数与审计上闭合；预期证据：并发集成测试、稳定错误码、写前后事实计数和追加审计查询。
- [ ] `CHK-001-G3-004`（`AC-001-010`）：并列排序和并发变化下连续游标遍历无回流、重复或漏项，页大小及无效/过期/越权游标均受控；日报、微事件 Evidence、微事件榜单、全文检索结果、运维任务、审计日志、内容/热点、Monitor、Document Match、来源连接、Rights Policy/Decision Batch、采集运行、采集条目工作、相关性匹配、反馈建议、Users 与 Knowledge Documents/Proposals 列表子项已由 `EV-001-006` 至 `EV-001-021` 证明；等待 Monitor Scans/Versions 补稳定游标，并判定 AI Model Profiles、Retention Policies、Source Presets 的固定有界参考集契约或补齐同类矩阵。
- [x] `CHK-001-G4-001`（`AC-001-007`）：来源、Agent、Redis、MinIO 和 Vault 降级均已记录“事实继续/暂停/重试/人工介入”结论；Python Agent 只提交建议，越权 Evidence/状态在 Go Application/Domain 写入前拒绝，5 类 Agent 故障为零 Claim 写入；证据：001 Acceptance `EV-001-004`。
- [x] `CHK-001-G4-002`（`AC-001-008`）：P0 全文检索只使用 PostgreSQL FTS/`pg_trgm`/权限重检，旧向量能力仍在受控迁移清单；自动知识区域可重建，人工区域只能从当前 Vault、受保护 Revision 或批准备份逐字恢复；证据：001 Acceptance `EV-001-005`。

## 验证命令

```bash
cd backend
go run ./test/runner vet ./...
go run ./test/runner test ./... -count=1
go build ./...
sh test/tools/validate-architecture.sh
sh test/tools/validate-repository.sh
sh test/tools/verify-schema.sh
sh test/tools/verify-database-runtime.sh

cd ../frontend
npm ci
npm run openapi:check
npm run typecheck
npm run test:unit
npm run build

cd ../agent
python -m pip install -e '.[dev]'
python -m ruff check .
python -m mypy src
python -m pytest
python -m pip_audit

cd ..
docker compose -f docker-compose.yml config --quiet
docker compose --env-file .env.prod -f docker-compose-prod.yml config --quiet
git diff --check
```

涉及 OpenAPI 时还必须运行仓库 CI 中的 Swaggo 生成命令并确认 `backend/openapi/docs.go`、`docs/openapi/swagger.json` 与生成客户端无漂移。实际执行结果只写入同编号 Acceptance，不在本 Plan 预填“通过”。

## 数据与契约顺序

1. 先冻结 PRD 行为、错误和 AC；
2. 先保存能够复现差距的失败测试；
3. 数据变更先修改 `backend/db/schema.sql`，再同步数据库兼容性模型和 Schema 测试；
4. 领域与 Application 规则通过后再修改 Transport 注解；
5. 生成 OpenAPI，再生成并检查前端客户端；
6. 最后修改 UI 和 Compose/CI，运行全量回归；
7. Acceptance 保存实际结果、环境、限制和回滚验证。

## 灰度、迁移与回滚

- 本计划本身不执行数据迁移或删除，只建立后续计划的兼容顺序；
- 角色变更先服务端双读/兼容旧角色，再更新前端菜单，最后在证据充分时收紧旧授权；
- Python Agent、全文检索和其他替换路径必须支持按配置或模型 Profile 灰度，保留旧 Provider/Codex 路径直到替换验收完成；
- Schema 只做向前兼容的增加与约束收紧准备，不自动删除列、表、扩展或历史事实；
- 若新基线导致 P0 主故事、权限、Result、OpenAPI、Schema 或启动拓扑回归，立即停止后续计划，将 Design/PRD 恢复到最近 accepted/approved 版本，并通过现有代码路径回滚；
- 文档回滚不能把已经发生的实现事实改写为“从未存在”，偏差由 Acceptance 或后继 Design 记录。

## 完成定义（DoD）

- 001 Design=`accepted`、PRD=`approved`，本 Plan 可进入 `in_progress`；
- 十条 AC 均有 TASK、SPEC 和 CHECKLIST 映射，没有孤儿 P0 需求；
- 当前/目标/差距、P0/P1、角色、事实源、依赖方向、降级和禁用架构已冻结；
- 当前事实、目标状态与 Acceptance 已验证状态未相互冒充；核心 UI 可访问性、联合恢复及 AI 建议边界均有可执行证据入口；
- 架构、Schema、OpenAPI、前端和 Compose 基线可复测，既有失败已单独记录；
- 002–005 的依赖、里程碑出口、候选指标和回滚边界与本计划一致；
- 没有删除 Provider、Embedding、向量数据或其他既有实现；
- 同编号 Acceptance 在实施完成后保存实际证据，G6 通过前本 Plan 不得标为 `completed`。
