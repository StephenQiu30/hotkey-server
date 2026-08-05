# HotKey Server

<p align="center"><a href="README.md">简体中文</a> · <a href="README_EN.md">English</a></p>

<p align="center">
  <strong>把分散的公开信号，转化为可验证、可追溯、可发布的事件情报。</strong>
</p>

<p align="center">
  <a href="https://github.com/StephenQiu30/hotkey-server/actions/workflows/ci.yml"><img alt="CI" src="https://github.com/StephenQiu30/hotkey-server/actions/workflows/ci.yml/badge.svg?branch=main"></a>
  <a href="https://go.dev/"><img alt="Go 1.26+" src="https://img.shields.io/badge/Go-1.26%2B-00ADD8?logo=go&logoColor=white"></a>
  <a href="LICENSE"><img alt="MIT License" src="https://img.shields.io/badge/license-MIT-green.svg"></a>
  <img alt="Self-hosted" src="https://img.shields.io/badge/deployment-self--hosted-6f42c1">
</p>

HotKey 是一个本地优先、可自托管的 AI 热点事件监控与 Obsidian 知识库治理平台。它从 RSS、Atom、Hacker News 等合规来源采集内容，将跨来源信息聚合为事件，保留证据和判断依据，并生成日报、周报及长期知识资产。

`hotkey-server` 是 HotKey 的后端和 OpenAPI 事实源，负责采集、标准化、相关性判断、事件智能、报告发布、订阅交付、身份权限与运行治理。

> 如果 HotKey 对你的研究、内容创作或情报工作有帮助，欢迎 Star、分享使用反馈，或从一个 Issue / Pull Request 开始参与。

## 项目组成

HotKey 由两个可独立开发、组合部署的开源仓库组成：

| 仓库 | 职责 |
|------|------|
| [hotkey-server](https://github.com/StephenQiu30/hotkey-server) | 本仓库；提供后端 API、采集与 AI 任务、数据模型、交付能力和 OpenAPI 契约 |
| [hotkey-web](https://github.com/StephenQiu30/hotkey-web) | 面向最终用户的 Web 工作台，提供事件、监控、来源、证据、报告和设置界面 |

本仓库可以独立运行 API 和后台任务；完整的交互体验需要同时运行 `hotkey-web`。

## 为什么是 HotKey

很多热点工具擅长展示“什么正在流行”，却很难回答“为什么值得相信”和“后续如何沉淀”。HotKey 关注的是一条完整、可审计的工作流：

- **本地优先**：业务事实保存在自己的 PostgreSQL，原始证据进入自己的 MinIO，可阅读知识写入自己的 Obsidian Vault。
- **证据优先**：事件、声明、来源、热度和 AI 运行记录保持可追溯关系，不把模型输出当作无来源事实。
- **合规采集**：只接入官方 API、RSS、Atom 或授权 Feed，不绕过登录、验证码或平台访问限制。
- **人机协作**：AI 用于扩展检索、Embedding、实体与声明提取、摘要等环节；关键配置和知识变更保留人工审批边界。
- **从发现到交付**：同一条链路覆盖监控配置、采集、事件聚类、报告冻结、Vault 发布、邮件和 RSS/Atom 订阅。
- **面向小团队**：单仓库、单二进制、模块化单体，适合个人和 5–10 人团队自托管与持续演进。

## 核心能力

| 领域 | 已实现能力 |
|------|------------|
| 身份与权限 | 注册、登录、刷新会话、密码重置、角色权限与管理接口 |
| 监控配置 | 版本化 Monitor 草稿、预览、发布、暂停、恢复、归档和 AI 候选规则审批 |
| 来源采集 | RSS、Atom、Hacker News，来源健康检查、采集运行记录，以及批次、检查点与 River 任务的原子重试 |
| 内容与证据 | 内容标准化、去重、Markdown 文档预览、MinIO 原始证据存储 |
| 相关性与反馈 | 多语言匹配、人工反馈、评估与规则建议 |
| 热点监测 | 事件变化时间线、可解释 Radar、多条件排序筛选、低噪声站内告警与处理审计 |
| 事件智能 | 聚类、生命周期治理、热度趋势、实体、声明和证据化摘要 |
| AI Provider | OpenAI、DeepSeek、Ollama，以及可选的本地 ONNX Embedding |
| 知识与报告 | Obsidian 提案/审批/对账、日报周报构建、预览、冻结和发布 |
| 交付与运维 | SMTP、私有 RSS/Atom、River 任务、Prometheus、OpenTelemetry 和运行治理接口 |

开发环境还提供自托管 Swagger UI（`/docs`）和 OpenAPI 文档（`/openapi.json`）。

热点监测链路以 Event 而不是单篇文章为中心：每次 24 小时热度重算会确定性判断是否形成一条不可变 EventUpdate；Radar 按 attention、momentum、breadth、latest 或 Monitor relevance 返回最多 100 个可解释事件；达到已发布 Monitor 阈值的实质变化进入站内 Alert Thread。告警支持 acknowledge、resolve、suppress 和乐观锁审计，重复任务与并发 Worker 不会重复创建事实。AI Provider 缺失或暂时不可用时，变化检测、Radar 和告警仍然运行，摘要则明确降级为基于证据的结果。

对应的认证 API 是 `GET /api/v1/radar/events`、`GET /api/v1/events/{id}/updates`、`GET /api/v1/alerts`、`GET /api/v1/alerts/{id}` 及三个告警状态动作；准确参数与响应以 OpenAPI 为准。

## 工作方式

```mermaid
flowchart LR
    A["RSS / Atom / HN"] --> B["采集与标准化"]
    B --> C["相关性与去重"]
    C --> D["事件聚类与热度"]
    D --> E["变化时间线与 Radar"]
    E --> F["低噪声告警"]
    E --> G["实体、声明与摘要"]
    G --> H["日报 / 周报"]
    H --> I["Obsidian Vault"]
    H --> J["Email / RSS / Atom"]
    B -. "原始证据" .-> K["MinIO"]
    B -. "业务事实" .-> L["PostgreSQL + pgvector"]
```

服务以同一个 Go 二进制运行，可选择：

- `all`：API 与 worker 同进程，适合本地体验和小规模部署。
- `api`：只提供 HTTP API。
- `worker`：只执行采集、AI、报告和投递任务。

## 快速开始

### 环境要求

- Go 1.26+
- PostgreSQL 16+ 与 pgvector
- Redis 7+
- MinIO
- 可选：SMTP、OpenAI / DeepSeek API、Ollama、ONNX Runtime

### Docker Compose 启动

日常环境使用 `.env`：

```bash
cp .env.example .env
docker compose -f docker-compose-env.yml up --build -d
```

生产环境使用独立的 `.env.prod`，其中 6 个必填空值必须先填写：

```bash
cp .env.prod.example .env.prod
docker compose --env-file .env.prod -f docker-compose-prod.yml up --build -d
```

日常文件会发布 Server、PostgreSQL、Redis 和 MinIO 端口；生产文件只发布 Server 的 `8080` 端口。`--env-file .env.prod` 只为基础设施映射必要凭据，Server 仍固定读取 `.env.prod`。上游镜像使用 `latest` 浮动标签；pgvector 官方没有 `latest`，因此使用兼容现有数据卷的浮动 `pg16` 标签。两份文件都会初始化空数据库和 MinIO Bucket，已有兼容数据库只执行验证。生产 HTTP 需要由外部反向代理提供 HTTPS。

### 1. 获取代码与配置

```bash
git clone https://github.com/StephenQiu30/hotkey-server.git
cd hotkey-server
cp .env.example .env
```

编辑 `.env`，至少配置专用 PostgreSQL、MinIO、Redis、精确 CORS Origin，以及两个各不少于 32 字节的随机密钥：

```dotenv
HOTKEY_DATABASE_URL=postgres://hotkey:hotkey@localhost:5432/hotkey?sslmode=disable
HOTKEY_JWT_SECRET=
HOTKEY_VERIFICATION_HMAC_SECRET=
HOTKEY_CORS_ALLOWED_ORIGINS=http://localhost:3000
```

请在本地为两个密钥填写各自独立、随机且不少于 32 字节的值。常用配置与可选 Provider 示例见 [`.env.example`](.env.example)；未列出的配置使用程序默认值，真实密钥不要提交到仓库。

注册和密码重置依赖兼容 SMTP 的邮件服务；需要启用这些功能时，请按 [`.env.example`](.env.example) 填写服务商地址、TLS 模式、账号、授权码和发件人信息。部署、诊断和生产检查见 [运行与升级手册](docs/operations/README.md)。

### 2. 初始化数据库

当前完整结构由 [`db/schema.sql`](db/schema.sql) 统一维护。请使用新的空数据库初始化：

```bash
go run ./cmd/hotkey db init --empty-only --confirm-empty
go run ./cmd/hotkey db verify
```

### 3. 启动后端

GoLand 直接运行 `cmd/hotkey` 的 `main` 包即可；命令行等价入口只有一个：

```bash
go run ./cmd/hotkey
```

首个用户通过 Web 正常完成邮箱验证和注册，默认角色为 viewer；随后由数据库操作员按 [首管理员数据库指定操作](docs/operations/009-首管理员数据库指定操作.md) 的一次性事务将该用户提升为 admin。启动过程不读取管理员邮箱或密码，也不需要额外的用户引导命令。

确认服务可用：

```bash
curl --fail http://127.0.0.1:8080/healthz
curl --fail http://127.0.0.1:8080/readyz
```

随后可访问：

- Swagger UI：<http://127.0.0.1:8080/docs>
- OpenAPI：<http://127.0.0.1:8080/openapi.json>
- Prometheus Metrics：<http://127.0.0.1:8080/metrics>

> 生产环境设置 `HOTKEY_ENV=production` 后，服务会在 `.env` 基础上覆盖读取 `.env.prod`；进程环境变量优先级最高。生产环境不会开放 Swagger UI 和 OpenAPI 路由。

### GoLand 运行与调试

使用 GoLand 打开本仓库后，直接运行 `cmd/hotkey` 的 `main` 包即可。工作目录使用仓库根目录，以便读取当前 `.env`；运行配置保存在个人 IDE 中，无需提交共享配置。GoLand 会使用 `go.mod` 中声明的 Go SDK 和依赖，代码格式由 [`.editorconfig`](.editorconfig) 统一管理。

## Web 工作台

推荐搭配 [hotkey-web](https://github.com/StephenQiu30/hotkey-web) 使用。Web 端提供热点总览、监控主题、来源管理、内容证据、事件智能、报告和通知配置等完整界面，并通过生成的 OpenAPI Client 与本服务保持契约一致。后端与 Web 分别启动和部署，不依赖 `.sh` 启动脚本。

## 开发与质量

后端采用单模块、单二进制的模块化单体结构：

```text
cmd/hotkey/                # 唯一可执行入口与命令行
internal/bootstrap/        # Fx 装配、角色与生命周期
internal/modules/          # 按业务域拆分的应用、领域、基础设施和 HTTP 传输
internal/platform/         # 数据库（含具体仓储）、HTTP、队列、配置、日志和可观测适配
internal/shared/           # 跨模块稳定错误、分页/仓储契约和基础类型
db/                        # 唯一完整 Schema 与嵌入资源
test/                      # 集中测试、镜像 testdata、runner 和架构门禁
docs/                      # Design、PRD、Plan、Acceptance 与 Operations
tools/                     # 构建期工具依赖
```

业务代码保持在 `internal/`，防止被仓库外部误用；当前没有跨项目复用的公共 Go 库，因此不创建无实际职责的 `pkg/`。所有运行角色继续由 `cmd/hotkey` 的 `all`、`api`、`worker` 参数提供，不增加重复入口。

`internal/shared` 不依赖 ORM、数据库驱动或 `internal/platform`；GORM CRUD 和 PostgreSQL 错误映射集中在 `internal/platform/database/repository`，业务模块的基础设施层向内依赖 shared 契约。

测试源码和纯测试 fixture 统一位于 `test/_suite`的包镜像路径；`test/runner` 在执行期间临时物化 `_test.go` 和包内 `testdata`，退出后清理，因此 `internal/` 生产树不保存纯测试资产。

该结构不是照搬某个所谓“标准模板”，而是结合真实项目取舍：

- [golang-standards/project-layout](https://github.com/golang-standards/project-layout) 明确声明自身不是 Go 官方标准，并建议只保留项目真正需要的部分。
- [ardanlabs/service](https://github.com/ardanlabs/service) 证明生产服务可以按应用、业务和基础能力分层，而不必固定使用 `internal/modules` 这一种命名。
- [ThreeDotsLabs/wild-workouts-go-ddd-example](https://github.com/ThreeDotsLabs/wild-workouts-go-ddd-example/tree/master/internal) 展示了按业务域组织 application、domain、ports 与 adapters 的实践，与本项目的模块边界最接近。
- [Grafana](https://github.com/grafana/grafana/tree/main/pkg/services)、[Gitea](https://github.com/go-gitea/gitea/tree/main/services) 和 [Prometheus](https://github.com/prometheus/prometheus) 使用不同的服务目录布局，说明大型 Go 项目并不存在唯一目录答案。

GoLand 共享运行配置采用 [JetBrains 官方 Run/Debug 配置模型](https://www.jetbrains.com/help/go/run-debug-configuration.html)：使用 `DIRECTORY` 运行类型，以 `$PROJECT_DIR$/cmd/hotkey` 为运行目录、仓库根目录为工作目录，避免依赖个人项目模块名。

常用命令：

```bash
make lint       # 静态检查
make test       # 单元与集成测试
make build      # 构建二进制
make validate   # 架构与仓库约束
make ci         # 完整质量门禁
```

完整 CI 需要可丢弃的 PostgreSQL 测试库和独立 Redis DB：

```bash
HOTKEY_TEST_DSN='postgres://hotkey:hotkey@localhost:5432/hotkey_test?sslmode=disable' \
HOTKEY_TEST_REDIS_URL='redis://127.0.0.1:6379/15' \
make ci
```

GitHub Actions 对 `main` 和 Pull Request 执行同一套门禁，包括 OpenAPI 漂移、数据库运行验证、测试、构建、Schema 与架构检查。

## 项目状态

HotKey 正处于积极开发阶段，核心端到端链路已经实现，接口和部署方式在 1.0 前仍可能调整。当前更适合技术预览、自托管评估和共同建设，而不是直接作为无人值守的关键生产系统。

已完成工作的长期证据位于 [`docs/acceptance/archive/`](docs/acceptance/archive/README.md)。路线与功能建议通过 [GitHub Issues](https://github.com/StephenQiu30/hotkey-server/issues) 公开讨论；外部依赖和生产备份恢复仍需在具体部署环境按 [Operations 手册](docs/operations/README.md) 演练。

## 文档导航

- [架构与设计](docs/design/README.md)
- [产品需求](docs/prd/README.md)
- [实施计划](docs/plans/README.md)
- [验收证据](docs/acceptance/README.md)
- [运行与升级手册](docs/operations/README.md)
- [OpenAPI JSON](docs/openapi/swagger.json)

## 参与贡献

欢迎提交 Bug、使用场景、连接器建议、文档改进和 Pull Request。开始前请阅读：

- [贡献指南](CONTRIBUTING.md)
- [安全策略](SECURITY.md)

大型功能建议先创建 Issue 对齐问题、边界和验收标准。涉及来源连接器时，请同时说明数据来源的官方或授权访问方式。

## 许可证

本项目基于 [MIT License](LICENSE) 开源。
