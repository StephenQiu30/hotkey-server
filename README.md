# Ripplesight

Ripplesight 是面向个人非商业使用的**公开资讯阅读与舆情监控项目**。围绕关键词连接来源材料、讨论与事件进展，让正在发生的变化有依据、可追溯。

GitHub 仓库：[StephenQiu30/Ripplesight](https://github.com/StephenQiu30/Ripplesight)。本机工作区为 `Ripplesight/`，当前仓库为其中的 `ripplesight-server/`，冻结的客户端为 `ripplesight-app/`。Compose 项目、容器、网络与自有镜像统一使用 `ripplesight`，生产项目为 `ripplesight-prod`。配置使用 `HOTKEY_*` 环境变量，业务库为 `hotkey`。

- 不登录：阅读有出处、经过去重的公开资讯、事件、日报周报，以及 AI 模型榜。
- 登录后：配置关键词和来源，查看已采集的帖子、评论与任务状态。来源、情感分析、报告与告警的可用范围以实际配置和验收为准，不代表各平台已全部接通。

原始需求按编号逐项整理在 [requirement 目录](docs/index.md#需求目录)，视觉和交互按同编号、同名称的 [设计文档](docs/index.md#设计主题) 维护。

页面与功能需求见 [编号需求](docs/index.md#需求目录)，执行顺序见 [编号计划](docs/index.md#开发阶段)，未完成事项见 [BACKLOG](BACKLOG.md)。

## 快速开始

需要 Docker Compose 2.24.4+，以及已经在运行的 PostgreSQL、Redis、Kafka。如果没有，见下文「全新环境」。

```bash
cp .env.example .env
```

编辑 `.env`，填写数据库、Redis、Kafka 的连接地址和各项密钥。然后启动：

```bash
docker compose --env-file .env -f docker-compose.yml up --detach --build --wait backend frontend
```

- 网站：<http://127.0.0.1:8666/>
- API 文档：<http://127.0.0.1:8667/docs>

前后端由 Docker 构建镜像运行，复用本机已有的数据服务。修改源码或依赖后需要重新构建对应服务。需要热更新时，按 [backend](backend/README.md) 或 [frontend](frontend/README.md) 的本地运行说明启动开发服务。项目文档在 docs 中编辑，Obsidian 打开同一目录。

需要重新构建依赖并重启前后端时：

```bash
docker compose --env-file .env -f docker-compose.yml up --detach --build --force-recreate --wait backend frontend
```

启动后台任务。Worker 建议跑在宿主机上，见 [backend/README](backend/README.md)；Scheduler 可以用 Compose 启动：

```bash
docker compose --env-file .env -f docker-compose.yml --profile worker up --detach scheduler
```

## 全新环境

只有在没有可复用的 PostgreSQL、Redis、Kafka 时才执行：

```bash
docker compose --env-file .env -f docker-compose-env.yml up --detach --wait
```

它只会在全新的空数据卷上初始化 `backend/sql/schema.sql`。不要在已有业务库上执行这个脚本，升级方法见 [PROJECT §6](PROJECT.md#6-数据库)。

新数据卷以当前 Compose 项目名为前缀。复用已有环境卷时，在 `.env` 中将 `HOTKEY_POSTGRES_VOLUME_NAME`、`HOTKEY_REDIS_VOLUME_NAME`、`HOTKEY_KAFKA_VOLUME_NAME` 设置为原卷名，保持现有数据存储。

## 生产

```bash
cp .env.example .env.prod
```

编辑 `.env.prod`，设置 `HOTKEY_ENVIRONMENT=production`、HTTPS 的 `HOTKEY_WEB_ORIGIN`，以及独立的生产密钥。然后启动：

```bash
docker compose --env-file .env.prod -f docker-compose-prod.yml up --detach --build --wait
```

API 和 Web 只绑定 localhost，需要通过反向代理对外提供访问。

停止服务时，使用与启动时相同的文件和参数执行 `down`。不要加 `--volumes`，否则会删除数据。

## 文档

项目文档在 [docs](docs/index.md) 下使用 Markdown/Git 管理，Obsidian 直接打开 `docs/`。按编号维护需求、设计、产品与计划，以及可选模板、文档视图与共享配置，不保留历史文档或归档副本。业务报告的 Obsidian 导出独立于项目文档。

| 文档 | 内容 |
|---|---|
| [产品定位](docs/prd/00-产品定位.md) | 用户与产品价值 |
| [编号需求](docs/index.md#需求目录) | 页面与功能需求 |
| [编号设计](docs/index.md#设计主题) | 视觉、组件、布局与交互规范 |
| [编号计划](docs/index.md#开发阶段) | 当前执行顺序与交付门槛 |
| [文档工作区](docs/index.md) | 编号目录、Obsidian 使用与链接检查 |
| [BACKLOG](BACKLOG.md) | 优先级与进度 |
| [PROJECT](PROJECT.md) | 技术架构与约定 |
| [AGENTS](AGENTS.md) | 工程规范与检查 |
| [backend](backend/README.md) / [frontend](frontend/README.md) | 本地开发与测试 |

参与开发见 [CONTRIBUTING](CONTRIBUTING.md)。本项目采用 [MIT 许可证](LICENSE)，复用第三方代码时须保留其许可证与作者署名。
