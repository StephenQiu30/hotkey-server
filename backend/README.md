# HotKey Backend

Python 3.12、FastAPI、Uvicorn、Pydantic 2、SQLAlchemy 2、psycopg 3、Alembic、PostgreSQL、Redis、Kafka、MinIO。

采用按业务领域分组的模块化单体。架构、目录、事务、依赖方向与验收规则统一执行根目录 [AGENTS.md](../AGENTS.md#fastapi-目录与命名必须执行)。

## 执行入口

后端初始化完成后，从 `backend/src/` 执行：

```bash
uvicorn main:create_app --factory
python -m worker
python -m cli
```

API、Worker 和 CLI 分别启动。数据库迁移作为独立步骤执行。

## 状态

后端应用、依赖锁文件和运行配置尚未建立。
