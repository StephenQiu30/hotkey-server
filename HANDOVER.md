# HotKey Server 交接

更新日期：2026-09-22。

## 当前结构

- `backend/`：Python 3.12、FastAPI、SQLAlchemy 2、PostgreSQL、Redis、Kafka；底座可运行，依赖由 uv 锁定，数据库 DDL 由单一事务化 SQL 文件管理。
- `frontend/`：Next.js App Router、shadcn/ui、Radix UI、Tailwind CSS、Axios；工程可构建。
- `hotkey-app/`：独立 Flutter 客户端仓库。

前端页面位于 `src/app/`。页面专属组件放在所属路由的 `components/`，跨页面复用组件按功能领域放在 `src/components/<feature>/`，shadcn 组件放在 `src/components/ui/`。不使用 `features`、`common`、`patterns`、`shared` 或 `scripts` 目录。

## 前端基础

- `src/request.ts` 是唯一 Axios 请求封装，统一处理凭据、超时、响应数据和错误；业务 HTTP、网络、超时、取消及协议错误保持可判别。
- Umi OpenAPI 读取后端自动生成的 `/openapi.json`，生成文件直接写入 `src/api/`；命令环境变量 `HOTKEY_OPENAPI_URL` 可覆盖默认地址。
- `src/proxy.ts` 处理页面 CSP nonce，并对缺少会话 Cookie 的 `/events` 做乐观登录跳转；`src/app/api/[[...path]]/route.ts` 负责同源 `/api/*` 转发及可控 502/504 错误契约，API 仍是认证和授权边界。
- 页面采用组件优先的无边框设计，只使用 Tailwind 命名尺度及 `sm/md/lg/xl/2xl`。
- App Router 已配置 loading、error、global-error、not-found 和 `/health`。
- Dockerfile 定义 standalone、非 root 用户和健康检查；根 Compose 已用只读文件系统及 tmpfs 实际验证。

## 后端基础

- `app/main.py` 是唯一 FastAPI 应用工厂，lifespan 管理 SQLAlchemy Engine 与 Session 工厂。
- `app/api/` 统一管理路由、依赖、中间件、异常与文档；提供 `/api/health`、数据库 `/api/ready`、`/openapi.json`、`/docs` 和 `/scalar`。
- `app/core/` 管理 `HOTKEY_` 配置、结构化日志、公共错误和 Pydantic 基类；`app/db/` 管理唯一 DeclarativeBase、Engine、Session 和运行时模型元数据。
- `database/schema.sql` 是唯一数据库结构事实源，仅用于全新空库。项目没有 Alembic、revision 目录或应用启动建表逻辑。
- 业务领域目录不提前创建空包；当前实际领域为 `identity`、`jobs`、`sources`、`evidence`、`backups`、`monitors`、`connections`，其余 `ai`、`audit` 等候选主责遵循 `PROJECT.md`，具体切片落地时再创建实际文件。
- `python -m worker` 是 Kafka Worker 入口。已有 outbox 发布、手动 offset、inbox、租约/检查点恢复的运行装配；尚无具体业务消息处理器时安全退出，不订阅或提交任何消息。处理器只能在对应任务设计完成后注册。
- `python -m cli` 是 Typer 管理入口。`tests/unit`、`tests/integration`、`tests/architecture` 分别承载规则、HTTP 契约和依赖边界验证。
- `app/identity/` 已实现单 owner 初始化、Argon2 密码散列、服务端不透明会话、CSRF、注销和维护恢复；`python -m cli identity reset-password` 从隐藏交互输入读取新密码并撤销全部旧会话。
- 身份 HTTP 契约为 `/api/identity/initialize`、`/api/identity/sessions` 与 `/api/identity/session`；Web 请求层自动为写请求补 CSRF，请求凭据和 Cookie 不进入生成客户端参数。
- `GET /api/identity/workspace` 从有效会话派生当前 owner，不接受客户端归属标识；`require_resource_owner` 为后续业务资源提供默认拒绝规则。Web 已有 `/login` 与受保护 `/events` 空工作台，尚未接入事件业务资源。
- `app/jobs/` 已实现内部持久受理与恢复：任务与 `job.accepted.v2` outbox 同事务写入，owner/kind/operation ID 唯一，绑定非敏感配置版本与来源能力，等价重试返回原任务，异范围重用拒绝；outbox 收到 Kafka 回执后才标记，消费者在数据库事务后手动提交 offset，inbox、lease epoch、attempt 和连续 checkpoint 防止重投与旧执行者覆盖，调度追赶默认最多 3 个窗口。阶段尝试与任务/Worker/资源尝试汇总可按 operation 核对；尚无 HTTP、具体业务处理器、有限重试/死信、错误/陈旧问题或 72 小时度量。
- `app/evidence/` 已实现内部来源访问与在线生命周期控制：owner/source/capability 唯一政策、原子换版、入库前白名单投影；结构化/原始/媒体保留取用户请求与来源上限的更严值，缩期立即作用于已追踪资源；删除或到期后默认拒绝读取，`python -m cli lifecycle cleanup-once` 以 PostgreSQL lease 和有限重试清理 Redis/MinIO 在线副本。`app/backups/` 复用证据 DTO 生成同快照数据库候选归档和 MinIO 引用清单；尚无 HTTP、来源/业务内容适配器、MinIO 内容备份、真实恢复/回补或平台授权证据。
- 本机既有 PostgreSQL 数据库包含旧系统历史表，不符合当前完整 schema。不得对这些旧库执行 `database/schema.sql`；需要保留数据时先备份，再用新库完整建表并校验导入。

## 运行基线

- 根 `compose.yaml` 是唯一编排，固定 PostgreSQL 17.11、Redis 7.2.16、Kafka 4.1.2；API/Web 仅绑定本机端口，内部依赖不发布宿主端口。
- 当前工作站的未跟踪 `backend/.env` 与 `frontend/.env.local` 直接复用已启动的 Homebrew PostgreSQL 18.4、Redis、Kafka 和 MinIO，不再启动第二套 Compose 依赖。当前代码使用同一 PostgreSQL 服务内的新空库 `hotkey_dev`；旧 `hotkey`、`hotkey-server` 与 `hotkey_test` 数据库未改动。部署镜像仍以本文固定的 PostgreSQL 17.11 为基线，本机 18.4 验证不能替代部署态版本验证。
- `schema.sql` 自带事务边界，通过 PostgreSQL 官方初始化目录仅作用于全新空卷；没有初始化 `.sh`、迁移框架或第二份 DDL。
- `.github/workflows/runtime.yml` 构建镜像并验证真实依赖、API/Web/同源代理、空 Worker、资源快照及部署态 OpenAPI 漂移。
- 验证不增加 `scripts/` 工具文件；复用 Compose、依赖官方 CLI、curl、docker stats 与现有 pnpm 命令。

## 待完成

**[046 前置计划](docs/plans/046-全局异常与响应契约前置计划.md) 已完成并通过 Acceptance。** 未知异常请求标识、5xx/校验信息泄漏、OpenAPI 错误模型、Web 错误读取、同源代理失败及生成客户端漂移门禁均已修复和验证。后续接口继续复用该契约；B00 的外部来源条件继续登记，但不阻塞 B02 内部领域切片。

**[042 计划](docs/plans/042-容量与部署可重复性计划.md) S00/S01 已完成，B01 底座前置已关闭。** 本地隔离 Compose 验证五个长期服务 healthy，API/Web/代理/空 Worker 和部署态客户端生成通过；该结果不代表完整 B0、两干净环境、恢复或 042 的 0/6 产品 AC 已通过。

**[034 计划](docs/plans/034-凭据与应用安全计划.md) S00/S01 已完成，Plan 保持 in_progress。** 全新隔离 PostgreSQL/Compose 已验证受控初始化、登录/注销、CSRF、旧会话失效、维护恢复和日志不泄密；后端 34 tests、前端 11 tests 及全量门禁通过。尚无受保护业务资料、连接秘密、正文、导出或网络目标，034 产品 AC 仍为 0/6，未建立 Acceptance。

**[035 计划](docs/plans/035-权限与数据隔离计划.md) S00/S01 已完成，Plan 保持 in_progress。** owner/外部主体规则、受保护工作区 API、生成客户端、登录/注销与事件空状态已通过真实 PostgreSQL、桌面和 390×844 浏览器验证；浏览器验证发现并修复动态 CSP nonce 与静态页面冲突。作品、事件、任务、导出、缓存、撤权与共享尚未接入，035 产品 AC 仍为 0/6，未建立 Acceptance。

**[031 计划](docs/plans/031-可靠执行与幂等计划.md) S00—S02 已完成，Plan 保持 in_progress。** 原子受理、outbox 重发、消费组中断/再均衡、手动 offset、inbox、租约 fencing、checkpoint 恢复、有限调度追赶和 Redis 不可用已通过本机既有 PostgreSQL 18.4 与 Kafka；后端 51 tests 与静态门禁通过。当前没有具体业务消息处理器，Worker 按文档入口安全空闲退出；S03/S04、72 小时运行及 031 产品 AC 仍为 0/6，未建立 Acceptance。

**[036 计划](docs/plans/036-数据访问与生命周期计划.md) S00—S02 已完成，Plan 保持 in_progress。** 来源政策、字段最小化、从严保留与缩期、即时读取屏障、幂等删除、Redis/MinIO 在线清理和有限重试已通过本机既有 PostgreSQL 18.4、Redis 与 MinIO；专用 12 tests、后端全量 63 tests 及静态门禁通过。受控样本不代表任何真实平台已授权；没有 HTTP/UI/Worker 变化或真实业务对象接入，S03 的备份/回补和来源状态、S04 及 036 产品 AC 仍为 0/6，未建立 Acceptance。

**[037 计划](docs/plans/037-费用与资源约束计划.md) S00—S02 已完成，Plan 保持 in_progress。** `jobs` 领域已增加免费组件策略、副作用前尝试账本、分层持久窗口、原子预留、幂等结算/释放与耗尽延期；目标 23 tests、后端全量 86 tests 及静态门禁通过。本切片复用已启动的 PostgreSQL 18.4，未接入真实 SDK/HTTP/UI/Worker，不证明 SDK 内部重试、长时占用自动回收或免费业务闭环；S03/S04 及 037 产品 AC 仍为 0/6，未建立 Acceptance。

**[038 计划](docs/plans/038-可维护与可替换计划.md) S00/S01 已完成，Plan 保持 in_progress。** `sources` 领域已增加四类纯能力请求、统一作品/评论、显式缺失值与父链、不透明分页/水位、页状态/停止原因及结构化适配器端口；新增 5 tests、相关 23 tests、后端全量 92 tests 及静态门禁通过。本切片复用现有 `.env` 与已启动服务，未新增真实适配器、固定版本/许可、样本/探针、HTTP/UI/Worker 或 SDK；S02—S04 及 038 产品 AC 仍为 0/6，未建立 Acceptance。

**[039 计划](docs/plans/039-可观测与可运维计划.md) S00/S01 已完成，Plan 保持 in_progress。** `jobs` 领域已增加配置/来源运行上下文、四类阶段尝试与七类显示状态互斥汇总，Worker 临时绑定安全关联字段，`job.accepted.v2` 跨进程传递相同上下文；新增 5 tests、相关 43 tests、后端全量 97 tests 及静态门禁通过。本切片复用现有 `.env` 与 PostgreSQL 18.4，将 0 行 `hotkey_dev` 按完整 schema 重建为 17 表；未新增服务、依赖、HTTP/UI 或具体业务处理器，不证明错误动作、陈旧问题、维护审计或运维闭环，S02—S04 及 039 产品 AC 仍为 0/6，未建立 Acceptance。

**[027 计划](docs/plans/027-数据正确性计划.md) S00 已完成，Plan 保持 in_progress。** Design v1.0 冻结不透明来源身份、不可变观察/版本、零与未知、时间类型、父链缺失和跨领域职责，为 028 S01 提供 `EvidenceResource` 生命周期锚点边界；本切片只有设计与依赖核对，没有代码、DDL、HTTP/UI、环境或产品 Acceptance，来源具体作用域/单位、受控异常样本及 S01—S04 仍待执行。

**[028 计划](docs/plans/028-可追溯与可复现计划.md) S00/S01 已完成，Plan 保持 in_progress。** `evidence` 领域已增加不可变输入清单、subject/reference 比较集、方法版本、受限参数和稳定 SHA-256 指纹；同一 job/result kind 等价重试幂等返回，冲突重用拒绝，创建与读取继续受当前资源到期/删除屏障约束。复用现有 `backend/.env` 与已启动 PostgreSQL 18.4，将确认 0 行的 `hotkey_dev` 按完整 schema 重建为 19 表；专用 4 tests、后端全量 101 tests、12 个架构测试及静态门禁通过，测试后 19 表 0 行，既有 API `/api/ready` 为 200。未新增服务、依赖、脚本、HTTP/UI、真实评分或模型输出；S02—S04 与 028 产品 AC 仍为 0/6，未建立 Acceptance。

**[029 计划](docs/plans/029-时效与数据新鲜度计划.md) S00/S01 已完成，Plan 保持 in_progress。** `jobs` 已增加可空计划到期时间，定时窗口按 `window.end` 持久化；严格时间链区分计划等待、受理排队、内部准备、来源等待、处理和可见延迟，使用非负整数微秒，缺失保持未知，无时区或未来来源时间不生成负发现延迟。复用现有 `backend/.env` 与已启动 PostgreSQL，将确认 19 表、0 行的同一 `hotkey_dev` 按完整 schema 重建；专用 8 tests、后端全量 109 tests、12 个架构测试及静态门禁通过。未新增服务、表、依赖、脚本、HTTP/UI 或陈旧阈值；S02—S04、B0 与 029 产品 AC 仍为 0/6，未建立 Acceptance。

**[032 计划](docs/plans/032-备份与恢复计划.md) S00/S01 已完成，Plan 保持 in_progress。** 维护 CLI 已增加 `backup create-candidate`，复用 PostgreSQL 18.4 官方 `pg_dump`/`pg_restore` 与现有 MinIO SDK：数据库归档、19 表计数和证据引用基于同一导出快照，清单记录 schema/归档 SHA-256、对象 present/missing/deleted 三态及 `0700/0600` 权限；临时 passfile 不把密码放入 argv、环境或候选包。复用现有 `.env` 和已启动服务，专用 5 tests、后端全量 114 tests、12 个架构测试及静态门禁通过，未新增 DDL、依赖、Compose、脚本、HTTP/UI。MinIO 内容仍未复制，输出明确 `candidate`、`inventory_only`、`restore_verified=false`；S02—S04、独立介质、真实恢复、删除重放、B0 与 032 产品 AC 仍为 0/6，未建立 Acceptance。

**[009 计划](docs/plans/009-采集任务控制计划.md) S00—S03 已完成，Plan 保持 in_progress。** S03 已实现结构化失败、来源有限策略、PostgreSQL attempt/job/Inbox/Outbox 同事务延期、`dispatch_sequence`/`available_at`、严格联合消息、到期发布及 `POST /api/jobs/{job_id}/retry`。受控验证得到 delayed/delayed/failed、权限错误零自动重投、同一消息三次重放只转换一次、Outbox 到期只发布一次，手动重试重复点击幂等。详情页展示失败分类、稳定代码、下一动作/时间和重试入口；真实点击后同一 job 转 queued，次数 1→2，桌面/390×844 与未登录边界通过。继续复用现有 env 及 PostgreSQL/Redis/Kafka/MinIO，未新增依赖、服务、脚本或 `.sh`；QA 数据已清理。后端 132 tests、前端 15 tests、静态检查、OpenAPI 生成与生产构建通过，G3/G4 已勾选。真实处理器/来源、S04 与产品 0/6 AC 仍待执行，未建立 Acceptance。

**[003 计划](docs/plans/003-监控主题管理计划.md) S00—S03 已完成，Plan 保持 in_progress。** `monitors` 领域除本地规则和不可变版本外，已实现 owner UUID 游标列表、独立复制 v1/默认暂停、行锁启停/归档、ready 门禁、归档写保护，以及会话/CSRF 保护的纯本地规则预览。预览不回显标题样本、不写主题/版本/job、不调用来源；本地别名明确 0 查询/0 预算，上游查询和预算保持未知。浏览器验证发现并修复 Portal 内预览提交冒泡导致外层主题表单误提交。运行时 OpenAPI、生成客户端、工作台/详情及共用预览 Dialog 已闭环。后端 149 passed/4 skipped，前端 16 tests、静态检查、OpenAPI 漂移与生产构建通过；桌面/390×844 无横向溢出，axe 0 violation，修复后数据库为 0 主题/0 版本/0 job，QA 数据已清理。没有真实上游扩词、调度器、主题业务 job 或来源，单任务取消继续使用 009 入口；未新增依赖、DDL、服务、脚本或 `.sh`，产品 AC 仍为 0/6，未建立 Acceptance。

**[004 计划](docs/plans/004-平台与连接管理计划.md) S00—S02 已完成，Plan 保持 in_progress。** `connections` 已实现 X/抖音静态目录、连接/不可变版本/追加式能力证据三表、当前准入政策与当前连接版本投影、按 `manual`/`scheduled` 分开的状态、会话保护的 `GET /api/source-capabilities`、生成客户端和 `/sources`。S02 复用 Typer 增加显式 `connections record-probe`，采集用例通过领域 Service 登记 persisted read；两者自动绑定 owner 当前版本，等价 operation 幂等、异义冲突拒绝、入口不串扰。专用 17 tests、后端全量 166 passed/4 skipped、前端 16 tests、静态门禁、OpenAPI 漂移和生产构建通过；真实 Web 验证显示 probe 不放行、persisted read 仅放行 manual，390×844 无溢出且 axe WCAG A/AA 0 violation。QA 清理后开发/测试库均 24 表 0 行，普通 API/Web 已恢复。没有真实连接、来源适配器、真实探测/采集、写 API、Worker 或外部请求，产品 AC 仍为 0/6，未建立 Acceptance。

后端采用模块化单体与按业务领域分组的分层结构，完整目录、文件职责、API 契约、事务和依赖方向固定在根目录 [PROJECT.md](PROJECT.md)；执行入口、实现门禁和验证命令见 [AGENTS.md](AGENTS.md#fastapi-目录与命名必须执行)。

1. 按 B03 顺序进入 007 S00，先评审作品资料与上下文的范围、领域归属、不变式和切片证据；004 S03 属于 B04，等 B03 内容与来源前置就绪后再进入。003 的真实上游扩词、调度/主题任务留待 S04 联验，009 S04 等待真实处理器和来源样本。
2. 保持旧 PostgreSQL 数据库不变；当前 `hotkey_dev` 已按完整 schema 重建，后续存量变更继续采用新库建表与校验导入，不增加运行时迁移。
3. 在业务表和任务接齐后执行 042 S02—S04 的完整 B0、高水位、共同负载、两环境恢复与回滚验证。
4. 按业务切片实现页面并完成桌面、窄屏和端到端验收。

## 检查

后端在 `backend/` 执行 `uv run ruff format --check .`、`uv run ruff check .`、`uv run mypy` 和 `uv run pytest`。前端执行 `pnpm test`、`pnpm lint`、`pnpm typecheck`、`pnpm format:check` 和 `pnpm build`；运行中的后端配合 `pnpm openapi:check` 校验生成漂移。产品进度以 `BACKLOG.md` 和对应 Acceptance 为准。

## 本轮产品文档复核

2026-09-21 先基于 HEAD `9093ed47` 静态复核工程，随后从 `37064d2a` 执行 046 与 042 S00/S01。BACKLOG 已补完整交付内容、跨计划批次、平台扩面及 App 队列；046 技术前置 8/8 AC 已通过，042 运行底座切片已通过，但所有产品 AC 仍未通过，业务流程、完整容量/恢复和验收仍待完成。

本轮已交付 009 S00—S03、003 S00—S03 与 004 S00—S02：前者覆盖持久任务受理/读取/取消、分类失败、有限重试与任务页；003 覆盖本地主题规则、不可变版本、创建/编辑、列表、独立复制、启停/归档和零写入预览；004 覆盖连接领域、事实表、状态投影、只读 API/页面与分立证据登记。验证复用当前 PostgreSQL/Redis/Kafka/MinIO 与 env，未启动第二套依赖；004 S02 结束时后端 166 passed/4 skipped、前端 16 tests、OpenAPI 漂移、生产构建、390×844 真实浏览器及 WCAG A/AA 通过。尚未执行真实业务处理器/来源、真实连接探测/采集、上游扩词、SDK 内部计量、真实评分/模型记录、独立对象备份/真实恢复、最后成功/缺口/陈旧传播、003 S04、004 S03/S04、009 S04、完整 B0 或产品 Acceptance。
