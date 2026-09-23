# HotKey Server 交接

更新日期：2026-09-23。

## X 官方 API 离线接入 S01a

2026-09-23 用户将 X 自动采集从网页登录/twscrape 改为官方 API；目前尚无开发者 App/Token，要求先离线实现。002 S01a 已在 `sources/adapters/x_api.py` 加入仅接收 `MockTransport` 的 Recent Search 只读适配器，覆盖 recency/relevancy、分页、作品字段、合法空和认证/限流/协议失败，复用共享 SourcePage；旧 `x_twscrape.py` 不注册 Worker，也不读取现有浏览器登录。15 项模拟 HTTP 测试、后端全量 227 passed/161 skipped（未设置集成/live 测试条件）、Ruff/format/mypy 通过；没有真实 X 请求、额外服务或脚本。

002/005/037 及 BACKLOG 已登记费用例外：X 的 App/Token 和控制台账期美元上限未确认，037 当前数据库仍拒绝 paid 核心组件，因此不能挂入 Worker、发起付费请求或把 002/005/037 产品 AC 标为通过。下一步依序完成 037 的精确付费门禁与最坏费用预留、004/034 的秘密连接、002 的真实搜索/作者/回复能力及 005 处理器。其他来源和基础分析继续免费/自建；不自动充值或切换网页登录路径。

## 本地网页与浏览器采集 S01 / S02 / S03-T00/T01、T02 局部

已建立 [047 Research](docs/research/047-本地网页与浏览器采集调研.md)、[047 Design](docs/design/047-本地网页与浏览器采集设计.md) 与 [047 Plan](docs/plans/047-本地网页与浏览器采集计划.md)。Design 已接受 S00/S01/S02 网页业务闭环及 S03-T00/T01 浏览器基础，Plan 为 in_progress，G1/G2/G3/G4-001 已关闭，产品 AC 仍为 0/8。`web` 已具备无凭据版本、精确允许域名、collector_call、类型化 API→Outbox→Kafka→Worker→Firecrawl→webpage 原子结果与恢复；现有内容页可提交 URL，任务页可取消、手动重试、显示部分状态并打开持久资料。用户明确 B站、小红书、抖音、微博评论均必需，后续按四平台独立完成评论路径。

调研发现小红书 0 值配置文档与源码不一致、微博候选缺楼中楼继续分页、opencli B站评论仅单页；MediaCrawler 的置顶漏采已修复但许可证限制仍需遵循，Nemo2011/bilibili-api 已关停。S03-T00 在相同受控页面比较 Scrapling 0.4.15 与直接 Playwright：两者均可展开并保留字符串 ID，但 Scrapling 动作失败/超时仍返回 200、自适应误认另一评论 ID，默认记录完整 URL。选用 Playwright 1.63.0 原生 WS 单浏览器拓扑，不引入 Scrapling/Selector/CDP。S03-T01 已构建非 root、sandbox、只读、内部 WS 且默认断网的 browser；Worker 一次性容器的无网络探针成功，公网/宿主数据库端点不可达。Linux-arm64 Docker Desktop 证据不代替生产主机或真实平台验收。

S01 已增加独立 `page_content` 文档契约、精确域名/默认端口目标约束、固定 `firecrawl/2.11.162` HTTPX 适配器、配置及 DDL 能力同步；`python -m cli sources probe-webpage` 只做显式、无持久化诊断，不输出目标和正文。独立 Firecrawl `main` 的 `6d9fb16` 完成中央日志脱敏、Playwright 原始请求日志移除和出站目标加固，只原位恢复既有两个容器并复用宿主依赖。HotKey CLI 的普通页 180 字符、动态页 1574 字符、私网拒绝和日志 query 标记 0 命中通过。S02 已将类型化 `webpage.collect` 接入真实 Kafka/Worker/Firecrawl，调用前锁定租约、连接/域名、政策/保留和预算，调用后结算 collector attempt，并把正文/观察/发现、生命周期、能力证据和 checkpoint 原子提交；任务状态暴露 `result_content_id`。真实 PostgreSQL 用例覆盖消息重放、换版、限流、未知 kind、partial、结算前与页提交后中断恢复，显式 live Kafka 链实际读取当前 Firecrawl 的 `example.com`。内容页新增 URL 表单，任务页可取消、手动重试、显示部分状态并打开结果；桌面/390×844、双击单请求、operation ID 重试、CSRF 请求头和 axe 0 violation 已验证。此前后端全量 310 tests、前端 35 tests 及静态/契约/构建通过。尚无会话、真实平台评论证据或 Acceptance。

S03-T02 的受控动态交互与离线状态子片已用现有断网 browser 内网通过 2 项 live 测试：内存页面输入、滚动、展开、有限页结束、无限分页显式上限；状态在临时 0700 目录以 owner/连接/版本不可变保存并显式加载到新 context，未传状态的新 context 不继承 Cookie/localStorage。状态存储单测覆盖 0600 文件、符号链接/宽权限/跨 owner 拒绝；没有永久平台秘密、新服务或真实平台请求。

S03-T02 连接执行门禁又增加 `browser_state` DDL/ORM 精确引用约束与事务内当前版本、停用、认证失效、缺文件拒绝。在现有 PostgreSQL 的一次性空库应用完整 `schema.sql` 后，真实集成及后端全量 320 passed/2 skipped，Ruff/mypy 通过；库随验证删除并确认临时库 0 个，既有开发/测试库未改动。此实现仍无平台目录、人工登录/换版 CLI 或业务任务调用，不能当作真实会话可用。

S03-T02 又为每次 BrowserContext 交互设置默认及最大 45 秒截止；现有 browser 内网 live 测试增至 3 项，超时、主动取消后页面关闭且可重新建 context。一次性 Worker 客户端持有页面时 browser 渲染进程为 1，`SIGKILL` 后回到 0，browser 保持 healthy，测试容器自动移除；这只证明本机渲染进程回收，不证明业务任务崩溃恢复。计时不包含连接/清理。人工登录导出拟复用 Playwright 官方 `codegen --save-storage`，仍未接入真实平台或人工登录采集命令。

S03-T02 现有 `browser_state` 连接维护 CLI 已落地：可从 0600 捕获文件换版，或按当前版本停用；文件/父目录权限、符号链接、当前版本、重复停用、再启用及输出脱敏由受控测试覆盖。复用既有 PostgreSQL 的一次性空库执行后端全量 325 passed/3 skipped，Ruff/mypy 通过，临时库删除；未改现有开发/测试库或启动第二套依赖。CLI 不创建首个真实平台连接，未执行平台人工登录、业务 Worker 或产品验收；G4-002/EV-047-005、AC 0/8 不变。

S03-T02 又将 context 创建纳入原有 45 秒协作式截止，关闭 context/连接分别以 5 秒等待预算请求取消，前者超时仍尝试后者。失败优先单测、现有 browser 内网 live 超时关闭与重连、一次性空库后端全量 328 passed/3 skipped、Ruff/mypy 通过；未启动第二套依赖。Python `wait_for` 可能等待吞没取消的协程，且 WS 建连/管理器退出与业务任务总时限未覆盖；G4-002/EV-047-005、产品 AC 0/8 仍未关闭。

S03-T02 再将 WS 建连纳入同一次默认/最大 45 秒协作式截止；旧实现的慢连接失败测试、11 项浏览器单测、现有 browser 内网 3 项 live 和一次性空库后端全量 329 passed/3 skipped 通过，Ruff/mypy 通过，测试库删除。Playwright 管理器进入/退出、关闭与业务 Job 总时限仍未覆盖；真实平台登录、出站边界、G4-002/EV-047-005 和产品 AC 0/8 不变。

S03 控制面已从公开根路径切换到本机私有、含 48 个随机十六进制字符的 `/ws/` 路径：根 Compose 将同一 URL 注入现有 browser 与一次性 Worker 客户端，Python 配置以 `SecretStr` 脱敏，browser 缺配置/弱路径拒绝启动。原位重建后 CLI 探针、旧路径拒绝及现有代理/动态交互/状态/取消共 5 项 live 通过；一次性空库后端全量 329 passed/5 skipped、Ruff/format/mypy 通过，库已删除。密钥仅在忽略 Git 的 0600 根 `.env`，未启动第二套依赖服务。真实平台会话、业务任务总截止/崩溃恢复与 G4-002/EV-047-005、产品 AC 0/8 仍待后续。

S03-T02 管理器启动现与 WS/context/交互共用 45 秒协作式截止，退出单独以 5 秒请求清理；即使前面关闭失败，也尝试停止本地 Playwright 驱动。两个失败优先单测、13 项浏览器单测、现有 browser/代理 5 项 live、一次性空库后端全量 331 passed/5 skipped 及 Ruff/format/mypy 通过，临时库删除。此限时不抗取消吞没，也不是业务任务硬截止；真实平台会话及 G4-002/EV-047-005、产品 AC 0/8 仍未完成。

047 S04/008 评论所需的 [010 增量与历史回补 Design](docs/design/010-增量更新与历史回补设计.md) 已接受来源无关的内部契约，Plan 保持 in_progress，S00 G0—G2 与 S01 G3/G4 内部门禁已关闭：`coverage_windows` 的 owner/范围唯一、部分缺口、连续确认及最近 Job 复合外键已与唯一 DDL/ORM 同步；`ContentService.persist_post_in_transaction` 允许受控分页中的作品观察、发现、范围状态和 Job checkpoint 同一事务。两轮重叠得到 1 个作品、2 条发现、2 次观察；前窗未确认时水位不越过缺口，回滚不留进度。一次性空库 342 passed/5 skipped 和静态门禁通过，临时库已删除；无真实多页来源样本、API/UI 或既有开发库迁移，不设置通用 10 分钟重叠或旧帖刷新周期。产品 0/6 AC、Acceptance 均未变化。

010 S02 时间标记子片已推进：范围 Job 必须显式标识 `new_scan|refresh|backfill`，旧 Job 类型缺失时保守返回 `null`；作品发现详情通过运行时 OpenAPI/生成客户端显示历史回补标签，原始发布时间与本次观察时间继续分列。隔离 PostgreSQL 全量 343 passed/5 skipped，前端 36 tests、静态/构建及桌面/390px 受控响应（axe 0 violation）通过；本轮未修改 DDL、未迁移现有开发库、未新建依赖服务。受控浏览器响应不代表真实来源；游标有限重扫、实际回补和通知抑制尚未实现，S02 G3/G4 与产品 0/6 AC 仍未关闭。

031 S03 预研确认：宽泛捕获 Worker 处理器异常并确认消息会破坏已有 Kafka 重投/检查点恢复；尝试已撤回，原恢复用例重新通过。只有明确分类的永久来源/解析错误可持久隔离；毒消息和 72 小时执行率仍需逐类期限、证据及真实运行，031 G3/G4 和产品 AC 保持未通过。

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
- 业务领域目录不提前创建空包；当前实际领域为 `identity`、`jobs`、`sources`、`evidence`、`backups`、`monitors`、`connections`、`content`，其余 `ai`、`audit` 等候选主责遵循 `PROJECT.md`，具体切片落地时再创建实际文件。
- `python -m worker` 是 Kafka Worker 入口。已有 outbox 发布、手动 offset、inbox、租约/检查点恢复的运行装配；尚无具体业务消息处理器时安全退出，不订阅或提交任何消息。处理器只能在对应任务设计完成后注册。
- `python -m cli` 是 Typer 管理入口。`tests/unit`、`tests/integration`、`tests/architecture` 分别承载规则、HTTP 契约和依赖边界验证。
- `app/identity/` 已实现单 owner 初始化、Argon2 密码散列、服务端不透明会话、CSRF、注销和维护恢复；`python -m cli identity reset-password` 从隐藏交互输入读取新密码并撤销全部旧会话。
- 身份 HTTP 契约为 `/api/identity/initialize`、`/api/identity/sessions` 与 `/api/identity/session`；Web 请求层自动为写请求补 CSRF，请求凭据和 Cookie 不进入生成客户端参数。
- `GET /api/identity/workspace` 从有效会话派生当前 owner，不接受客户端归属标识；`require_resource_owner` 为后续业务资源提供默认拒绝规则。Web 已有 `/login` 与受保护 `/events` 空工作台，尚未接入事件业务资源。
- `app/jobs/` 已实现内部持久受理与恢复：任务与 `job.accepted.v2` outbox 同事务写入，owner/kind/operation ID 唯一，绑定非敏感配置版本与来源能力，等价重试返回原任务，异范围重用拒绝；outbox 收到 Kafka 回执后才标记，消费者在数据库事务后手动提交 offset，inbox、lease epoch、attempt 和连续 checkpoint 防止重投与旧执行者覆盖，调度追赶默认最多 3 个窗口。阶段尝试与任务/Worker/资源尝试汇总可按 operation 核对；尚无 HTTP、具体业务处理器、有限重试/死信、错误/陈旧问题或 72 小时度量。
- `app/evidence/` 已实现内部来源访问与在线生命周期控制：owner/source/capability 唯一政策、原子换版、入库前白名单投影；结构化/原始/媒体保留取用户请求与来源上限的更严值，缩期立即作用于已追踪资源；删除或到期后默认拒绝读取，`python -m cli lifecycle cleanup-once` 以 PostgreSQL lease 和有限重试清理 Redis/MinIO 在线副本。`app/backups/` 复用证据 DTO 生成同快照数据库候选归档和 MinIO 引用清单；已有 X 受控适配器、Firecrawl 网页适配器/显式 CLI 与 `webpage.collect` 业务处理器，当前本地 env 已启用既有 Firecrawl。网页 UI、MinIO 内容备份、真实恢复/回补和社交平台授权证据仍未完成。
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

**[035 计划](docs/plans/035-权限与数据隔离计划.md) S00/S01 已完成，S02 当前资源先行输出通过，Plan 保持 in_progress。** 在既有工作区授权上增加主题/任务越权操作矩阵、作品关联失败整笔回滚、连接证据隔离、删除目标引用不展开、混合证据清单整体拒绝及注销后拒绝。修复统一错误响应缺少 no-store 的缓存边界。真实 PostgreSQL 全量 243 tests、前端 22 tests、静态/契约及隔离构建通过；现有 API 已替换为当前代码，原 Web 代理确认禁存头与 request ID 透传，未新增服务/脚本。完整 S02 的评论/事件、实际缓存/异步发布/导出下载及 S03/S04 随业务接入，产品 AC 仍为 0/6，未建立 Acceptance。

**[031 计划](docs/plans/031-可靠执行与幂等计划.md) S00—S02 已完成，Plan 保持 in_progress。** 原子受理、outbox 重发、消费组中断/再均衡、手动 offset、inbox、租约 fencing、checkpoint 恢复、有限调度追赶和 Redis 不可用已通过本机既有 PostgreSQL 18.4 与 Kafka；后端 51 tests 与静态门禁通过。047 已在该底座上登记首个固定 `webpage.collect` 处理器，并验证真实 Kafka 重投与检查点恢复；其他业务 kind、S03/S04、72 小时运行及 031 产品 AC 仍为 0/6，未建立 Acceptance。

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

**[004 计划](docs/plans/004-平台与连接管理计划.md) S00—S03 内部技术闭环已完成，Plan 保持 in_progress。** `connections` 已实现 X/抖音目录、连接/不可变版本/追加式证据三表、准入政策与当前版本投影、manual/scheduled 独立状态、能力查询和连接配置/替换/启停 API/UI。S02 的 probe/persisted read 分立机制经 S03 收紧为必填执行版本；更换/停用拒绝迟到新增，原事实重放仍幂等。维护者只在服务端配置凭据，浏览器不输入/回读秘密；停用和当前版认证失效阻断新任务/人工重试。受控证据、真实 PostgreSQL 并发及浏览器闭环通过，最新 266 后端/25 前端测试和运行证据见下文。没有真实连接/探测/采集或业务 Worker，产品 AC 仍为 0/6，未建立 Acceptance。

**[007 计划](docs/plans/007-作品资料与上下文计划.md) S00—S03 已完成，Plan 保持 in_progress。** `content` 已交付唯一作品、发现、不可变指标/正文版本、完整度与来源标签、quote/repost、追加式可见性、编辑历史、乱序稳定投影和 PostgreSQL 生命周期清理；只读 API、生成客户端和列表/详情已闭环。S03 提交 `f3d704ad`，技术验证记录见 Plan；QA 后开发/测试库均 30 表 0 行。没有稳定原生 ID 的链接仍只作线索；S04、真实来源及产品 0/6 AC 仍待执行，未建立 Acceptance。

**[002 计划](docs/plans/002-X免费采集与热点监控计划.md) S01 受控技术切片已完成，Plan 为 in_progress。** `sources/adapters/x_twscrape.py` 固定 twscrape 0.20.1，复用 SDK 常量/解析/签名，由有界 HTTPX 统一管理初始化、分页、全部尝试计量、取消和故障停止；不调用默认账号池、隐式重试/轮换及解析落盘。27 项专用测试、后端全量 226 tests、Ruff/format/mypy 通过；应用关闭传输层敏感诊断，CDN 不带会话。复用既有 env 和 PostgreSQL/Redis/Kafka/MinIO；无新 HTTP、DDL、页面、脚本、服务或实际 X 请求。S00 真实准入、S02 的持久预算/连接租约/业务装配及 S03/S04 尚未完成，X 仍未就绪，产品 AC 为 0/10，未建立 Acceptance。

后端采用模块化单体与按业务领域分组的分层结构，完整目录、文件职责、API 契约、事务和依赖方向固定在根目录 [PROJECT.md](PROJECT.md)；执行入口、实现门禁和验证命令见 [AGENTS.md](AGENTS.md#fastapi-目录与命名必须执行)。

1. 047 S02 网页业务闭环及 S03-T00/T01 浏览器基础已完成：固定 Playwright Python/Server 1.63.0 原生 WS，browser 默认断网且非 root/sandbox，暂不引入 Scrapling/Selector/CDP。S03-T02 受控动态动作、离线状态文件、连接执行门禁和已有连接维护 CLI 已局部验证；下一步仍需真实平台登录/连接初始化、业务任务取消/崩溃恢复及平台出站安全验证。不得把内部无网络探针当作四平台评论或产品 Acceptance，产品 AC 仍为 0/8。
2. 保持旧 PostgreSQL 数据库不变；当前 `hotkey_dev` 已按完整 schema 重建，后续存量变更继续采用新库建表与校验导入，不增加运行时迁移。
3. 在业务表和任务接齐后执行 042 S02—S04 的完整 B0、高水位、共同负载、两环境恢复与回滚验证。
4. 按业务切片实现页面并完成桌面、窄屏和端到端验收。

## 检查

后端在 `backend/` 执行 `uv run ruff format --check .`、`uv run ruff check .`、`uv run mypy` 和 `uv run pytest`。前端执行 `pnpm test`、`pnpm lint`、`pnpm typecheck`、`pnpm format:check` 和 `pnpm build`；运行中的后端配合 `pnpm openapi:check` 校验生成漂移。产品进度以 `BACKLOG.md` 和对应 Acceptance 为准。

## 本轮产品文档复核

2026-09-21 先基于 HEAD `9093ed47` 静态复核工程，随后从 `37064d2a` 执行 046 与 042 S00/S01。BACKLOG 已补完整交付内容、跨计划批次、平台扩面及 App 队列；046 技术前置 8/8 AC 已通过，042 运行底座切片已通过，但所有产品 AC 仍未通过，业务流程、完整容量/恢复和验收仍待完成。

本轮已交付 009/003/007/004 S00—S03 和 002 S01，技术证据见上文与各 Plan。验证复用当前 PostgreSQL/Redis/Kafka/MinIO 与 env，未启动第二套依赖；002 S01 收尾后后端 226 tests、前端 22 tests、静态检查、OpenAPI/客户端无漂移及隔离生产构建通过。构建另发现既有路由辅助函数非法导出，已以独立修复提交保留内部函数并新增导出集合断言。尚未执行真实业务处理器/来源、真实连接探测/采集、上游扩词、跨任务来源预算/租约装配、真实评分/模型记录、独立对象备份/真实恢复、完整 B0 或产品 Acceptance。
