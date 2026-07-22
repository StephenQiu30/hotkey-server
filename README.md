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
| 来源采集 | RSS、Atom、Hacker News，来源健康检查、采集运行记录和可靠重试 |
| 内容与证据 | 内容标准化、去重、Markdown 文档预览、MinIO 原始证据存储 |
| 相关性与反馈 | 多语言匹配、人工反馈、评估与规则建议 |
| 事件智能 | 聚类、生命周期治理、热度趋势、实体、声明和证据化摘要 |
| AI Provider | OpenAI、DeepSeek、Ollama，以及可选的本地 ONNX Embedding |
| 知识与报告 | Obsidian 提案/审批/对账、日报周报构建、预览、冻结和发布 |
| 交付与运维 | SMTP、私有 RSS/Atom、River 任务、Prometheus、OpenTelemetry 和运行治理接口 |

开发环境还提供自托管 Swagger UI（`/docs`）和 OpenAPI 文档（`/openapi.json`）。

## 工作方式

```mermaid
flowchart LR
    A["RSS / Atom / HN"] --> B["采集与标准化"]
    B --> C["相关性与去重"]
    C --> D["事件聚类与热度"]
    D --> E["实体、声明与摘要"]
    E --> F["日报 / 周报"]
    F --> G["Obsidian Vault"]
    F --> H["Email / RSS / Atom"]
    B -. "原始证据" .-> I["MinIO"]
    B -. "业务事实" .-> J["PostgreSQL + pgvector"]
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

### 1. 获取代码与配置

```bash
git clone https://github.com/StephenQiu30/hotkey-server.git
cd hotkey-server
cp .env.example .env
```

编辑 `.env`，至少配置专用 PostgreSQL、MinIO、Redis、精确 CORS Origin，以及两个各不少于 32 字节的随机密钥：

```dotenv
HOTKEY_DATABASE_URL=postgres://hotkey:hotkey@localhost:5432/hotkey?sslmode=disable
HOTKEY_JWT_SECRET=replace-with-your-random-secret-at-least-32-bytes
HOTKEY_VERIFICATION_HMAC_SECRET=replace-with-another-random-secret-32-bytes
HOTKEY_CORS_ALLOWED_ORIGINS=http://localhost:3000
```

完整变量、默认开发值和可选 Provider 配置见 [`.env.example`](.env.example)。真实密钥不要提交到仓库。

### 2. 初始化数据库

当前完整结构由 [`db/schema.sql`](db/schema.sql) 统一维护。请使用新的空数据库初始化：

```bash
go run ./cmd/hotkey db init --empty-only --confirm-empty
go run ./cmd/hotkey db verify
```

### 3. 创建管理员并启动

先在 `.env` 中临时设置 `HOTKEY_BOOTSTRAP_ADMIN_EMAIL` 和 `HOTKEY_BOOTSTRAP_ADMIN_PASSWORD`：

```bash
go run ./cmd/hotkey user bootstrap-admin
go run ./cmd/hotkey
```

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

## Web 工作台

推荐搭配 [hotkey-web](https://github.com/StephenQiu30/hotkey-web) 使用。Web 端提供热点总览、监控主题、来源管理、内容证据、事件智能、报告和通知配置等完整界面，并通过生成的 OpenAPI Client 与本服务保持契约一致。

## 开发与质量

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

已完成工作的验收证据位于 [`docs/acceptance/archive/`](docs/acceptance/archive/README.md)。外部 MinIO、SMTP 以及生产备份恢复仍需在具体部署环境按 [Operations 手册](docs/operations/README.md) 演练。

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
- [行为准则](CODE_OF_CONDUCT.md)
- [安全策略](SECURITY.md)

大型功能建议先创建 Issue 对齐问题、边界和验收标准。涉及来源连接器时，请同时说明数据来源的官方或授权访问方式。

## 许可证

本项目基于 [MIT License](LICENSE) 开源。
