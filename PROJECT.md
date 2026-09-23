# HotKey Server 项目与技术选型

更新日期：2026-09-23。本文固定仓库边界、技术栈、后端目录、API 契约和运行约束。

## 1. 定位与仓库边界

HotKey 面向热点事件监控、作品与评论采集、事件时间线、讨论证据分析、提醒和报告。本仓库同时维护 Python 后端与 Web 前端；独立客户端仓库为同级 `hotkey-app`，固定 Flutter + Dart。两端消费同一 FastAPI OpenAPI 契约。

```text
HotKey/
├── hotkey-server/
│   ├── PROJECT.md
│   ├── HANDOVER.md
│   ├── AGENTS.md
│   ├── BACKLOG.md
│   ├── backend/          # Python API、后台执行与数据访问
│   ├── frontend/         # Next.js Web 工作台
│   └── docs/             # 调研、需求、设计、计划与后续验收
└── hotkey-app/            # 独立 Flutter 客户端仓库
    ├── PROJECT.md
    ├── HANDOVER.md
    └── AGENTS.md
```

`frontend/` 和 `backend/` 的目录与职责由本文固定。目录规划、测试目标和生成文件清单不代表业务能力已经实现；每个业务切片仍须经过 Design、实现和 Acceptance。

### 1.1 产品用户与交付结果

主要使用者是跟踪热点、研究评论和观察账号的信息研究者，首期按单人自建工作台安排。目标是让用户完成“配置主题或账号、持续发现作品、整理为事件、核查评论证据、形成分析、收到变化提醒并导出”的完整流程。平台覆盖数量和采集条数不单独作为产品成功依据。

产品需求的事实源为 [001 PRD](docs/prd/001-热点事件监控平台功能与非功能需求.md) 与同编号逐项 PRD；本文固定产品边界及技术约束，[BACKLOG](BACKLOG.md) 编排交付内容和顺序，[Plan 索引](docs/plans/README.md) 承载详细任务与验收映射，[HANDOVER](HANDOVER.md) 记录实现快照。优先级、范围和验收变化须同步上述文档，不能只改台账分母。

### 1.2 发布范围

| 交付层 | 内容 | 完成判断 |
|---|---|---|
| Web 首版（M0—M5） | 18 项 P0 功能与 16 项质量要求；X 加首批至少一个国内来源；基础统计、人工观点整理与证据分析 | 34 项全部必要验收、X 专项、真实来源闭环及部署/恢复/体验证据通过；当前均未完成 |
| 后续增强（M6） | 7 项 P1：热榜订阅、自动分析、媒体上下文、离线通知、模型配置及两类模型信息获取 | 独立价值和条件满足后逐项交付；增强关闭时基础流程可用 |
| 价值扩展（M7） | 2 项 P2：资料问答/比较、团队协作 | 需求与使用价值确认后单独验收 |
| 国内平台扩面 | B站、微博、小红书、抖音；首批择一，其余保留逐平台交付队列 | 每个平台分别证明关键词、账号作品、评论/回复、增量和失败状态；不以数量或 SDK 宣称支持 |
| Flutter App | 复用服务端需求与契约，目标平台和首批功能尚未冻结 | 独立 App 范围与设备验收；Web M5 通过不代表 App 完成 |

上述 P0/P1/P2 继承现有 PRD 草案，不因本轮文档修订变成已经批准的工期。X 必需、免费或自建、既有 MinIO、Python/Next.js/Flutter 的已定方向继续执行；只将确实未定的平台样本、设备和参数保留为开工输入。Web 首版与 App 独立验收，App 的未决平台不阻塞已确定的 Web 交付。

### 1.3 当前实现边界（2026-09-23 核对）

本轮从 server HEAD `37064d2a` 开始执行。已有后端应用工厂、健康/文档路由、Worker/CLI 入口、依赖锁、唯一根 Compose 及基础 CI；046 已补齐统一异常/响应契约、请求标识与脱敏、Web 传输分类、可控同源代理失败、运行时 OpenAPI 客户端生成和契约漂移 CI；042 S00/S01 已补真实 PostgreSQL/Redis/Kafka、空库初始化、资源上限和部署态运行门禁；034 S00/S01 已补单 owner、服务端会话、CSRF、注销及维护恢复，`schema.sql` 已登记身份和会话表；035 S00/S01 已补默认拒绝 owner 规则、受保护工作区 API、登录页及 `/events` 空工作台；031 S00—S02 已补持久任务、事务 outbox、手动 offset、inbox、租约/检查点恢复与有限调度追赶；036 S00—S02 已补来源准入、字段最小化、从严保留、即时读取屏障和在线清理；037 S00—S02 已补免费组件准入、全尝试计量、分层窗口、原子预留、幂等结算/释放与耗尽延期；038 S00/S01 已补纯来源能力请求、作品/评论、分页/水位、停止原因与结构化适配器端口；047 S00—S02 已补公开网页契约/目标约束、匿名连接、collector_call、固定 `webpage.collect` Worker、真实 Kafka/Firecrawl、原子持久结果及 URL 表单→任务→资料入口；039 S00/S01 已补任务运行上下文、阶段尝试、互斥状态与三类尝试汇总；027 S00 已冻结内容身份与不可变观察边界；028 S00/S01 已补不可变输入清单、比较参考集、方法版本、稳定指纹、幂等冲突和当前生命周期屏障；029 S00/S01 已补计划到期事实、严格时区/顺序校验、分阶段整数微秒和来源时间异常分类；032 S00/S01 已补同快照数据库候选归档、证据对象引用核对和秘密隔离。隔离浏览器与平台会话、四平台真实评论处理器、真实评分/模型记录、最后成功/延期/缺口/陈旧传播、维护审计、独立介质/真实恢复/回补清理、完整容量和产品 Acceptance 仍待交付。

046 的本地测试、生产构建、真实 Next/FastAPI 代理和桌面/窄屏浏览器验证记录在对应 Acceptance；它不证明数据库、消息、来源、部署或产品闭环。后续应扩展现有底座，不以旧的“空工程”描述重建第二套实现。

## 2. 固定技术栈

### Web 前端

**pnpm + Next.js + shadcn/ui + Radix UI + Tailwind CSS + Axios + ESLint + Prettier。**

| 技术 | 职责 |
|---|---|
| pnpm | 唯一 Web 包管理器；提交 `pnpm-lock.yaml`，在 `package.json` 中固定 `packageManager` |
| Next.js App Router + React + TypeScript | 页面、布局、交互、服务端代理与类型约束 |
| shadcn/ui + Radix UI | shadcn/ui 采用 Radix primitives 的组件方案，不切换其他底层组件实现 |
| Tailwind CSS + CSS Variables | 样式与统一设计令牌 |
| Axios | 集中处理请求、认证与错误；封装固定在 `frontend/src/request.ts` |
| `@umijs/openapi` | 从后端 OpenAPI 生成类型与端点函数至 `frontend/src/api/` |
| ESLint + Prettier | 代码检查与格式化 |

页面归 `frontend/src/app/`，页面专属组件放对应路由的 `components/`；跨页面复用组件按明确功能领域归 `frontend/src/components/<feature>/`，shadcn 基础组件归 `components/ui/`。不建立 `src/features/` 或含糊的 `common/patterns/shared` 层。浏览器调用同源 `/api/*`；`HOTKEY_API_ORIGIN` 仅供 Next.js 服务端代理使用。具体认证与错误契约由后端统一维护。现有根 DESIGN.md 是视觉研究参考；Web 执行规范为 frontend/DESIGN.md 及对应切片 Design，参考中的边框、原始尺寸或示例模块不构成实现要求。

Web 设计固定为组件优先的无边框系统：App Router 页面只组合页面专属组件、按功能领域分类的复用组件与 shadcn/Radix 基础组件；默认信息表面通过留白、排版和语义背景分层。布局只使用 Tailwind 命名尺度和 `sm/md/lg/xl/2xl` 标准响应式层级，不使用原始像素值或任意布局尺寸。输入、焦点、错误与浮层保留必要轮廓；加载、路由错误、全局错误、404 与进程健康状态都有统一边界。组件归属、复用范围、目标路径、数据来源和状态覆盖必须在对应切片 Design 阶段明确。

### Python 后端与基础设施

架构固定为模块化单体，按业务领域分组；Router 处理 HTTP，Service 处理业务与事务，Pydantic Schema 定义契约，SQLAlchemy Model 定义持久化。Repository 按需引入。完整目录、文件职责、依赖方向和事务边界由本文固定；AGENTS.md 负责把这些决策转成实现门禁和验证命令。

**Python + SQLAlchemy 2 ORM + FastAPI + PostgreSQL（PGSQL）+ Redis + Kafka。**

| 技术 | 职责 |
|---|---|
| Python 3.12 | 语言基线；应用源码直接位于 `backend/app/`，不增加 `app/app`、`app/hotkey` 或 `<package_name>` 包装层 |
| FastAPI + Pydantic | API、验证、错误契约及唯一 OpenAPI 源；不预先创建未定义的 API 版本目录 |
| Uvicorn + pydantic-settings | ASGI 运行与类型化配置 |
| psycopg 3 | PostgreSQL 驱动，默认使用同步 SQLAlchemy Session |
| SQLAlchemy 2 | 运行时 ORM 映射与事务；不创建或修改数据库结构 |
| `database/schema.sql` | 唯一 PostgreSQL DDL 事实源；只用于初始化全新空库 |
| PostgreSQL | 业务事实、权限、任务、进度、幂等记录与 Outbox 的持久存储 |
| Redis | 缓存、限流和可重建临时状态；关键权限、预算与任务状态仍有数据库依据 |
| Kafka | 任务事件与异步消息传输，由 Python Worker 消费 |
| MinIO | 复用既有对象存储，保存有权限与保留期约束的文件及证据 |
| Docker Compose | 根目录唯一运行编排；开发/生产差异通过配置叠加 |
| Ruff + mypy + pytest | 格式/静态检查、类型、单元/集成/架构验证 |
| uv | 依赖、虚拟环境与 `uv.lock`，按锁文件安装 |
| HTTPX + Tenacity | HTTP 客户端与有界重试 |
| structlog + Typer | 结构化日志与命令行 |
| redis-py + confluent-kafka + minio | Redis、Kafka、MinIO 客户端 |
| Swagger UI | `/docs` 交互文档，读取唯一 `/openapi.json` |
| scalar-fastapi | `/scalar` 增强交互文档，与 Swagger UI 共用契约 |

### 后端目录、职责与唯一事实源

后端采用按业务领域分组的模块化单体。以下是实现目标结构，Python 包目录中的 `__init__.py` 在图中省略；没有明确使用方的领域文件不得提前创建。

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
│   │   ├── router.py              # 唯一 HTTP 路由汇总点
│   │   ├── dependencies.py        # Session、身份和 Service 的类型化注入
│   │   ├── docs.py                # Swagger/Scalar 文档注册
│   │   ├── middleware.py          # request ID、访问日志等 HTTP 横切逻辑
│   │   ├── exception_handlers.py  # 全局异常到 HTTP 错误响应的映射
│   │   └── routers/<resource>.py  # 资源接口，只处理 HTTP 协议
│   ├── core/
│   │   ├── config.py              # pydantic-settings 配置
│   │   ├── logging.py             # structlog 与标准 logging 配置
│   │   ├── errors.py              # 不依赖 FastAPI 的应用异常
│   │   └── schemas.py             # 公共输入、输出和 ErrorView
│   ├── db/
│   │   ├── base.py                # 唯一 DeclarativeBase
│   │   ├── session.py             # Engine 与 Session 工厂
│   │   └── metadata.py            # 模型注册，不负责建表
│   ├── <domain>/                  # 按业务领域命名，不建立全局 models/utils
│   │   ├── models.py              # SQLAlchemy 持久化映射
│   │   ├── schemas.py             # 领域输入、输出和服务 DTO
│   │   ├── services.py            # 业务规则、权限和事务边界
│   │   ├── repositories.py        # 仅复杂或复用查询需要时增加
│   │   └── adapters/              # 仅外部 SDK 或服务差异需要时增加
│   ├── worker/
│   │   ├── __main__.py            # python -m worker 入口
│   │   ├── app.py                 # Worker 生命周期和服务装配
│   │   └── messaging.py           # Kafka 收发与位点提交
│   └── cli/
│       ├── __main__.py            # python -m cli 入口
│       └── commands.py            # 管理命令
└── tests/
    ├── conftest.py
    ├── unit/
    ├── integration/
    └── architecture/
```

图中的 `<domain>` 和 `<resource>` 只是目录职责的表示法，不是要创建的字面目录；每个实际名称必须在对应 Design 中明确登记。

目录规则如下：

- `main.py` 只创建 FastAPI 应用、注册 lifespan、路由、中间件和异常处理器；不放业务规则。
- `api/routers/` 只处理 HTTP 参数、认证依赖、状态码和响应模型；不得导入 SQLAlchemy、业务 Service 实现、Worker 或消息客户端。
- 领域 Service 负责业务用例和事务；跨领域原子写入使用同一 Session，内层函数不得自行提交。
- Schema 不依赖 ORM、Session 或 FastAPI；Model 只负责持久化映射；Adapter 只封装外部系统差异。
- `worker/` 和 `cli/` 调用领域 Service，不复制 HTTP 层或业务规则。
- 不创建未定义的 API 版本目录、`app/app/`、`app/hotkey/` 或其他没有明确职责的包装目录。

### API 契约与版本策略

FastAPI 路由装饰器、类型注解和 Pydantic 模型是唯一可编辑的 API 契约事实源。运行时 `/openapi.json` 是由这套代码生成的唯一契约视图，Swagger UI、Scalar、Web 客户端、移动端客户端和契约测试都读取它。禁止手工维护第二份 OpenAPI/Swagger JSON 或 YAML。

当前 API 使用无版本路径，例如 `/api/topics`、`/api/jobs`、`/api/health` 和 `/api/ready`，不创建版本目录或版本前缀。OpenAPI 的 `openapi` 字段、`info.version` 和 URL 路径版本属于三个不同概念，不能互相替代。只有出现两个需要同时兼容的不兼容公共契约时，才可以先更新本文和 API 设计，再建立明确的版本策略。

每个 HTTP 操作必须声明唯一人工 `operation_id`、tag、成功状态、Pydantic 响应模型和实际可达的错误响应。生成的 OpenAPI、客户端代码和文档页面是派生物，不能反向成为第二个事实源。

### 全局异常与响应处理

统一决策见 [046 全局异常与响应契约设计](docs/design/046-全局异常与响应契约设计.md)。HTTP 状态、稳定错误码、任务状态、页面状态分别建模；应用异常不携带 HTTP 状态，API 边界负责映射。成功响应统一为资源 DTO、`PageView[T]`、`JobAcceptedView` 三类，失败统一 `ErrorView`；不引入全接口 Result 外壳或成功 body 改写中间件。分页固定 `items/next_cursor`；异步受理必须先持久提交；204/304 无 body，文件与流按实际媒体协议处理。

`ErrorView` 的 code/message/request_id 必需，校验错误的 details 只含安全 location/message/type。公共消息来自登记表，自定义 HTTP 5xx detail 和 validator 原始消息不能直接公开。请求 UUID 保存到 scope/state，正常及异常响应头、错误 body 和日志一致，不能回退为 unknown；日志异常链也需脱敏。公开错误码、HTTP 映射、必要响应头、客户端本地传输错误分类按 046 统一登记并验证。

运行响应、OpenAPI 与生成客户端必须一致；有请求校验的路由显式声明 ErrorView 422。Web 统一读取 details 并支持请求 ID 的响应头/body 回退；网络、超时、取消、非 JSON 与业务错误区分。可控代理失败、Worker 和流发送后的失败有各自处理边界，不假定 FastAPI handler 能覆盖整个系统。新增接口持续通过同一契约检查。

全局异常处理由 `api/exception_handlers.py` 统一注册，`main.py` 只负责调用注册函数。处理范围固定为：

1. `core.errors.ApplicationError` 及其子类：映射为稳定错误码和明确 HTTP 状态。
2. FastAPI/Starlette HTTP 异常：保留必要状态和响应头，转换为统一错误模型。
3. `RequestValidationError`：返回字段级输入错误，不泄露内部文件路径或原始敏感请求体。
4. 数据库和外部服务异常：先在 Service/Adapter 边界转换为应用异常；不得把驱动异常直接返回客户端。
5. 未处理的 `Exception`：服务端记录异常类型和脱敏后的堆栈位置，客户端只返回稳定的 `internal_error` 与 `request_id`。

公共错误模型 `ErrorView` 位于 `core/schemas.py`，至少包含稳定 `code`、面向用户的 `message` 和 `request_id`。错误处理器不得把异常字符串、SQL、Token、Cookie、连接字符串或完整请求体写入响应。成功响应使用端点级 `response_model`；不使用中间件自动包装所有成功响应，以免破坏文件、流式和特殊状态响应。

本规范参考 [FastAPI 多文件应用指南](https://fastapi.tiangolo.com/tutorial/bigger-applications/)、[FastAPI 错误处理指南](https://fastapi.tiangolo.com/tutorial/handling-errors/)、[FastAPI Lifespan 指南](https://fastapi.tiangolo.com/advanced/events/)、[FastAPI 官方全栈模板](https://github.com/fastapi/full-stack-fastapi-template) 和 [RFC 9457](https://www.rfc-editor.org/rfc/rfc9457.html)。这些资料用于确认框架机制和通用协议；目录、事实源和版本策略以本文为准。

身份切片采用 `pwdlib[argon2]` 处理密码；选定 JWT 时使用 PyJWT。依赖按真实使用方引入。Web 固定 Node.js 24.19.0、Next.js 16.3.5、React 19.2.8 与 pnpm 12.3.4；后端依赖版本在初始化时写入 `uv.lock`。

## 3. 数据与任务执行边界

1. API 路由负责 HTTP、认证与输入输出，经依赖注入调用领域服务；服务管理事务，SQLAlchemy 模型负责持久化。
2. 业务变更与 Outbox 写入同一 PostgreSQL 事务。独立发布器可靠地发送到 Kafka；消费者允许重复读取，以消息 ID、数据库唯一约束和业务状态保证幂等。
3. 消费者在业务事务提交后提交连续完成位置的 offset；处理并发时不得越过尚未完成的记录。Kafka 事务不能直接保证 PostgreSQL 副作用的原子性。[Kafka 消息交付语义](https://kafka.apache.org/41/design/design/)
4. Redis 的数据丢失不能导致任务或证据丢失。缓存设有效期与失效规则；限流故障时采用明确的保守策略。执行权、不可超额预算与撤权不能只依赖 Redis 锁或缓存。
5. `worker/` 维护 Kafka 客户端和消费者生命周期，`jobs/` 维护任务状态机；入口 `python -m worker`。031 已实现 outbox、手动 offset、inbox、租约/checkpoint 和有限调度恢复；039 S01 将受理消息升级为 `hotkey.jobs.accepted.v2`；009 S01—S03 已交付持久受理、owner 隔离读取、进度/取消、结构化失败、有限持久重试、到期 Outbox、重放防重和手动重试。047 S02 已登记固定 `webpage.collect` 处理器并通过真实 Kafka/Firecrawl 持久结果和恢复验证；其他 kind 没有处理器时必须持久失败后确认，不能把排队记录或空 Worker 当作业务执行成功。
6. API、Worker 各自创建数据库连接池和消息客户端，Session 不跨线程/任务共享。同步数据库调用不直接放入异步路由。
7. FastAPI 从路由装饰器、类型注解和 Pydantic 模型自动生成 `/openapi.json`。它是唯一 API 契约视图；Swagger UI、Scalar、Umi OpenAPI 和 Flutter 客户端共用该地址，不维护独立契约文件。客户端由生成命令更新，CI 负责自动生成与差异检查。
8. 数据库结构只由 `backend/database/schema.sql` 定义，SQLAlchemy Model 必须与其同批更新。当前不支持存量库自动就地升级；保留数据时采用备份、全新建库、完整建表和校验后导入流程。

## 4. 产品约束与未定事项

本地网页与浏览器采集的设计见 [047 Design](docs/design/047-本地网页与浏览器采集设计.md)，实施拆分见 [047 Plan](docs/plans/047-本地网页与浏览器采集计划.md)。复用既有 Firecrawl 获取公开网页正文，平台交互固定选用 Playwright Python `1.63.0` 原生 WebSocket 与匹配版本的单 browser 服务；Scrapling 0.4.15、CDP 及自适应 Selector 不作为本期运行依赖。浏览器脚本归 `sources/adapters/`，本地浏览器登录状态的受控文件读写归 `connections/adapters/local_secrets.py`，业务执行与存储仍归 content/jobs/connections/evidence 及 Kafka/PostgreSQL，不另建队列或任务库。连接版本允许 `none`、`server_credential`、`browser_state` 三种认证类型；`browser_state` 仅保存与该行 owner/连接/版本完全相符的受控引用，执行前由 connections 行锁和认证失效屏障解析。本地维护 CLI 只可管理既有 browser_state 连接，捕获文件须经过权限/类型校验；不从页面/API 接收文件路径，不提前创建无适配器的假平台来源。状态文件以 owner/连接/版本定位，运行时只接收已解析状态。047 S00—S02 已交付单 URL 网页业务和用户闭环；S03 已交付受控选型、隔离浏览器基础及仅允许测试域名的专用代理；平台会话、四平台评论与产品验收尚未完成。Playwright Python 包、镜像和隔离配置随 T01 真实 runtime/CLI 调用者加入。

S03 browser 使用 `backend/Dockerfile` 的独立构建 target、`backend/browser/server.js` 和固定官方 seccomp；现有 API/Worker target 不安装浏览器二进制。根 Compose 的 browser 仅接入内部控制/出口网络，不发布端口、不挂业务卷、不连接数据库网络；专用 Squid 代理独占公网桥接网络，browser 不直接接入该网络。Playwright 控制面使用由本机环境提供的 48 个随机十六进制字符组成的 `/ws/` 路径；browser 与调用方读取同一 `HOTKEY_BROWSER_WS_URL`，未配置或仍为根路径时拒绝启用，不将路径写入日志或版本库。代理首片仅放行 `example.com` 受控测试目标；真实平台域名及子资源仍须经适配器准入、请求计量和安全验证逐项开放，不以通用 Docker bridge 直通替代 SSRF 边界。

浏览器适配器把 Playwright 管理器启动、WS 建连、context 创建与交互纳入同一次默认/最大 45 秒协作式截止；context、连接和管理器按序各有 5 秒关闭等待预算，前一步失败仍尝试后一步。此限制不是进程级硬回收，也不是业务任务总截止；平台任务的持久恢复仍随 047 S03/S04 验证。

- 复用现有 MinIO；内容来源采用免费或自建方案，只采集公开或获授权数据。X 为首版必需来源；2026-09-22 用户明确 B站、小红书、抖音、微博的评论与回复都需要，四个平台分别验收。已知作品 URL 评论路径可先于各平台全站搜索，不能以首个国内来源成功关闭四平台需求。契约见 [008 Design](docs/design/008-评论与回复采集设计.md)，候选与证据见 [047 Research](docs/research/047-本地网页与浏览器采集调研.md)。
- twscrape 仍为 [002](docs/design/002-X免费采集与热点监控设计.md) 的验证候选；模型供应商、SDK、模型与设备仍按 [043](docs/design/043-模型服务接入与模型配置设计.md) 等专项处理，不能写成已接入。
- 002 S01 的受控适配器归 `backend/app/sources/adapters/x_twscrape.py`：复用固定 SDK 解析与协议常量，有界 HTTPX 负责单会话请求、初始化计量与资源释放。依赖方向只允许标准库、外部 SDK、所属来源契约；不直接操作业务表或启动采集任务。真实来源能力仍需 S02 验证。
- 内容预算不构成付费 AI 授权；现有模型方案继续按本地优先、外部默认关闭、零付费约束推进。
- Flutter 的目标平台、状态管理、路由及 Dart 客户端生成器尚未冻结；它们不影响 Flutter 技术方向或 Web 目录归属。

## 5. 实施与验证

按总体 Design → 需求/Plan → 失败验证 → 实现 → 回归/Acceptance 推进。046 S03 已通过，042 S00/S01 已交付底座；009/003 S00—S03、004 S00—S03 与 007 S00—S03 已交付任务控制、本地主题规则、连接能力证据及可追溯作品资料，含追加式可见性、乱序稳定投影和生命周期清理。002 S01 已交付固定 SDK、有界单会话传输与受控故障验证，不代表真实来源接入。035 S02 已验证当前资源的授权、关联回滚和 HTTP 错误禁存，完整分支仍随业务接入；其余已登记先行技术切片继续有效。004 S03 配置/替换/启停 API/UI、版本屏障与停用/认证失效受理阻断已通过，真实处理器外采门禁随 B04 与 S04 联验；当前没有真实平台连接或探测/采集证据，003 的真实上游扩词、调度/主题任务联验留在 S04。外部来源条件继续登记，不阻塞无关的内部切片，技术切片通过不等于产品验收。

交付前执行后端 Ruff、mypy、pytest、OpenAPI 漂移与客户端生成检查，以及前端 ESLint、Prettier、类型检查、生产构建和浏览器验证。集成测试使用隔离的 PostgreSQL、Redis、Kafka，并验证重复事件、提交后中断、消费者再均衡、Redis 失效和任务恢复。

## 6. 维护

PROJECT.md 是项目技术、架构、目录、API 契约和数据库事实源。AGENTS.md 只补充实现执行门禁、工具命令和验证要求，不得定义与本文冲突的架构；两者共同约束实现。技术、目录或运行约束变化时同步更新本文、AGENTS、HANDOVER 和对应 Design。产品进度只在完成实际验收后更新 BACKLOG。
