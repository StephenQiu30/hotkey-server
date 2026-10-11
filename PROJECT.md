# Ripplesight 技术架构

本文说明 Ripplesight 由哪些部分组成、各部分负责什么、必须遵守哪些技术约定。产品能力与验收标准见[文档工作区](docs/index.md)，工程流程见 [AGENTS](AGENTS.md)，进度见 [BACKLOG](BACKLOG.md)。

## 1. 技术栈

| 部分 | 技术 |
|---|---|
| 后端 | Python 3.12、FastAPI、Pydantic 2、SQLAlchemy 2、psycopg 3；依赖用 uv 管理（`pyproject.toml` + `uv.lock`） |
| Web | Node.js 24、pnpm 12、Next.js 16（App Router）、React 19、TypeScript、Tailwind、shadcn/ui + Radix、Axios |
| 数据 | PostgreSQL（业务数据唯一来源）、Redis（只放可重建的缓存和限流）、Kafka（持久任务消息）、MinIO（证据与文件） |
| 后台任务 | 独立 Worker 进程消费 Kafka；独立 Scheduler 进程用 APScheduler 每 30 秒扫描到期任务 |
| 本机外部服务 | RSSHub（1200）、SearXNG（8888）、Firecrawl（3002）、MediaCrawler（宿主子进程）、Codex app-server（宿主子进程） |
| 质量工具 | 后端 Ruff、mypy、pytest；Web 端 ESLint、Prettier、tsc、Vitest、生产构建 |

## 2. 进程与端口

| 进程 | 启动方式（在 `backend/app` 或 `frontend`） | 本机端口 |
|---|---|---|
| API | `uvicorn main:create_app --factory` | 8667（文档 `/docs`、`/scalar`，契约 `/openapi.json`） |
| Worker | `python -m worker` | — |
| Scheduler | `python -m worker.scheduler` | — |
| CLI | `python -m cli` | — |
| Web | `pnpm dev` / `pnpm start` | 8666 |

API 进程不跑定时器，也不消费消息。同一时间只运行一个 Worker，并且它运行在宿主机上，因为 Codex 登录状态和 MediaCrawler 浏览器都在宿主机。Compose 中容器内的端口统一为 8080，映射到宿主机的 8666/8667。

本机前端与 API 由 Compose 的 frontend、backend 服务启动，仅绑定 localhost。项目文档回归 docs/ 下的 Markdown/Git 管理，由 Obsidian 或编辑器直接阅读和修改，不运行文档网站。

Compose 本机项目名为 `ripplesight`，生产项目名为 `ripplesight-prod`，容器、网络和自有镜像使用 `ripplesight` 前缀；后端容器用户也使用 `ripplesight`，UID 保持 10001。应用继续使用本机已有数据库及 `HOTKEY_*` 配置。可选环境栈的新数据卷跟随当前 Compose 项目名，已有卷可通过 `HOTKEY_POSTGRES_VOLUME_NAME`、`HOTKEY_REDIS_VOLUME_NAME`、`HOTKEY_KAFKA_VOLUME_NAME` 显式复用；改名不复制、删除或初始化已有数据。

Compose 使用 `docker-compose.yml` 中的构建镜像。修改源码或依赖后重新构建对应服务；需要热更新时，按 backend/README.md 或 frontend/README.md 在宿主机启动开发服务。环境变量和数据库结构变化分别按配置与数据库流程处理。

## 3. 目录

本机父工作区为 `Ripplesight/`，其中 `ripplesight-server/` 是下方的主仓库，`ripplesight-app/` 是尚未初始化的冻结客户端仓库。工具和配置统一使用这两个仓库路径。

```text
ripplesight-server/
├── backend/
│   ├── app/main.py            # 应用工厂，只负责装配
│   ├── app/api/               # 路由汇总、依赖注入、错误映射
│   ├── app/core/              # 配置、日志、公共错误、通用 Schema、时间
│   ├── app/db/                # ORM 基类、连接池、模型注册
│   ├── app/<领域>/            # 各业务领域，见 §4
│   ├── app/worker/            # Kafka 消费、任务子进程、Scheduler
│   ├── app/cli/               # 维护命令
│   ├── sql/schema.sql         # 唯一的完整建表脚本
│   └── tests/                 # unit / integration / architecture
├── frontend/
│   ├── src/app/               # 路由；页面专属组件放在路由下的 components/
│   ├── src/components/        # ui/（shadcn 基础组件）及按功能划分的复用组件
│   ├── src/layout/            # 全站外壳
│   ├── src/api/               # 由 OpenAPI 生成的客户端（不要手改）
│   ├── src/request.ts         # 唯一的 HTTP 传输层
│   ├── src/proxy.ts           # 会话门禁与 CSP
│   └── tests/                 # 前端测试
└── docs/                     # 唯一项目文档目录，也是 Obsidian vault
    ├── index.md              # 知识库入口与使用说明
    ├── requirement/          # 当前页面和功能需求
    ├── design/               # 现行视觉、布局与交互规范
    ├── prd/                  # 产品目标、边界与验收标准
    ├── plan/                 # 当前执行顺序与交付门槛
    ├── templates/            # 四类可选模板
    ├── views/文档.base       # Obsidian 文档视图
    └── .obsidian/            # 共享配置；个人布局与缓存继续忽略
```

Web 外壳使用 `BasicLayout → PageContainer`：前者管理侧栏、移动导航和会话，后者统一有限高度的正文滚动区与宽度，页脚仅登录页使用；页内标题由页面提供。`LayoutContainer` 只负责横向对齐。滚动引用指向 `PageContainer` 的正文区，路由切换、阅读定位和跳到正文共用该节点；打印恢复自然文档流。全站只保留一个 main，业务页面不另建全屏滚动容器。

普通页头复用 `components/system/page-header.tsx` 的 `PageHeader`，统一标题、可选说明、操作与详情面包屑；面包屑直接组合现有 shadcn 基础组件。空状态由 `PageState` 组合 `Empty`，说明与操作均按当前场景提供，不自动添加无关跳转或技术诊断。刊物报头与登录表单保留专用排版。

## 4. 业务领域

后端只允许下列顶层包，架构测试（`tests/architecture/test_structure.py`）会拒绝未登记的包。新增领域需要先修改本表和该测试。

| 领域 | 负责 |
|---|---|
| identity | 账户、密码、会话、邮箱验证码、GitHub 登录、头像 |
| monitors | 主题与关键词、采集版本、调度 |
| jobs | 任务、Outbox、预算、租约、重试、取消、到期 |
| connections | 来源连接、授权与状态 |
| sources | 来源定义与外部采集适配器（`sources/adapters/`） |
| content | 内容身份、正文版本、评论父链、观察记录、主题匹配 |
| evidence | 文件元数据与 MinIO 适配器 |
| ai | 模型调用、Codex app-server 适配器、调用回执与用量账本 |
| analysis | 相关性、精选、结构化、写作、翻译、情感、观点、评测 |
| events | 事件归并、事实与进展、热度、向量 |
| reports | 个人日报/周报、公开刊物、导出 |
| notifications | 订阅、投递、告警 |
| knowledge | Obsidian 业务报告导出、检索与问答 |
| publication | 公开内容的许可、投影与撤回（对外分发部分已冻结） |
| leaderboard | 模型榜 |
| operations | 运营权限、站点设置、反馈、心跳、维护 |
| backups | 备份与恢复验证 |
| api / core / db / worker / cli | 见 §3 |

## 5. 分层规则

以下规则由架构测试和 ESLint 强制执行：

- **Router** 只处理 HTTP、身份校验、CSRF 和 DTO；通过 `api` 的依赖注入调用服务，不直接用 ORM，不构造 Service，也不发布消息。
- **Service** 只直接使用本领域的 ORM 模型；读取其他领域的数据要通过对方提供的函数或 DTO。跨领域的原子写入共用同一个 Session，由最外层负责提交。
- **Schema**（`schemas.py`）不依赖 ORM、Session 或 FastAPI。
- **Adapter**（`*/adapters/`）不依赖 API、ORM、Worker 或任务状态。
- `core` 不反向依赖业务领域；Worker 不依赖 HTTP 协议层。
- 应用错误不携带 HTTP 状态码，由 API 边界统一映射成 `ErrorView`。
- 不建通用的 BaseService / BaseRepository、全局 utils，也不建第二套队列、账本或正文库。
- Session 按请求或任务创建，不跨线程共享；资源由所属进程创建和关闭，模块导入时不联网。

## 6. 数据库

- `backend/sql/schema.sql` 是唯一的建表来源，自带 `BEGIN/COMMIT`，只用于全新的空库。不用 Alembic，不用 ORM 建表，也不用 SQLite 存业务数据。所有时间字段使用 `TIMESTAMPTZ`。
- 改表结构时，SQL、ORM 模型和结构断言要在同一次提交里更新，并在一个全新的 PostgreSQL 上核对所有表的列、类型、可空性和主键。
- 业务库名固定为 `hotkey`。测试只用独立的 `hotkey_test_<后缀>` 库，用完删除；绝不能在业务库上跑测试。
- **升级有数据的库**：先停写并备份，在新库上实际恢复一遍，确认能读；再建一个全新空库、执行完整的 `schema.sql`、导入并核对数据（各表行数、外键、任务和预算等）；全部核对无误后才切换，旧库保留作回退。禁止直接在旧库上执行 `schema.sql`。

## 7. 任务与可靠性

- 业务状态和 Outbox 写在同一个 PostgreSQL 事务里，由 Outbox 发布到 Kafka。Worker 在业务提交后才提交 offset，通过消息 ID、租约和唯一约束保证重复消费也不会重复写入。
- Worker 父进程独占 Kafka 消费和任务的最终状态；每个任务在 `spawn` 出的子进程里执行，子进程自己建立数据库连接。任务取消、超时或进程异常退出时，回收整个进程组。
- 持久的重试和预算由 `jobs` 管理；适配器内部只做有限次数的安全重试。状态不明的外部发送不会自动重发。

## 8. 外部来源与模型

- RSSHub/SearXNG 聚合服务只连接本机实例，主机只能是 `127.0.0.1`（宿主机）或 `host.docker.internal`（Compose）。平台直连和聚合服务是不同路径；各 HTTP 适配器均强制目标主机白名单，并校验每一次重定向。
- MediaCrawler 桥保留固定版本核验和本地已保存样本/评论读取。其搜索子进程目前只有整批预算预扣与轮询终止，缺逐 HTTP 请求准入，线上搜索工厂在启动前拒绝；补齐逐请求保护并验证后才恢复。B站实时采集使用具备逐请求回调的本人本机 Chrome 方式；浏览器资料目录权限为 700、文件权限为 600，遇到验证码、登录失效或限流就停用。
- 模型只走本机 Codex app-server：每个分析任务启动一个子进程，只读、不需要审批、只传最小环境变量、使用空的工作目录。模型输入一律视为不可信文本，输出必须是结构化结果并经过校验；数字、排序和引用由程序计算。
- 费用、账号、许可等产品边界见 [产品边界](docs/requirement/00-产品目标与边界.md#已确认边界)。

## 9. API 与身份

- FastAPI 路由注解加 Pydantic 是唯一的接口契约，运行时生成 `/openapi.json`；Web 客户端由它生成，不手写 OpenAPI 或客户端 DTO。
- 每个接口声明唯一的 `operation_id`、中文 summary 和 tag、成功响应模型和实际可能返回的错误。成功时返回资源 DTO、`PageView[T]`（`items` / `next_cursor`）或 `JobAcceptedView`；失败时统一返回 `ErrorView(code, message, request_id, details)`。
- 路径统一是 `/api/*`，不带版本号。日志只记录方法、路由模板、状态码和耗时，不记录 URL 参数、正文、Cookie 或 Token。
- 登录方式有三种：密码（邮箱或用户名）、邮箱验证码、GitHub OAuth。注册仅通过邮箱或 GitHub 验证身份，首次必须设置自选用户名和密码再进入工作台；密码入口只用于已有账号。GitHub 回调与邮箱验证统一导向账户设置，私有页面对未设置密码的会话继续引导设置，业务读写 API 返回 `account_setup_required`，身份验证与凭据设置接口仍可使用。会话存在数据库里，有效期 12 小时，可撤销；Cookie 为 HttpOnly + SameSite=Lax，生产环境加 Secure。写请求校验与会话绑定的 CSRF。修改密码会撤销全部旧会话。
- 公开页面不需要登录，只读取 `HOTKEY_PUBLIC_PUBLICATION_OWNER_ID` 指定的发布账号；未配置时返回 `publication_not_configured`。个人数据按 owner 隔离，跨账户访问返回 404。

## 10. 配置与部署

- **项目文档**：docs/ 是 Markdown 唯一原文和 Obsidian vault；文档按 requirement、design、prd、plan 四个目录维护，进度和技术约定仍在根目录 BACKLOG、PROJECT、AGENTS。链接与锚点检查复用 frontend/tests/docs/ 的 pnpm docs:check，需求、设计和计划同编号同名称，不强制元数据或生成索引。业务报告的 Obsidian 导出继续使用 knowledge/obsidian.py。
- 环境文件只放在仓库根目录：本机用 `.env`，生产用 `.env.prod`，模板是 `.env.example`。所有进程都读这一份，进程注入的环境变量优先。
- `docker-compose.yml` 定义应用（API、Web，以及按需启用的 Worker / Scheduler / CLI）；`docker-compose-env.yml` 只在需要全新的 PostgreSQL/Redis/Kafka 时使用；`docker-compose-prod.yml` 通过 include 复用应用定义。
- 浏览器采集由宿主机来源适配器执行；报告导出使用 Playwright，不部署独立的远程 browser 服务。
- Web 生产构建为 standalone，以非 root 用户和只读文件系统运行；每个请求生成独立的 CSP nonce。

## 11. 项目文档

[文档入口](docs/index.md) 连接四类编号文档：requirement 逐项维护原始需求、功能约束与验收；design 与 plan 按 requirement 逐项对应，使用相同编号和文件名，分别维护结构、交互及实施步骤与门槛；prd 维护产品定位与研究依据。目录从 00 开始连续编号，不保留统一正文；新增或调整需求同步维护三目录与引用，不强制 frontmatter 或额外任务卡。

Obsidian 直接打开 docs，共享配置、Templates 与文档视图保留；进度仅在根目录 BACKLOG，工程约定仅在 PROJECT 和 AGENTS。API 与数据库直接查代码和唯一 schema。当前文档只维护有效要求与使用说明，不保存历史文档、归档副本或已完成事项流水。业务报告的 Obsidian 导出独立于项目文档。

## 12. 关键词监控运行链

| 当前链路 | 职责 |
|---|---|
| 主题与来源配置 | monitor_topics/versions、source_connections/versions；编辑比较版本，来源准入由服务校验 |
| 运行与调度 | jobs/outbox、租约、预算和到期窗口；页面读取任务与回执 |
| 材料与评论 | content 身份、正文版本、观察记录和 threads；来源发布时间与采集时间分离 |
| 分析与展示 | annotations、公开许可投影、事件热度和报告引用；缺失数据使用真实空态 |

B 站 Chrome 来源使用 `bilibili` 来源键与固定 API 地址 `https://api.bilibili.com`，通过 SourceAdapter 接入 `keyword.search` 和 `source.comments`。适配器版本写入任务冻结的 `jobs.scope.source_adapter_version`，主题、任务、内容、预算与覆盖复用业务数据库。登录由本机 cookie-source 提供，仅对显式绑定的 owner 生效；凭据只驻留宿主进程内存，验证码、登录失效或限流时停止。

来源预设为每小时一轮、每日 60 次请求、单页两帖、每帖最多 20 条根评论、00–08 点静默、保留 30 天；搜索与评论共用预算。有界单机执行器按 owner 与来源限定任务，复用 JobExecutionService 的租约、幂等和完成流程。抽样始终标为部分覆盖，真实增量、自然持续运行和模型质量分别验收。

字段与约束见唯一 [schema](backend/sql/schema.sql)，接口以 [生成客户端](frontend/src/api) 与 [后端路由](backend/app/api/routers) 为准。


## 13. 页面与服务的边界

页面需求与视觉规范分别在 [编号需求](docs/index.md#需求目录) 和 [编号设计](docs/index.md#设计主题) 中逐项维护。页面需要的字段由现有客户端和后端合同核对；查询派生值、本机状态与持久业务事实分开。每个新增结构必须有实际读写方，不能为路由、卡片或计数复制一套数据。

`/topics`、`/monitors/[topicId]` 和 `/workspace` 共用 TopicsWorkspace。权限、公开许可、版本、任务租约与 outbox 等现有运行职责保留；物理删表须先核对消费者、历史数据和外键，再按 §6 在独立库完成恢复与迁移验证。

## 14. CI 检查

CI 验证当前锁定运行栈。frontend执行完整静态检查、交互回归与构建；contract仅执行同提交运行API的生成客户端漂移及传输层专项，不重复全套Web测试；backend使用隔离hotkey_test_*库、唯一schema和实际Redis/Kafka/MinIO做全量回归；runtime使用生产镜像验证依赖、HTTP代理、会话/CSRF、动态CSP、Worker及非root只读边界。项目文档仅保留本地链接与锚点校验、检查工具测试，镜像测试使用隔离Git夹具而不依赖Docker上下文携带仓库元数据。CI成功不替代Figma视觉、真实提供方和长期服务验收。

## 15. 页面路由约定

页面的静态路径用斜线表达层级，不使用连字符拼接多个层级：来源维护为 `/sources/editorial`。动态标识和日期仍保留其业务格式。公开来源由发布账号配置，个人配置继续按会话账号隔离；不得把个人来源自动公开。

个人 RSS/网页/JSON 来源复用 `editorial_source_profiles` 及其不可变版本、连接与审计，不建第二套来源表。服务端生成 `ed_personal_` 命名空间的 source_key，个人接口只访问会话 owner 下此命名空间；站点运营列表读取原 `ed_` 来源。个人命名空间拒绝配置公开发布策略，即使该用户也是发布账号也不能自动进入公开页面。创建默认关闭，不发起外部请求；启用继续核验原来源许可和保留策略。个人配置接口位于 `/api/sources/personal`，页面位于 `/sources/personal`。
