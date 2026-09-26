# HotKey Server 项目与技术选型

更新日期：2026-09-26。本文固定仓库边界、技术栈、后端目录、API 契约和运行约束。产品需求见 [PRD 001 v5.0](docs/prd/001-热点舆情监控平台需求.md) 和各里程碑 PRD，Epic 设计见 [Design 001 v4.0](docs/design/001-热点舆情监控平台总体设计.md) 及 Design 002—007，逐 Issue 执行计划见 [Plan 索引](docs/plan/README.md)。

## 1. 定位与仓库边界

HotKey 当前以**可信的信息获取**为核心：按主题持续取得可追溯的帖子、评论与六个公开热榜，显示来源 × 能力 × 时间窗的覆盖和缺口，并由本机 Codex 判断相关性。日报、周报、Obsidian 知识库和推送是后续分别验收的结果；飞书推送暂缓。使用性质为个人/非商业研究。本仓库同时维护 Python 后端与 Web 前端；同级 `hotkey-app`（Flutter）暂停。

```text
HotKey/
├── hotkey-server/
│   ├── PROJECT.md        # 本文：技术与架构约束
│   ├── AGENTS.md         # 实现门禁与验证命令
│   ├── BACKLOG.md        # 唯一进度看板（≤ 10 KB）
│   ├── HANDOVER.md       # 当前实现快照（≤ 5 KB）
│   ├── backend/          # Python API、Worker、CLI
│   ├── frontend/         # Next.js Web 工作台
│   └── docs/             # PRD、Design/Epic、逐 Issue Plan、验收记录
└── hotkey-app/            # Flutter 客户端（暂停）
```

目录规划和测试目标不代表业务能力已经实现；能力是否可用以 BACKLOG 和验收记录为准。

### 1.1 用户与交付结果

主要用户是使用本机单 owner 工作台的研究者；报告订阅者属于后续推送的接收方，不需要工作台账号。当前成功标准是逐来源真实采集、可追溯和可核对的连续覆盖；报告与送达另验。

### 1.2 能力里程碑

| 阶段 | 内容 |
|---|---|
| M1 | 四个关键词来源、HN 评论父链、六榜、覆盖查询和 Codex 相关性；同窗连续 72 小时验收 |
| M2 | 本人账号 B 站 MediaCrawler 试点，真实低频采集、风控停止/人工恢复，再无风控运行 72 小时 |
| M3 | 跨平台事件归并、热度与人工修订 |
| M4 | 分别验收分析质量、日报/周报、Obsidian 导出与问答 |
| M5 | SMTP 与暂缓的飞书按渠道分别验收 |
| M6 | 告警、指定账号、导出增强及后续来源逐项准入 |

### 1.3 当前实现边界（2026-09-26）

已实现并可复用：单 owner 身份与会话、主题与关键词规则、来源连接、持久任务执行（Outbox/Kafka/Worker、重试、取消、租约、恢复）、内容身份/版本/指标观察、来源契约、网页正文采集、隔离浏览器服务、预算账本、备份恢复、统一错误契约和 Web 工作台框架。

宿主机 Worker 已注册 `webpage.collect`、`keyword.search`、`source.comments`、`source.hotlist`、`analysis.annotate`、`report.daily`、`notification.send`、`knowledge.export`；独立调度、四关键词来源、六榜、Codex 调用/标注、日报、Obsidian 日报导出和飞书发送均有代码与受控测试。开发库约 4 小时运行仅证明有限真实范围，M1 连续 72 小时及各产品 AC 未通过。B 站适配器有离线回放，修复后真实采集未做且开关关闭；SMTP 与事件尚未实现。现状和证据等级见 Design 001 第 2 节及对应逐 Issue Plan；旧总 Plan 001 的历史证据从 Git 历史查阅。


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
| PostgreSQL | 业务事实、权限、任务、进度、幂等记录与 Outbox 的持久存储；M4 问答启用 `pg_trgm` 检索（不引入向量库） |
| Redis | 缓存、限流和可重建临时状态；关键权限、预算与任务状态仍有数据库依据 |
| Kafka | 任务事件与异步消息传输，由 Python Worker 消费 |
| MinIO | 复用既有对象存储，保存有权限与保留期约束的文件及证据 |
| Docker Compose | 根 `docker-compose.yml` 编排 HotKey 应用；RSSHub、SearXNG 由同级 `Docker` 服务集合编排，Firecrawl、MediaCrawler 各有本地入口 |
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

现行统一决策如下；旧 Design 046 全局异常与响应契约已删除，见 Git 历史。HTTP 状态、稳定错误码、任务状态、页面状态分别建模；应用异常不携带 HTTP 状态，API 边界负责映射。成功响应统一为资源 DTO、`PageView[T]`、`JobAcceptedView` 三类，失败统一 `ErrorView`；不引入全接口 Result 外壳或成功 body 改写中间件。分页固定 `items/next_cursor`；异步受理必须先持久提交；204/304 无 body，文件与流按实际媒体协议处理。

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
8. 数据库结构只由 `backend/database/schema.sql` 定义，SQLAlchemy Model 必须与其同批更新。`schema.sql` 自身以 `BEGIN`/`COMMIT` 保证完整 DDL 原子性；CI 的 psql stdin 和 Compose 官方 entrypoint 挂载均依赖此事务，显式 `--single-transaction` 可额外使用。只对全新空库执行，失败后核对没有部分业务表。当前不支持存量库自动就地升级；保留数据时采用备份、全新建库、完整建表和校验后导入流程。

## 4. 产品约束与未决事项

- 只采集公开或获授权的数据；遵守平台频率限制；凭据和登录态只存服务端，不进前端、日志和代码库。
- 本轮逐来源验收四个关键词来源（HN Algolia、Google News 搜索 RSS、本机 SearXNG 的 `duckduckgo news`、本机 RSSHub `/36kr/newsflashes`）、六个公开 RSSHub 热榜，以及本人账号 B 站试点。B 站使用宿主机 MediaCrawler 子进程和 `~/Desktop/Docker/mediacrawler-start-local/` 的固定补丁记录、独立 CDP 资料；微博等登录平台后续逐项准入。搜索、帖子、评论、热榜分别验收，不以公开热榜代替登录内容。
- 来源频次、请求与模型调用经来源预设及 037 预算账本设置硬上限。预设执行策略按连接版本存于 `source_connection_versions.execution_policy` 非秘密 JSONB，来源预算按 owner/source/metric/窗口规则保持稳定身份；升版不返还已用额度。`schema.sql` 的新增列仅用于全新空库，保留库先经 051 恢复。模型仅经本机 Codex app-server，不发付费模型请求。X 仅用官方 API，凭据与月度上限未确认前禁止真实请求。
- 模型适配器归 `ai/adapters/`，供应商可替换。报告中的数字一律由数据库计算，模型只负责判断和写作，正文的数字与链接须通过校验。
- 外部正文按不可信内容处理：进入模型时放入分隔的数据区，模型输出只接受结构化字段。
- 复用现有 MinIO。不更换 PostgreSQL 镜像，不引入向量库或搜索引擎（DEC-001-208）。
- 知识库是本地 Obsidian vault（`HOTKEY_OBSIDIAN_VAULT_PATH`，默认 `~/Desktop/Markdown/Obsidian`）的 `HotKey/` 子目录；HotKey 单向写入管理区块，原子写，不覆盖用户区块（[Design 005 第 3.3 节](docs/design/005-报告与知识库设计.md)）。
- 流水线由独立调度进程 `python -m worker.scheduler` 扫表驱动（DEC-001-203）；当前 M1/M2 只在宿主机运行一个 `python -m worker`，Compose 中的 worker 服务不启动（DEC-001-205）。
- 模型经本机 Codex app-server，每个分析 Job 启动一次，只传最小环境变量（DEC-001-207）；模型名必须显式配置。
- 推送秘密只从环境变量读取（DEC-001-210）；飞书暂缓，SMTP 后续独立实现，渠道送达不阻断信息获取或报告生成。
- 冻结（保留代码，不再扩展、不作前置门禁）：032 备份 S03+、033 公平派发/熔断、028 S02+、029 S03、039 S02+、040、042 B0、010 历史回补、Flutter App。移出范围：044、045。

## 5. 实施与验证

按 [Plan 索引](docs/plan/README.md) 的单 Issue 推进：计划评审先固定文件/接口/数据/调度/测试与验收合同 → 核对技术依赖和代码漂移 → 失败测试 → 实现 → 回归 → 阶段验收记录。核心契约未定不得列为实施就绪；真实账号/费用/渠道条件仅阻塞对应步骤。架构或数据库变化同步所属Design/Epic、总Design001与本文。

现行逐Issue计划001—057的持久化/任务细则见Design001 §4、子Design及Plan索引：到期窗口与采集周期归jobs，事件事实归events，报告设置唯一读取/写入`monitor_topics.report_time`、`report_timezone`、`weekly_report_enabled`，冻结和导出归reports，不新增`report_schedules`；原始导出归content，告警/投递审计归notifications，账号归monitors，检索投影/回答归knowledge。055—057仅承接共享底座回归/冻结，均不代表产品验收；不创建额外共享层、服务或存储桶。新增router按目标路径独立注册，现有 `/api/v1/reports` 由Plan018统一到 `/api/reports` 并同步生成客户端。新任务硬截止见Design001，真实依赖和保留库恢复仍按既有门槛验证。

Plan 001 的主题采集版本在 `monitor_topic_versions` 固定关键词组、排序后的来源键和主题请求间隔；`monitor_topics` 与 `monitor_schedules` 保存当前投影，既有 Job 的配置版本不随更新重释。只改显示名称或报告/推送偏好不生成采集版本。来源保存须有已应用搜索预设及当前准入/运行策略；恢复还检查可用来源预算，真实采集可用性仍须逐来源验收。

`collection_due_windows` 属于 jobs 的持久到期事实，唯一键为 `(owner_id, schedule_key, due_at)`，允许未受理窗口没有 Job；`coverage_windows`、内容观察、资源尝试和预算账本仍分别保存执行事实。Plan 033 提供领域只读 DTO，Plan 031 才把调度受理写入同一事务，Plan 004 消费查询。

交付前执行后端 Ruff、mypy、pytest、OpenAPI 漂移与客户端生成检查，以及前端 ESLint、Prettier、类型检查、生产构建和浏览器验证。数据库和消息行为用隔离的真实 PostgreSQL/Redis/Kafka 验证。适配器用固定样本做契约测试，并以一次真实请求冒烟；模拟数据不能算采集成功。

## 6. 维护

PROJECT.md 是技术、架构、目录、API 契约和数据库约束的事实源；AGENTS.md 只补充实现门禁和命令，两者不得冲突。产品需求以总 PRD 001 和对应里程碑 PRD 为准；Epic 边界在 Design，任务实施规格在单 Issue Plan；BACKLOG 是唯一进度看板（≤ 10 KB），HANDOVER 保持 ≤ 5 KB，均不追加流水账。
