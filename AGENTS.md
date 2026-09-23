# HotKey 工程规范

本文件适用于整个仓库。后端固定 Python、SQLAlchemy 2、FastAPI、PostgreSQL、Redis、Kafka；前端固定 pnpm、Next.js、shadcn/ui、Radix UI、Tailwind CSS、Axios、ESLint、Prettier。

`PROJECT.md` 是项目技术、架构、目录、API 契约和数据库事实源；本文件负责实现执行门禁、工具命令和验证要求。发生冲突时以 PROJECT.md 的架构决策为准。模块 README 记录使用方式，HANDOVER 记录实现状态；这些文件不得定义冲突的架构规则。变更架构或目录时必须先更新 PROJECT.md、对应 Design 和本文件，再修改代码。

实现前明确 Design、需求、Plan 和验收标准；完成真实验证后建立 Acceptance。正式文档使用 `docs/TEMPLATE.md`，编号登记在 `docs/README.md`。

## 任务开始前的目录与选型门禁

- 统一异常、响应和状态契约按 [046 Design](docs/design/046-全局异常与响应契约设计.md) 执行；除 [046 前置修复计划](docs/plans/046-全局异常与响应契约前置计划.md) 本身外，任何 Plan 进入实现前必须有 046 S03 的真实通过证据。范围/研究/设计可先准备；已有 in_progress 不豁免。此门禁不额外要求逐项用户批准普通实现细节。
- API 层映射无 HTTP 状态的应用错误；统一 ErrorView、类型化成功/分页/受理响应，禁止全局 body 改写。检查请求 ID、5xx/校验脱敏、必要头、运行响应与 OpenAPI 422 一致、生成客户端和非 JSON/网络/取消路径。不得用所有路由统一声明所有错误码、手改生成代码或宽泛测试忽略绕过检查。
- Web/App 不手写服务端 DTO，不按 message 判断状态，不在传输拦截器全局弹提示或自动重试写请求。Worker 的持久状态、重试和连续 offset 仍由 009/031 实现与真实验证，HTTP 契约通过不能替代。

- 每个实现切片开始前先明确技术选择与目录职责，列明新增、修改、移动和生成文件；不先写文件后找目录。
- 技术选择已有用户决定的直接沿用；影响本片的部署、费用、平台范围或新框架等未决项，先给出选项和影响询问用户，未答复不执行依赖该决定的工作。普通实现细节按已定规范处理，不重复确认已确定的技术栈。
- 用户已明确：复用现有MinIO作对象存储；不付费采购内容，采用免费或自建服务；X等主流信息平台优先发现，评论研究覆盖B站/微博/小红书/抖音。AI推理费用范围单独确认，不将内容预算解释成付费AI授权。
- 新后端模块、前端功能目录必须登记职责并纳入全源码/依赖检查；门禁重新实现并验证前，不得声称已覆盖新模块。
- 按业务切片创建目录，不提前创建空模块。来源适配器放sources/adapters，MinIO适配器放evidence/adapters，模型SDK适配器放ai/adapters；业务状态仍由业务模块持有。前端页面使用 `frontend/src/app`，页面专属组件放对应路由的 `components/`，跨页面复用组件按明确功能领域放 `frontend/src/components/<feature>/`，shadcn 基础组件放 `components/ui/`。不创建 `features`、`common`、`patterns` 或 `shared` 层；生成客户端固定在 `src/api`，Axios 封装固定在 `src/request.ts`。

- `backend/` 是唯一后端：Python 3.12、FastAPI、Pydantic、SQLAlchemy 2、PostgreSQL、Redis、Kafka。禁止恢复 Go 后端、独立旧 Agent 或兼容旧接口。
- `frontend/` 是唯一 Web 前端，采用 pnpm、Next.js App Router、React、TypeScript、Tailwind CSS、shadcn/ui、Radix UI、Axios、ESLint 和 Prettier。工作台入口由 `frontend/src/app/` 管理；公开产品页与 SEO 路由必须以真实可公开内容为基础，登录工作台使用 `noindex`，不可加入 sitemap。
- 品牌资产只保留唯一母版，页面图标通过 Next.js Metadata API 引用。
- Next 配置位于 `frontend/next.config.ts`，页面 CSP 使用 `frontend/src/proxy.ts` 的逐请求 nonce。浏览器对 `/api/*` 的请求保持同源，Compose 服务环境将 `HOTKEY_API_ORIGIN` 指向 `http://backend:8080`，本机开发默认 `http://127.0.0.1:8867`；容器内 Web 端口固定为 `8080`。生产镜像使用 standalone 输出和非 root 用户，生产文件系统保持只读。
- 独立客户端仓库固定为同级 `hotkey-app`，使用 Flutter + Dart；Web 只在本仓库 `frontend/` 实现。两个仓库各自维护根 PROJECT.md 与 HANDOVER.md。
- Web 依赖统一由 pnpm 管理，提交 pnpm-lock.yaml 并在 package.json 声明 packageManager；不混用 npm/yarn 锁文件。
- Web 设计固定为组件优先的无边框系统：路由组合页面组件、按功能领域分类的复用组件与 ui 组件，默认信息表面不用装饰性边框；输入、焦点、错误与浮层保留必要轮廓。布局只使用 Tailwind 命名尺度和 `sm/md/lg/xl/2xl` 响应式层级，禁止原始像素值和任意布局尺寸。前端不建立独立 `scripts/` 目录，使用 ESLint、TypeScript、Prettier、生产构建和代码审查维护这些约束。
- 每个前端切片必须在 Design 阶段列出组件名称、所属 feature、复用范围、目标路径、数据来源和状态覆盖。页面专属组件不得提前放入公共目录；只有至少两个页面存在稳定复用时才迁移到 `components/<feature>/`。
- 唯一 HTTP 契约由 FastAPI 路由装饰器、类型注解和 Pydantic 模型自动生成，通过 `/openapi.json` 提供；前端端点函数与类型全部由 `@umijs/openapi` 读取该地址生成。禁止手写契约 JSON/YAML、端点请求及生成类型。
- `docs/` 保存 Research、PRD、Design、Plan、Acceptance、Operations。历史实现从 Git 查询，不在工作树中归档。目标能力不得描述为已完成。
- 修改前阅读相关设计和测试。行为变化先验证失败，再实现；修复需针对实际故障验证。
- API 路由负责协议、认证和验证；业务服务负责事务；SQLAlchemy 模型负责持久化。禁止路由直接执行 SQL 或发布消息。
- Session 不跨线程或任务共享。同步数据库端点使用同步路由；各 API/Worker 进程独立拥有连接池和消息客户端，禁止跨进程继承连接。
- 业务服务只能直接导入本领域ORM模型；跨领域读取使用所属模块提供的函数/DTO，跨领域原子写显式传入同一Session。禁止为绕过边界建立全局repository或共享models目录。
- 顶层模块只在当前切片真实创建时登记；architecture测试不得预先白名单未来模块。新增模块必须先以失败测试证明未登记代码会被拒绝。
- `backend/database/schema.sql` 是唯一数据库 DDL 事实源；SQLAlchemy Model 只负责运行时映射。禁止 Alembic、revision 目录、`metadata.create_all`、应用启动建表和第二份 DDL。
- `schema.sql` 只用于全新空库，必须通过 `psql -X --set ON_ERROR_STOP=on --single-transaction` 原子执行。当前不支持存量库自动就地演进；需要保留数据时先验证备份，再新建数据库、应用完整 Schema 并导入校验后的数据。禁止对旧系统库直接执行。
- 业务状态与 Outbox 同事务提交。Outbox 发布到 Kafka，消费者在业务事务提交后提交连续完成位置的 offset，允许重投并通过消息 ID、epoch、fencing、租约和唯一约束保证幂等。Kafka 事务不等于与 PostgreSQL 的跨系统原子提交。Redis 只承担缓存、限流及可重建临时状态，不保存唯一业务事实；关键执行权以 PostgreSQL 为准。不再采用 RabbitMQ/Celery，不以 Redis 另建任务队列。
- 唯一运行编排为根 Compose。生产差异使用两个 `-f` 文件叠加，无第二套服务栈。不得删除用户持久卷。
- 认证信息不入日志或 Git；配置使用 `HOTKEY_` 前缀。公开错误只含稳定错误码、面向用户的消息和请求 ID；输入校验可附带脱敏字段详情，不回显敏感请求体。
- HTTP完成日志只记录request_id、方法、路由模板、状态码和耗时；禁止记录原始URL/query、请求/响应正文、Cookie、Token或连接字符串。未处理异常记录类型与堆栈，但不回显给客户端。
- 锁定依赖；运行 Ruff、mypy、pytest、OpenAPI 漂移检查及前端类型检查/构建。数据库和消息行为必须用真实 PostgreSQL/Redis/Kafka 验证，UI 必须用浏览器验证。
- 只采集公开或获授权数据；平台连接器必须明确能力、分页、限流和失败状态。禁止把模拟数据、空结果或诊断任务当成采集成功。
- 不擅自提交、推送或删除远端引用；用户明确授权后，先检查差异、敏感信息和远端状态，再按下列 Git 规范提交。

## Git 提交规范与交付

- 提交标题格式固定为 `<type>(<scope>): <subject>`。`scope` 必填，使用稳定的小写英文模块名（如 `api`、`jobs`、`docs`、`ci`、`repo`）；`subject` 使用简体中文动宾短语，冒号后保留一个空格，标题不超过 72 个字符。
- `type` 限用 `feat`、`fix`、`test`、`refactor`、`docs`、`chore`、`perf`、`build`、`ci`、`revert`。示例：`feat(api): 新增任务状态查询`。
- 提交正文和脚注使用简体中文；正文说明变更摘要、原因和实际验证结果。避免“更新代码”“修复问题”等无具体信息的描述。
- 不兼容变更使用 `<type>(<scope>)!:`，并在脚注用 `BREAKING CHANGE: <中文迁移说明>` 记录影响和迁移方式。
- 一个提交只包含一个可独立验收的任务；生成物与对应源文件同提交，不混入无关格式化、重构或文档。
- 未经用户明确授权，不创建提交、推送、创建或合并 Pull Request。

## FastAPI 目录与命名（必须执行）

- 后端固定为模块化单体，按业务领域分组，采用 Router、Service、Schema、Model 分层。Repository 仅在查询复杂或需复用时增加，不创建通用 BaseRepository、ServiceImpl 或每层一套空接口。
- 后端切片在 Design 阶段明确领域归属、变更路径、路由与 DTO、服务入口、事务所有者、跨领域依赖及消息恢复行为；业务和目录规范确定后再创建模块。
- 最外层业务用例提交或回滚事务；依赖注入只管理 Session 创建与释放。跨领域原子写共用 Session，内层函数不自行提交。HTTP 和 Worker 各自装配服务，业务服务不依赖 HTTP 上下文。
- 后端工程及 Compose HTTP 服务均为 `backend`，作为普通应用运行，不构建独立安装包；禁止恢复 `server/` 别名。部署入口为 `main:create_app`。
- 应用代码统一放在 `backend/app/`；禁止在 app 下增加 hotkey 或 app 包装层；`main.py` 只做应用工厂和 lifespan 装配；`api/router.py` 汇总路由，`api/routers/*.py` 按资源组织，依赖和 HTTP 横切逻辑分别在 dependencies.py、middleware.py、exception_handlers.py。
- 已登记的 `identity/`、`monitors/`、`jobs/`、`connections/`、`content/` 领域在对应切片落地时拥有各自 models.py、schemas.py、services.py。models 定义 SQLAlchemy 持久结构，schemas 定义 Pydantic 契约，services 拥有事务和业务行为。禁止预先创建空领域包；也禁止用通用 Workspace/BaseService 聚合无关领域或为每个简单查询增加无意义仓储层。
- `core/` 只放配置、通用错误、输入输出基类和时间函数，不反向依赖业务模块。`db/` 拥有 DeclarativeBase、连接池及元数据注册；`audit/` 承载跨领域审计。`jobs/execution.py` 维护执行状态机，`worker/messaging.py` 对接 Kafka；`worker/app.py` 装配消费者生命周期，`cli/commands.py` 实现管理命令，`cli/__main__.py` 为命令入口。运行目录为 backend/app，拟定 Worker 入口为 `python -m worker`（由 `worker/__main__.py` 承接）；不得沿用 Celery 启动命令。
- 路由禁止导入 SQLAlchemy、业务 models、services 实现、执行器或消息组件；只能通过 `api/dependencies.py` 注入服务。禁止经 request.app.state 在路由中绕过业务服务读写数据库或发布任务。服务、模型、Schema 不导入 FastAPI/Starlette/HTTP 路由；Schema 不导入 ORM 或数据库资源。
- 每个HTTP操作必须有唯一人工`operation_id`、tag、成功状态和Pydantic响应模型；错误响应按操作显式声明，不在应用级虚报所有状态码。输入继承严格Input并给集合、字符串、页大小和正文设置上限。游标不得泄漏内部数据，应有明确的校验和分页边界。
- Python 文件、目录、函数使用 snake_case，类使用 PascalCase，常量使用 UPPER_SNAKE_CASE；同类职责文件统一使用 models.py / schemas.py / services.py。绝对导入；`__init__.py` 仅标识包或说明包，不放业务代码和重导出别名。
- 测试放 `backend/tests/unit/`、`backend/tests/integration/`、`backend/tests/architecture/`，公共 fixture 放 tests/conftest.py；不新增项目 `scripts/` 目录或一次性 `.sh` 文件，容器与部署验证复用 pytest、领域 CLI、Compose、依赖官方 CLI 和 CI 工作流。组件文件使用 kebab-case.tsx，导出组件使用 PascalCase；生成客户端固定在 `frontend/src/api/`，Axios 传输封装固定在 `frontend/src/request.ts`，不创建 features/patterns/shared 层，英文 README 为 README.en.md。
- 后端结构与依赖方向由 architecture 测试强制检查；前端 Next 层登记与依赖方向遵循本文件及 `frontend/DESIGN.md`，并通过 ESLint、TypeScript、生产构建和代码审查验证。Ruff 检查命名/绝对导入，mypy 严格检查后端应用与工具；锁文件、`schema.sql`、ORM 映射、HTTP 和消息契约必须在目录重构中保持可验证。增加架构例外需同步 Design，禁止添加宽泛忽略绕过标准检查。
- 不因“异步更先进”将同步psycopg调用放进`async def`路由。只有整条调用链非阻塞且有独立并发/连接池验证时才引入AsyncSession，并保证每个并发task独立Session。

- `sources/` 的适配器不依赖 API、ORM、Worker 或 CLI；业务来源契约不导入 HTTP 客户端。来源探测只经独立 CLI 显式执行，查询预览不发送网络请求。未通过持久化采集验收前，来源连接状态保持 not_connected。
- `sources/adapters/x_twscrape.py` 拥有固定 SDK 的只读映射与有界单会话传输；初始化与失败请求纳入计量，禁止 SDK 默认账号轮换、隐式重试和解析失败落盘。业务预算/连接执行权由调用方装配，不由适配器直接操作 ORM。
- 本地网页/浏览器采集按 [047 Design](docs/design/047-本地网页与浏览器采集设计.md) 与 [047 Plan](docs/plans/047-本地网页与浏览器采集计划.md) 分片推进；S00/S01 与 S02 公开网页业务闭环已接受，下一步从 S03 隔离浏览器运行时与会话继续。不得把公开网页闭环当作平台接入成功，或把 Firecrawl 内置渲染器当作完整交互服务。`connections/adapters/local_secrets.py` 只管理受控浏览器状态文件，运行时不得接收不可信文件路径；本地 CLI 捕获文件也需拒绝符号链接、宽权限及超限内容。`browser_state` 引用必须与版本行身份一致，执行前由 `connections` 服务判定当前版本、停用及认证失效，不由文件存在性代替；无适配器时不在目录新增假来源。外采适配器保持无 ORM，业务编排归 content，执行权和预算归 jobs；不新增第二套队列或任务数据库。跨仓库 Firecrawl 修复单独检查差异，不混入 HotKey 提交。
- S03 浏览器固定 Playwright Python/Server `1.63.0` 原生 WS；browser 构建 target、必要的 `server.js` 入口与 seccomp 置于 `backend/`。browser 只接 Worker 共享的 WS 内网及专用代理内网，只有 HotKey 自有 Squid 代理接公网桥接网；初始代理仅放行 `example.com` 探针，不放行真实平台。CLI 的无网络探针和代理通路仅证明运行基础，不得据此声明平台能力；真实平台出口/请求计量须经后续专门验证。不得安装 Scrapling、CDP 服务、第二套依赖栈或把浏览器二进制加入 API 镜像。
- B站、小红书、抖音、微博评论/回复均为已确认范围，按 [008 Design](docs/design/008-评论与回复采集设计.md) 分平台验证作品定位、线程关系、一级/楼中楼分页、采样缺口和旧帖新回复。候选爬虫先核对固定源码/许可/运行边界，不能照 README 的“全量”或单页结果声明完整；四平台评论不依赖 X 先就绪。

### 固定技术与运行入口

| 项目 | 固定要求 |
|---|---|
| HTTP 服务 | Python 3.12、FastAPI、Uvicorn |
| 契约与配置 | Pydantic 2、pydantic-settings；配置使用 `HOTKEY_` 前缀 |
| 数据访问 | SQLAlchemy 2、psycopg 3、PostgreSQL；默认同步 Session |
| 数据库结构 | `database/schema.sql`；唯一 DDL 事实源，只初始化全新空库 |
| 缓存与事件 | Redis 负责可重建状态；Kafka 负责持久任务事件 |
| 对象存储 | MinIO，适配器归 `evidence/adapters/` |
| 工具 | Ruff、mypy、pytest、HTTPX；依赖精确版本随锁文件提交 |
| API 入口 | 在 `backend/app/` 执行 `uvicorn main:create_app --factory` |
| Worker 入口 | 在 `backend/app/` 执行 `python -m worker` |
| CLI 入口 | 在 `backend/app/` 执行 `python -m cli` |

后端依赖统一使用 uv、`pyproject.toml` 和 `uv.lock`，作为普通应用管理，不构建安装包。CI 和镜像使用 `uv sync --locked`；精确版本在底座初始化时解析、验证并提交，不手工编辑锁文件。

### 通用工具与复用边界

| 能力 | 固定工具 | 放置与使用规则 |
|---|---|---|
| 配置校验 | `pydantic-settings` | `core/config.py`；禁止手写环境变量解析框架 |
| HTTP 请求 | `httpx` | 领域 adapters 使用复用的 Client/AsyncClient；明确连接、读取、写入和连接池超时 |
| 结构化日志 | `structlog` + 标准 logging | `core/logging.py` 统一配置，输出结构化日志，按请求绑定并清理 request_id |
| 有限重试 | `tenacity` | 只用于适配器中可安全重试的调用；明确异常类型、次数、时间预算和退避 |
| 命令行 | `typer` | `cli/commands.py` 定义命令，`cli/__main__.py` 启动；不自行解析 argv |
| Redis | `redis`（redis-py） | 复用官方连接池，设置超时与资源释放；业务缓存规则留在所属领域 |
| Kafka | `confluent-kafka` | `worker/messaging.py` 适配；确认投递结果，业务提交后提交 offset |
| 对象存储 | `minio` | `evidence/adapters/minio.py`；复用官方签名、上传和下载能力 |
| 密码哈希 | `pwdlib[argon2]` | 身份切片按需安装；不手写密码加密或哈希算法 |
| JWT | `PyJWT` | 仅在身份 Design 选定 JWT 后安装；不自行编码签名、解码或验证令牌 |
| 标准通用能力 | `datetime`、`zoneinfo`、`uuid`、`pathlib`、`contextlib` | 标准库能完成的功能直接使用，不建立重复工具类 |

- 工具选型固定，依赖随真实使用方加入；禁止为凑工具清单安装未使用的框架。
- 仅为业务契约、生命周期和外部服务差异做薄封装，不创建通用 HttpUtils、RedisUtils、BaseService 或万能工具包。
- 直接复用 FastAPI/Starlette 的依赖注入、异常处理、中间件、表单解析和响应序列化能力；不另造 Web 框架。
- Tenacity 不承担持久任务调度或消费重试状态；不得无条件重试写操作，避免 SDK 重试与应用重试叠加。
- 同步 SDK 不直接运行在异步路由中；连接池、HTTP Client 和消息客户端由所属进程生命周期统一创建与关闭。
- 日志禁止输出原始请求、响应、Token、Cookie 和连接字符串；外部异常先映射为业务错误，再交由 API 输出。

### 接口文档

- FastAPI 路由与 Pydantic Schema 是唯一契约源；运行时统一提供 `/openapi.json`，Swagger UI 使用 `/docs`。
- 增强交互文档默认使用 `scalar-fastapi`，入口 `/scalar`，与 Swagger UI 共用 `/openapi.json`；不额外开启 ReDoc。
- Knife4j 只有在明确要求该产品时作为替代 UI 接入，不能安装 Spring Boot starter 到 Python 后端。接入前验证实际 OpenAPI 版本、nullable、联合类型、认证与调试兼容性；不得只修改 Schema 版本号冒充兼容。
- 不手写第二份 Swagger JSON，不另用注解体系生成契约；文档 UI 和 Umi OpenAPI 客户端读取同一份契约。
- 接口变更只修改路由装饰器、参数类型、`Field`/`Query`/`Path` 声明和 Pydantic 模型。后端重载或重启后，FastAPI 自动更新 `/openapi.json`，Swagger UI 和增强文档刷新后展示新契约；不逐接口维护文档页面。
- Umi OpenAPI 的 `schemaPath` 直接指向后端 `/openapi.json`，通过命令环境变量 `HOTKEY_OPENAPI_URL` 覆盖。生成客户端需要执行 `pnpm openapi:generate`，不宣称后端变更会自动热更新 TypeScript 文件。
- 后端底座 CI 必须启动同一提交的应用，自动执行客户端生成和类型检查，并检查生成差异；契约不可读取或生成失败时构建失败。需要归档的 JSON 仅由程序导出为 CI 产物，不作为手工维护的源文件。
- `api/docs.py` 负责文档 UI 注册，由 `main.py` 装配；文档页面使用 `include_in_schema=False`，不得进入生成客户端。
- 端点必须填写中文 summary、必要 description、tag、operation_id、参数约束、成功和错误模型、适用示例及认证方式。
- Swagger UI 与增强文档的静态资源固定版本；生产文档在受控入口开放或关闭，调试功能沿用真实 API 权限。
- 文档验收包含 Schema 加载、分组、认证、参数输入、实际调试、错误展示和 Umi OpenAPI 生成；页面能打开不等于契约兼容。

### 后端目录标准

以下为目标结构；按实际切片创建文件。领域模块无持久化需求时不创建 models.py，无查询复用需求时不创建 repositories.py。Python 包必须有 `__init__.py`，图中省略。

```text
backend/
├── pyproject.toml                 # 依赖、Python 版本、检查和测试配置
├── uv.lock                        # uv 生成的唯一依赖锁文件
├── Dockerfile
├── database/
│   └── schema.sql                 # 唯一 PostgreSQL DDL 事实源
├── app/
│   ├── main.py                    # 唯一 create_app 与 lifespan 装配
│   ├── api/
│   │   ├── router.py              # 注册所有 HTTP 路由
│   │   ├── dependencies.py        # 身份、Session、服务的类型化注入
│   │   ├── docs.py                # 接口文档 UI 注册
│   │   ├── middleware.py          # 请求 ID 和访问日志
│   │   ├── exception_handlers.py  # 业务异常转 HTTP 响应
│   │   └── routers/<resource>.py  # 资源接口，不含业务实现
│   ├── core/
│   │   ├── config.py              # pydantic-settings
│   │   ├── logging.py             # structlog 与标准 logging 配置
│   │   ├── errors.py              # 与 HTTP 无关的异常
│   │   └── schemas.py             # 公共输入输出基类
│   ├── db/
│   │   ├── base.py                # 唯一 DeclarativeBase
│   │   ├── session.py             # Engine 与 Session 工厂
│   │   └── metadata.py            # 仅用于模型注册
│   ├── <domain>/                  # 按业务领域命名
│   │   ├── models.py              # 表、关系、索引和约束
│   │   ├── schemas.py             # 输入、输出和服务 DTO
│   │   ├── services.py            # 业务规则、资源权限、事务
│   │   ├── repositories.py        # 按需拆出的查询与持久化
│   │   └── adapters/              # 按需隔离外部 SDK
│   ├── worker/
│   │   ├── __main__.py
│   │   ├── app.py                 # 生命周期和服务装配
│   │   └── messaging.py           # Kafka 收发与提交位点
│   └── cli/
│       ├── __main__.py
│       └── commands.py
└── tests/
    ├── conftest.py                # 隔离环境与公共 fixture
    ├── unit/
    ├── integration/
    └── architecture/
```

| 领域目录 | 唯一业务主责 |
|---|---|
| `identity/` | 身份、账号及授权规则 |
| `monitors/` | 监控配置与规则 |
| `jobs/` | 任务、Outbox、执行状态机、取消与恢复 |
| `connections/` | 平台连接版本、能力证据与可用状态投影 |
| `content/` | 作品身份、发现关系、内容版本与观察投影 |
| `sources/` | 来源契约、来源适配器与采集能力 |
| `evidence/` | 证据元数据、文件与 MinIO 适配器 |
| `ai/` | 模型调用契约及 SDK 适配器 |
| `audit/` | 跨领域审计记录 |

新增领域必须先在切片 Design 登记主责、依赖和目标目录，再更新本表；不得把业务代码堆入 `core/`、全局 `utils/` 或全局 `models/`。

### 依赖、事务与资源边界

| 层 | 允许依赖 | 禁止事项 |
|---|---|---|
| Router | Schema、API 依赖别名 | SQLAlchemy、直接导入或构造 Service、发布消息 |
| API dependencies | Session 工厂、Service、身份依赖 | 业务编排、自动提交事务 |
| Service | 本领域 ORM/Repository、Schema、显式领域服务、适配器契约 | HTTP 上下文、直接访问其他领域 ORM、循环依赖 |
| Schema | Pydantic、标准类型、公共 Schema | ORM、Session、FastAPI |
| Model/Repository | SQLAlchemy、db 基类、本领域数据结构 | HTTP、调用上层 Service、独立 commit |
| Adapter | 外部 SDK、所属领域契约 | HTTP 路由、任务状态机、修改其他领域数据 |
| Worker/CLI | 业务服务、运行资源与装配 | 复制业务规则、调用 HTTP 路由实现 |

- 服务简单时使用函数；需要持有注入依赖时使用类。只为实际可替换边界定义 Protocol，不要求每个服务都配接口与实现类。
- 最外层用例显式开启并结束事务；跨领域写入由编排方传入同一 Session，内层只读写或 flush。独立任务重新取得 Session。
- Session 按请求或任务创建，不跨并发执行单元共享；请求依赖清理时仅释放或回滚未完成事务。响应 DTO 在 Session 有效期内构造，禁止响应序列化触发隐式数据库访问。
- Engine、连接池和外部客户端按进程初始化，由 API lifespan 或 Worker 生命周期释放；导入模块时不建立网络连接。
- 同步数据库使用 `def` 路由；异步 Worker 调用同步业务时，整个用例及 Session 生命周期在同一个受控执行单元内完成。不得在事件循环中直接调用阻塞数据库或 SDK。
- API 与 Worker 独立运行；FastAPI lifespan 不启动业务消费者。存活检查验证进程，就绪检查验证必需依赖；必需资源初始化失败必须阻止服务就绪。
- 数据写入和 Outbox 原子提交；Worker 幂等完成业务事务后再提交 offset。失败重投、死信、取消和恢复策略必须在任务 Design 中明确。

### 前后端契约与目录标准

```text
frontend/src/
├── app/
│   ├── page.tsx
│   ├── components/               # 根页面专属组件
│   └── <route>/
│       ├── page.tsx
│       └── components/           # 对应页面或路由树专属组件
├── components/
│   ├── ui/                       # shadcn/Radix 基础组件
│   └── <feature>/                # 按功能分类的跨页面复用组件
├── api/                          # Umi OpenAPI 生成文件
├── lib/                          # 无业务语义的纯工具
├── request.ts                    # 唯一 Axios 传输层
└── proxy.ts                      # 同源代理与 CSP
```

- HTTP 契约链固定为路由装饰器/类型注解/Pydantic → 运行时 `/openapi.json` → Umi OpenAPI → `frontend/src/api/` → `src/request.ts`。后端输入、输出 Schema 分离；响应不得暴露敏感字段。
- 每个端点显式声明稳定的 `operation_id`、tag、成功状态、响应模型和适用错误响应。客户端生成物与对应契约变更同批交付。
- 页面专属组件不得被所属路由树外部导入；需要跨页面复用时迁移至对应 `components/<feature>/`。不创建 `src/features`、`common`、`patterns`、`shared` 或前端 `scripts` 目录。

### 设计阶段必须交付的内容

| 内容 | 必须明确 |
|---|---|
| 文件清单 | 新增、修改、移动、生成文件的确切路径及职责 |
| 领域边界 | 主责模块、服务入口、允许依赖及跨领域调用 |
| 数据 | `schema.sql`、ORM、Schema、约束、事务所有者、重建与数据导入行为 |
| HTTP | 方法、路径、身份和资源权限、operation_id、响应与错误 |
| 任务 | 消息契约、幂等、offset、超时、重试、取消与恢复 |
| 前端 | 组件名称、分类、复用范围、路径、数据来源和各状态 |
| 验收 | 必要场景、验证命令、隔离依赖、完成条件及证据位置 |

无明确职责或没有当前使用方的文件不得提前创建。实现改变以上决策时，同步更新 Design 和本规范。

### 实施验收要求

- 后端初始化时配置 Ruff、严格 mypy、pytest 及 `app` 导入路径；建立架构测试约束实际模块和依赖方向。
- 单元测试验证业务规则；集成测试在全新隔离 PostgreSQL 中执行完整 `schema.sql`，验证 ORM 映射、HTTP/OpenAPI、事务、依赖释放及 Redis/Kafka 行为。
- 每次 DDL 变更必须以失败验证证明旧 `schema.sql` 不满足新结构，再在同一提交更新 SQLAlchemy Model、完整 SQL 和数据库断言。CI 从空库建表并验证，不读取开发机旧库，不接受仅靠 mock 或 SQLite 的结果。
- 数据库或消息改动必须验证回滚、重复消费和进程重启恢复；不能用 mock 通过代替真实集成验收。
- 前端运行 ESLint、TypeScript、Prettier 和生产构建；页面变更完成桌面与窄屏浏览器检查。
- 文档变更检查路径、命名和规则一致性。规划目录不代表代码已经存在，测试目标不代表已经通过。
