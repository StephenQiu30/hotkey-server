# HotKey Server

HotKey 是个人创作者的热点监控与 AI 选题助手。`hotkey-server` 是整个产品的 **Go 后端服务**，也是 **OpenAPI 契约的唯一事实源**——Web 工作台与小程序客户端都从这里生成 API 类型与调用代码。

## 产品定位

HotKey 帮助内容创作者：

- 从多平台信息源持续采集与聚合热点
- 用 AI 快速理解事件、生成摘要与选题建议
- 通过榜单、日报、邮件与通知及时跟进值得创作的话题

本仓库负责把上述能力落地为可部署、可观测、可扩展的后端系统。

## 核心能力

| 领域 | 说明 |
|------|------|
| 账号与授权 | 邮箱注册登录、会话刷新；多平台 OAuth 授权托管（X、YouTube、B 站、微博、小红书、知乎等） |
| 监控配置 | 频道、关键词、监控主题、用户偏好与 RSS 订阅 |
| 内容采集 | 平台适配器 + 公开源采集（Reddit、Hacker News、新闻 RSS、微信公众号等） |
| 数据处理 | 内容标准化、去重、相似度筛选、Embedding 与热点聚类评分 |
| AI 能力 | 事件摘要、时间线、日报/周报生成、选题建议 |
| 触达与同步 | SMTP 邮件投递、Obsidian Git 同步、站内通知 |
| 存储与治理 | PostgreSQL + pgvector、Redis 任务队列、MinIO 对象存储、数据最小化留存 |
| 运维管理 | 管理员 API、审计日志、任务队列观测、用户撤权与禁用 |

完整接口定义见 [`docs/openapi.yaml`](./docs/openapi.yaml)。

## 技术栈

- **语言与框架**：Go 1.25、Gin
- **数据层**：PostgreSQL（pgvector）、Redis
- **对象存储**：MinIO（可选）
- **AI**：DashScope / Qwen（Embedding 与摘要）
- **邮件**：SMTP
- **编排**：Symphony + Linear 工作流（见 [`WORKFLOW.md`](./WORKFLOW.md)）

## 快速开始

### 环境要求

- Go 1.25+
- PostgreSQL 16（建议启用 pgvector）
- Redis 7+
- Python 3（仓库治理与 WORKFLOW 契约测试）

### 本地运行

先在本机启动 PostgreSQL 与 Redis，再复制环境变量并按 `.env.example` 填写本机连接信息：

```bash
cp .env.example .env
# 编辑 .env：确认 HOTKEY_DATABASE_URL、HOTKEY_REDIS_URL 指向本机服务

make test
make run

curl http://127.0.0.1:8080/healthz
```

### Docker Compose（本地开发）

PostgreSQL、Redis、MinIO **不在 compose 中启动**，需在本机先行运行；`docker-compose.yml` 仅拉起 API 与 Web 容器，并通过 `host.docker.internal` 连接宿主机服务。

```bash
cp .env.example .env
# 确认 POSTGRES_*、REDIS_* 与本机服务一致

make compose-up    # 构建并启动 server + web
make compose-logs  # 查看日志
make compose-down  # 停止
```

- API：`http://127.0.0.1:8080`
- Web：`http://127.0.0.1:3000`

### Docker Compose（线上 / 自托管全栈）

`docker-compose.prod.yml` 会一并启动 PostgreSQL（pgvector）、Redis、MinIO、n8n 以及 API / Web 应用，适合单机自托管或线上环境。

```bash
cp .env.prod.example .env.prod
# 编辑 .env.prod：替换所有 replace-with-* 占位符，设置 PUBLIC_API_URL / PUBLIC_WEB_URL

make compose-prod-up    # 构建并启动全栈
make compose-prod-logs  # 查看日志
make compose-prod-down  # 停止（数据卷保留）
```

| 服务 | 默认端口 | 说明 |
|------|----------|------|
| API | 8080 | `WEB_PUBLISHED_PORT` |
| Web | 3000 | `WEB_FRONTEND_PORT` |
| n8n | 5678 | `N8N_PUBLISHED_PORT` |
| PostgreSQL / Redis / MinIO | 仅容器内网 | 不映射宿主机端口 |

首次启动会自动执行 `db/schema.sql` 初始化数据库，并创建 MinIO 存储桶。

### E2E 测试

```bash
make e2e-up      # 启动 Postgres + Redis 测试环境
make e2e-smoke   # 冒烟测试
make e2e-test    # 完整 E2E
make e2e-down    # 清理环境
```

## 仓库结构

```text
cmd/hotkey-api/     # 服务入口
internal/           # 领域逻辑、HTTP 处理器、平台适配器、调度与 Worker
docs/               # OpenAPI、PRD、计划与工程文档
db/                 # 数据库 schema 与迁移
tests/              # 契约测试与 E2E
```

## 跨仓协作

HotKey 由三个独立仓库组成，默认开发顺序为：

```text
server → web → miniapp → 回归
```

后端接口变更时，**先稳定本仓 OpenAPI**，再通知前端仓库执行 `openapi:generate` 重新生成客户端。不要在 Web 或小程序仓库手写后端类型。

| 仓库 | 职责 |
|------|------|
| [hotkey-server](https://github.com/StephenQiu30/hotkey-server) | 后端 API、采集、AI、榜单、通知（本仓） |
| [hotkey-web](https://github.com/StephenQiu30/hotkey-web) | Next.js 创作者工作台 |
| [hotkey-miniapp](https://github.com/StephenQiu30/hotkey-miniapp) | Taro 微信小程序轻量端 |

## 文档与协作

- [文档中心](./docs/README.md)
- [环境变量说明](./.env.example)
- [数据库说明](./db/README.md)
- [运维手册](./docs/operations/README.md)
- [CLAUDE.md](./CLAUDE.md) — Agent 协作规范
- [WORKFLOW.md](./WORKFLOW.md) — Symphony / Linear 调度契约
- [OpenSpec 规范](./openspec/specs/) — SDD 规范层（Markdown/YAML，无 Node 依赖）

## 许可证

本项目为 HotKey 产品私有仓库，未经授权请勿对外分发。
