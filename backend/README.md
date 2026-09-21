# HotKey Backend

Python 3.12、FastAPI、Uvicorn、Pydantic 2、SQLAlchemy 2、psycopg 3、PostgreSQL、Redis、Kafka、MinIO。

采用按业务领域分组的模块化单体。项目架构、目录、API 契约和数据库事实源统一执行根目录 [PROJECT.md](../PROJECT.md)；实现门禁和验证命令执行 [AGENTS.md](../AGENTS.md#fastapi-目录与命名必须执行)。

## 执行入口

复制 `.env.example` 为未跟踪的 `.env` 并填写数据库凭据。在 `backend/` 执行 `uv sync --locked`；从 `backend/app/` 执行以下独立入口：

```bash
uv run --locked uvicorn main:create_app --factory
uv run --locked python -m worker
uv run --locked python -m cli
```

API、Worker 和 CLI 分别启动。应用启动不会创建或修改数据库结构。

## 数据库结构

`database/schema.sql` 是唯一 DDL 事实源；SQLAlchemy Model 只负责运行时映射。每次数据结构变更必须在同一提交中更新 SQL、Model 与真实 PostgreSQL 验证。

该文件只允许写入新建空库，并自行包含完整事务边界：

```bash
psql -X --set ON_ERROR_STOP=on \
  --dbname 'postgresql://USER:PASSWORD@HOST:5432/DATABASE' \
  --file database/schema.sql
```

当前不支持对存量数据库自动就地升级。需要保留数据时，先完成备份与恢复演练，再新建数据库、应用完整 `schema.sql` 并导入经过校验的数据；禁止对现有旧库直接执行该文件。

仓库根 `compose.yaml` 是唯一编排。复制根 `.env.example` 为未跟踪的 `.env` 并设置 URL-safe 数据库密码后，从根目录执行：

```bash
docker compose config --quiet
docker compose build
docker compose up --detach --wait
```

Compose 仅在全新 PostgreSQL 数据卷中通过官方初始化目录运行 `schema.sql`；普通停止不删除持久卷。Worker 与 CLI 是按需 profile，分别使用 `docker compose run --rm worker` 和 `docker compose run --rm cli`。验证直接使用 Compose 与各依赖官方 CLI，不建立额外脚本。

业务接口统一使用 `/api` 命名空间，例如存活检查 `/api/health`、就绪检查 `/api/ready`；接口文档入口为 Swagger UI `/docs`、Scalar `/scalar`，共用 `/openapi.json`。

## 状态

当前底座包含应用工厂、数据库会话、唯一 `schema.sql`、结构化日志、健康检查、Swagger UI、Scalar、Kafka Worker、管理 CLI 和架构测试。业务领域只在对应切片完成 Design 登记后创建，当前不保留空的未来领域包。
