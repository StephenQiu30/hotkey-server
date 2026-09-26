# HotKey · 知微见澜

从一个关键词观察变化：收集公开或获授权的内容，追踪主题与来源，并逐步生成可追溯的舆情日报、周报和本地知识库。

HotKey 当前面向个人、非商业研究场景；代码授权范围以 [MIT 许可证](LICENSE) 为准。本仓库包含 Python 后端和 Next.js Web 工作台；独立的 [Flutter 客户端仓库](https://github.com/StephenQiu30/hotkey-app) 目前尚未初始化。项目仍在开发中，适合试用和参与开发，完整产品验收尚未完成。

## 当前能做什么

- 单个部署者初始化账户，创建监控主题、关键词规则和来源连接。
- 使用 PostgreSQL 保存内容、任务与运行状态；Kafka Worker 执行持久任务，Web 展示主题、来源、内容、任务和报告页面。
- 已有 Hacker News、RSS、网页搜索等采集适配器及调度、分析、日报相关代码；实际来源需要单独配置和验证。
- 提供 FastAPI 自动生成的 OpenAPI、Swagger UI 与 Scalar 文档。

**当前边界：**项目正在验证真实来源采集、评论、模型标注和日报链路。邮件推送、周报、事件归并、知识库后续能力及连续运行验收尚未完成；页面、适配器、单元测试或服务健康检查不代表某来源已经通过真实业务验收。最新进度和验收状态以 [BACKLOG](BACKLOG.md) 为准。

## 快速启动本地底座

需要 Docker 与 Docker Compose。以下命令在本仓库根目录执行，会创建本地 PostgreSQL、Redis、Kafka、API 和 Web 容器；首次启动需要构建镜像。请先在**本地未跟踪**的 `.env` 中设置 URL 安全的随机数据库密码，以及至少 32 字符的临时 `HOTKEY_BOOTSTRAP_TOKEN`。不要提交 `.env` 或把密钥粘贴到 Issue。

```bash
cp .env.example .env
# 编辑 .env，设置 HOTKEY_POSTGRES_PASSWORD 和 HOTKEY_BOOTSTRAP_TOKEN
docker compose config --quiet
docker compose build
docker compose up --detach --wait
```

打开 [http://127.0.0.1:3000/register](http://127.0.0.1:3000/register)，用部署密钥初始化唯一 owner，然后到 [http://127.0.0.1:3000](http://127.0.0.1:3000) 登录。初始化完成后，从 `.env` 移除 `HOTKEY_BOOTSTRAP_TOKEN` 并执行 `docker compose up --detach --force-recreate backend`，使运行中的 API 不再持有该值。API 默认位于 `127.0.0.1:8867`，接口文档位于 `/docs` 和 `/scalar`。

Worker 是按需 profile，可在底座启动后运行：

```bash
docker compose --profile worker up --detach worker
```

这套 Compose 启动步骤只验证本地底座。周期调度与实际采集还需要按来源配置外部服务、准入与预算，并完成对应的真实验收；现有 Compose 没有独立调度服务。不要对已有业务数据库直接执行 `backend/database/schema.sql`，它仅用于**全新空库**。停止服务请用 `docker compose down`；不要随意添加 `--volumes`，这会删除本地数据卷。更详细的开发与运行说明见 [后端 README](backend/README.md) 和 [Web README](frontend/README.md)。

## 技术与文档

后端使用 Python 3.12、FastAPI、SQLAlchemy 2、PostgreSQL、Redis 和 Kafka；Web 使用 Next.js、React、TypeScript 与 pnpm。`backend/database/schema.sql` 是数据库结构事实源；API 契约由运行中的 FastAPI 生成，再生成 Web 客户端。

| 文档 | 用途 |
| --- | --- |
| [项目约束](PROJECT.md) | 架构、目录与运行边界 |
| [文档索引](docs/README.md) | 需求、设计、计划与验收记录 |
| [进度看板](BACKLOG.md) | 当前任务和真实验收状态 |
| [贡献指南](CONTRIBUTING.md) | 开发、验证与 PR 要求 |
| [安全策略](SECURITY.md) | 私密报告漏洞及敏感信息处理 |

仅采集公开或获授权的数据；请遵守来源平台规则与适用法律，并自行控制请求频率、凭据与数据保留。

## 参与项目

欢迎提交问题、文档修正与聚焦的 Pull Request。开始前请阅读 [贡献指南](CONTRIBUTING.md) 和 [进度看板](BACKLOG.md)；涉及来源接入时请说明真实可用的能力、失败状态和验证范围。安全漏洞请按 [安全策略](SECURITY.md) 私密报告。

本项目采用 [MIT 许可证](LICENSE)。
