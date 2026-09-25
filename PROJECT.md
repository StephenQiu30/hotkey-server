# HotKey Server 项目与技术选型

更新日期：2026-09-25。本文固定仓库边界、技术栈、后端目录、API 契约和运行约束。产品需求、总体设计和计划分别以 [PRD 001](docs/prd/001-热点舆情监控平台需求.md)、[Design 001](docs/design/001-热点舆情监控平台总体设计.md)、[Plan 001](docs/plans/001-热点舆情监控平台总计划.md) 为准。

## 1. 定位与仓库边界

HotKey 按关键词持续收集各主流平台的帖子和评论，整理成可检索的知识库，并每天、每周自动生成舆情报告推送给用户。2026-09-25 起按 v2.0 推进，交付物收敛为**日报、周报、知识库**。本仓库同时维护 Python 后端与 Web 前端；同级 `hotkey-app`（Flutter）暂停。

```text
HotKey/
├── hotkey-server/
│   ├── PROJECT.md        # 本文：技术与架构约束
│   ├── AGENTS.md         # 实现门禁与验证命令
│   ├── BACKLOG.md        # 阶段与任务状态（≤ 5 KB）
│   ├── HANDOVER.md       # 当前实现快照（≤ 5 KB）
│   ├── backend/          # Python API、Worker、CLI
│   ├── frontend/         # Next.js Web 工作台
│   └── docs/             # PRD/Design/Plan 001、现有实现参考、验收记录
└── hotkey-app/            # Flutter 客户端（暂停）
```

目录规划和测试目标不代表业务能力已经实现；能力是否可用以 BACKLOG 和验收记录为准。

### 1.1 用户与交付结果

主要用户是负责舆情的产品、市场、公关人员；报告订阅者只通过推送接收日报和周报。首版按单个团队自建部署，沿用单 owner 登录。成功标准是用户每天按时收到可信、可追溯的日报，而不是平台数量或采集条数。

### 1.2 交付阶段

| 阶段 | 内容 |
|---|---|
| P1（第 1—2 周） | A 档来源（RSS、Hacker News、Reddit）→ 帖子 + 评论 → 相关性/摘要/情感 → 日报 → 推送 |
| P2（第 3—4 周） | B 档来源（B 站、微博、知乎）、热榜、事件归并与热度 |
| P3（第 5—6 周） | 周报、知识库检索与问答、导出 |
| P4（第 7 周起） | 突发告警、账号追踪、C 档来源（X、小红书、抖音、公众号）逐个接入 |

### 1.3 当前实现边界（2026-09-25）

已实现并可复用：单 owner 身份与会话、主题与关键词规则、来源连接、持久任务执行（Outbox/Kafka/Worker、重试、取消、租约、恢复）、内容身份/版本/指标观察、来源契约、网页正文采集、隔离浏览器服务、预算账本、备份恢复、统一错误契约和 Web 工作台框架。

尚未实现：Worker 只注册了 `webpage.collect`；评论无法入库；没有周期调度；没有模型、分析、事件、报告、推送和知识库；没有任何真实社交来源数据。修正方案见 Design 001 第 2 节。


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
| PostgreSQL | 业务事实、权限、任务、进度、幂等记录与 Outbox 的持久存储；P3 起启用 pgvector 与 pg_trgm 承担知识库检索 |
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
│   │   ├── messaging.py           # Kafka 收发与位点提交
│   │   └── execution.py           # 单任务子进程监督与有界终止
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
- `worker/` 和 `cli/` 调用领域 Service，不复制 HTTP 层或业务规则。Worker 父进程独占 Kafka Consumer、offset 与任务终结；`worker/execution.py` 只监督单个 `spawn` 子进程，子进程自行创建数据库资源，不接收父进程 Session、Engine、Kafka Consumer 或网络连接。
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

## 4. 产品约束与未决事项

- 只采集公开或获授权的数据；遵守平台频率限制；凭据和登录态只存服务端，不进前端、日志和代码库。
- 来源按接入难度分三档交付：A 档公开 API/RSS，B 档公开 Web 接口 + 现有 Playwright 浏览器服务，C 档需登录或付费。每个来源分别验收搜索、帖子、评论、热榜能力；不以一个来源接通代表其他来源可用。
- 付费来源（X 官方 API 等）和外部大模型调用都经 037 预算账本做月度硬上限：80% 提醒，100% 停止付费调用；免费来源不受影响。是否允许付费模型、上限多少见 OPEN-001-102；X 是否启用见 OPEN-001-104。上限确认前不发起真实付费请求。
- 模型适配器归 `ai/adapters/`，供应商可替换。报告中的数字一律由数据库计算，模型只负责判断和写作，正文的数字与链接须通过校验。
- 外部正文按不可信内容处理：进入模型时放入分隔的数据区，模型输出只接受结构化字段。
- 复用现有 MinIO。P3 起 PostgreSQL 镜像改为带 pgvector 的同主版本镜像（DEC-001-201），不引入独立向量库或搜索引擎。
- 冻结（保留代码，不再扩展、不作前置门禁）：032 备份 S03+、033 公平派发/熔断、028 S02+、029 S03、039 S02+、040、042 B0、010 历史回补、Flutter App。移出范围：044、045。

## 5. 实施与验证

按 Plan 001 的任务推进：开工前在任务下补一段说明（做什么、改哪些文件、怎么验收）→ 失败测试 → 实现 → 回归 → 阶段验收记录。架构或数据库结构变化同步更新 Design 001 与本文。

交付前执行后端 Ruff、mypy、pytest、OpenAPI 漂移与客户端生成检查，以及前端 ESLint、Prettier、类型检查、生产构建和浏览器验证。数据库和消息行为用隔离的真实 PostgreSQL/Redis/Kafka 验证。适配器用固定样本做契约测试，并以一次真实请求冒烟；模拟数据不能算采集成功。

## 6. 维护

PROJECT.md 是技术、架构、目录、API 契约和数据库约束的事实源；AGENTS.md 只补充实现门禁和命令，两者不得冲突。产品需求只在 PRD 001 维护，任务只在 Plan 001 维护；BACKLOG 与 HANDOVER 各保持 ≤ 5 KB，不追加流水账。
