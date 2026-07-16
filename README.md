# HotKey Server

[![CI](https://github.com/StephenQiu30/hotkey-server/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/StephenQiu30/hotkey-server/actions/workflows/ci.yml)

HotKey Server 是一个本地优先、面向个人与小团队的 AI 热点事件监控和 Obsidian 知识库治理后端。

项目当前正在按 `docs/design/001`–`015` 进行绿色重建。旧的 Kafka、Redis 核心任务、Topic/Event/HotEvent 重复模型、Git 知识库提交链路及旧数据库结构已经移除，不提供兼容运行路径。

## 目标能力（不等同于当前实现）

- 通过 RSS/Atom、GDELT/新闻 API、Reddit、Hacker News、YouTube 和 X 官方 API 采集候选内容
- 完成标准化、去重、多语言相关性判断、事件聚类、热度与趋势分析
- 将原始证据写入 MinIO，将可阅读知识投影写入本地 Obsidian Vault
- 生成日报、周报，并通过邮件和 RSS/Atom 交付
- 使用 PostgreSQL 保存业务事实和 River 后台任务状态
- 使用同一个 Go 二进制以 `all`、`api` 或 `worker` 角色运行

## 架构约束

- 模块化单体，不拆分微服务
- PostgreSQL 是唯一业务事实源
- 业务模块采用 `domain -> application -> transport/infrastructure` 边界
- 每张业务表提供统一 Repository CRUD，公共 API 仅开放安全业务操作
- 知识库运行链路不依赖 Git
- 当前阶段不提供 Docker 或线上部署编排

## 当前交付状态

已完成并有长期验收证据的交付是：模块化单体与工程门禁、单一 Schema/数据库平台、统一 HTTP 契约与观测基础、身份认证/会话/权限、版本化 Monitor 与 Source 配置，以及查询规划与 RSS/Atom/Hacker News 的共享采集运行（PRD/Plan 001–006）。对应证据见 [Acceptance 索引](docs/acceptance/README.md)。

Content 标准化、去重、MinIO、事件、热度、AI、知识、报告和投递仍是后续目标；治理任务 018 是已审核的非阻塞 `ready` 支线，PRD/Plan 006 已归档为 `archived` / `done`，007–017 仍处于 `review` / `backlog`（007 仅成为自身审核后的 ready 候选），不得把本节“目标能力”描述为已上线能力。唯一的任务状态事实源是 [PRD 索引](docs/prd/README.md) 和 [Plan 索引](docs/plans/README.md)；设计边界见 [设计索引](docs/design/README.md)。

## 本地启动

服务只读取两个配置文件：默认 `.env`，以及在 `HOTKEY_ENV=production` 时覆盖读取的 `.env.prod`。进程环境变量优先级最高。JWT 与认证 HMAC 没有不安全默认值；每个环境都必须填入自己的至少 32 字节的 `HOTKEY_JWT_SECRET` 与 `HOTKEY_VERIFICATION_HMAC_SECRET`，并配置精确的 `HOTKEY_CORS_ALLOWED_ORIGINS`、Redis URL 和专用 PostgreSQL 库。

现有旧库不能作为当前绿色重建的运行库。创建独立空库后，只能通过显式 empty-only 初始化写入 schema：

```bash
createdb hotkey_server_dev
HOTKEY_DATABASE_URL='postgres://USER@localhost:5432/hotkey_server_dev?sslmode=disable' \
  go run ./cmd/hotkey db init --empty-only --confirm-empty
go run ./cmd/hotkey
```

启动后的最小验证：

```bash
curl --fail http://127.0.0.1:8080/healthz
curl --fail http://127.0.0.1:8080/readyz
```

GoLand 可直接运行 `github.com/StephenQiu30/hotkey-server/cmd/hotkey`，工作目录设为 `hotkey-server` 模块目录。生产启动前设置 `HOTKEY_ENV=production`，程序便会在读取 `.env` 后用 `.env.prod` 覆盖对应值。

常用验证命令：

```bash
make lint
make test
make build
make validate
make database-runtime-verify
make ci
```

完整 Schema 的空库验证使用可丢弃的 PostgreSQL 16+ 与 pgvector 数据库：

```bash
HOTKEY_TEST_DSN='postgres://hotkey:hotkey@localhost:5432/hotkey_test?sslmode=disable' make database-runtime-verify
```

该命令会重建目标测试库的 `public` schema，再执行嵌入 Schema 的空库初始化、只读兼容性检查、真实 PostgreSQL 集成测试与游标计划验证。每个 Go 集成测试与容量 fixture 都会创建并删除自己的数据库，因此 `HOTKEY_TEST_DSN` 必须是可丢弃的 PostgreSQL URL，且其角色需要 `CREATE DATABASE` / `DROP DATABASE` 权限；容量 fixture 不与命令或测试共享数据库。运行服务时则使用 `HOTKEY_DATABASE_URL`：

```bash
HOTKEY_DATABASE_URL='postgres://hotkey:hotkey@localhost:5432/hotkey?sslmode=disable' go run ./cmd/hotkey db verify
HOTKEY_DATABASE_URL='postgres://hotkey:hotkey@localhost:5432/hotkey_new?sslmode=disable' go run ./cmd/hotkey db init --empty-only --confirm-empty
```

`make ci` 也会执行该验证，因此 CI 必须提供 `HOTKEY_TEST_DSN`。

身份认证的完整 CI 还会运行真实 Redis 验证流程。为避免污染开发验证码状态，请显式使用可丢弃的 Redis DB，并同时传入两个测试连接：

```bash
HOTKEY_TEST_DSN='postgres://hotkey:hotkey@localhost:5432/hotkey_test?sslmode=disable' \
HOTKEY_TEST_REDIS_URL='redis://127.0.0.1:6379/15' \
make ci
```

Redis 只承载验证码、验证票据和限流测试状态；用户、会话与刷新凭据的事实仍在 PostgreSQL。
身份 API 还要求设置不少于 32 字节的 `HOTKEY_VERIFICATION_HMAC_SECRET`；验证码状态使用该密钥绑定验证码、用途和规范化邮箱的 HMAC。`HOTKEY_SMTP_ENABLED=false` 时，新的邮箱验证流程会安全地返回 503，且不会投递邮件或写入验证码状态。

## 持续集成

GitHub Actions 在推送到 `main`、面向 `main` 的 Pull Request 和手动触发时运行唯一质量门禁 `make ci`。工作流使用临时 PostgreSQL+pgvector 与 Redis 服务，因此真实数据库、Schema、Redis 集成测试、OpenAPI 漂移、构建和架构校验都在同一入口内执行。可复现命令、测试数据边界和失败处理见[本地与 GitHub CI 质量门禁](docs/operations/001-本地与GitHub%20CI质量门禁.md)。

本地 Go 工具链可以放在未跟踪的 `.tools/go` 目录，或直接使用系统中的 Go 1.26+。

## 许可证

[MIT](LICENSE)
