# HotKey Backend

本目录为唯一 Python 后端，技术基线见 [PROJECT.md](../PROJECT.md)，工程规则见 [AGENTS.md](../AGENTS.md)。

固定使用 Python 3.12、FastAPI、Pydantic、SQLAlchemy 2、Alembic、PostgreSQL、Redis、Kafka，复用既有 MinIO。业务代码计划位于 `src/`，应用工厂入口 `main:create_app`；Kafka Worker 拟定入口 `python -m worker`。

当前仅建立目录与职责说明，尚无应用、依赖、锁文件或运行命令。后续按 001 总体设计与 042 底座计划初始化；不恢复 RabbitMQ/Celery，不提前创建空业务模块。
