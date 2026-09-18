# HotKey Backend

Python 3.12、FastAPI、Uvicorn、Pydantic 2、SQLAlchemy 2、psycopg 3、Alembic、PostgreSQL、Redis、Kafka、MinIO。

采用按业务领域分组的模块化单体。架构、目录、事务、依赖方向与验收规则统一执行根目录 [AGENTS.md](../AGENTS.md#fastapi-目录与命名必须执行)。

## 执行入口

复制 `.env.example` 为未跟踪的 `.env` 并填写数据库凭据。在 `backend/` 执行 `uv sync --locked`；从 `backend/src/` 执行以下独立入口：

```bash
uv run --locked uvicorn main:create_app --factory
uv run --locked python -m worker
uv run --locked python -m cli
```

API、Worker 和 CLI 分别启动。数据库迁移作为独立步骤执行。

接口文档入口为 Swagger UI `/docs`、Scalar `/scalar`，共用 `/openapi.json`。

## 状态

后端底座已建立应用工厂、数据库会话、Alembic、结构化日志、健康检查、Swagger UI、Scalar、Kafka Worker、管理 CLI 和架构测试。各业务领域已固定包边界，模型、Schema、服务与迁移在对应业务切片落地时增加。
