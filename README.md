# HotKey

[![CI](https://github.com/StephenQiu30/hotkey-server/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/StephenQiu30/hotkey-server/actions/workflows/ci.yml)

HotKey 采用单体仓库管理 Go Core、Python 数据分析 Agent 与 Web 工作台。

| 目录 | 内容 |
| --- | --- |
| [`backend/`](backend/README.md) | Go 后端、PostgreSQL Schema、采集与 AI 任务、运行时 OpenAPI 和镜像配置 |
| [`agent/`](agent/README.md) | Python 数据分析服务、版本化内部契约和独立安全边界 |
| [`frontend/`](frontend/README.md) | Next.js Web 工作台、页面组件和生成的 API 客户端 |
| [`docs/`](docs/README.md) | 不按应用分层的统一正式文档与 OpenAPI 发布契约 |

## 运行与本机测试

项目运行固定使用根目录 Docker Compose；Go API/Worker、Python Agent、Next.js、PostgreSQL、Redis 与 MinIO 不作为本机直启进程分别维护：

```bash
git clone https://github.com/StephenQiu30/hotkey-server.git
cd hotkey-server
cp .env.example .env
cp backend/.env.example backend/.env
docker compose -f docker-compose.yml up --build --detach --wait --wait-timeout 240
```

日常开发验证直接使用本机已安装的 Go、Python/uv 和 Node 工具链；后端集成测试复用已经存在的可丢弃 PostgreSQL/Redis 测试实例。格式、静态、单元、集成或生成检查不启动 API、Worker、Agent、Frontend，也不为每轮测试执行 `docker compose up/down`。只有 Compose 配置、新鲜容器 E2E、发布和恢复演练会创建隔离容器栈。

默认 Compose 对外提供 Web `http://127.0.0.1:8010` 与 Go API `http://127.0.0.1:8866`；Agent 只在内部网络提供分析服务，不接收业务存储或来源凭据。

登录后，管理员可在“来源管理”中直接填写和轮换第三方来源凭据。来源参数与加密密文以 PostgreSQL 为事实源；`.env` 只保留数据库、身份、存储等部署配置和独立的 `HOTKEY_SOURCE_CREDENTIAL_MASTER_KEY`。旧 `env:NAME` 来源继续兼容，任何读取接口都不会返回凭据明文或引用。

Compose 的 `db-init` 一次性服务使用唯一 [`backend/db/schema.sql`](backend/db/schema.sql) 初始化空库并在 API 接流量前完成。它拒绝覆盖非空或不兼容数据库；现有数据的备份、重建与恢复必须按 Operations 手册在隔离环境执行。

## Docker Compose 部署与隔离验收

Docker Compose 是完整项目唯一运行入口。根 `docker-compose.yml` 统一定义前端、Go Core、内部 Python Agent、PostgreSQL、Redis、MinIO、默认环境配置、健康检查和卷；Agent 不发布宿主机端口，生产文件只覆盖生产环境差异。

容器使用固定名称 `hotkey-postgres`、`hotkey-redis`、`hotkey-minio`、`hotkey-minio-init`、`hotkey-db-init`、`hotkey-agent`、`hotkey-api` 和 `hotkey-web`，不带 `-1`。同一主机运行多套时通过 `HOTKEY_CONTAINER_PREFIX` 设置唯一前缀。

默认容器环境：

```bash
cp backend/.env.example backend/.env
cp .env.example .env
docker compose -f docker-compose.yml up --build --detach --wait --wait-timeout 240
curl --fail --silent http://127.0.0.1:8866/readyz
curl --fail --silent http://127.0.0.1:8866/metrics | grep 'hotkey_runtime_metrics_collection_success'
```

生产环境：

```bash
cp .env.prod.example .env.prod
docker compose --env-file .env.prod -f docker-compose-prod.yml up --build --detach --wait --wait-timeout 240
```

公共服务、初始化命令、依赖和健康检查均在默认 Compose 文件中完整定义，生产文件只覆盖镜像、凭据和运行环境。子项目目录不维护 Compose；Dockerfile 与 `.dockerignore` 仍跟随各自应用，以保持最小构建上下文。

## 质量检查

```bash
cd backend
make openapi  # 契约变更时重新生成后端与发布 OpenAPI
make ci       # 后端完整验收；需要隔离的 PostgreSQL 与 Redis 测试环境

cd ../frontend
npm run openapi:check
npm run typecheck
npm run test:unit
npm audit --omit=dev --audit-level=high
npm run build

cd ../agent
uv sync --all-extras --locked
uv run ruff format --check .
uv run ruff check .
uv run mypy src
uv run pytest
uv run pip-audit
```

后端 OpenAPI 发布契约位于 [`docs/openapi/swagger.json`](docs/openapi/swagger.json)，运行时注册代码位于 `backend/openapi/docs.go`，生成的前端客户端位于 `frontend/src/services/hotkey/hotkey-server/`。

重新立项后的部署、恢复、来源故障和容量契约见 [Operations 手册](docs/operations/README.md)。其中标为 `planned` 的手册尚未通过新基线验收，不得直接作为生产操作依据；生产更新前必须完成恢复演练，回滚不删除事实、任务、审计或持久卷。

贡献和安全报告请参阅 [CONTRIBUTING.md](CONTRIBUTING.md) 与 [SECURITY.md](SECURITY.md)。
