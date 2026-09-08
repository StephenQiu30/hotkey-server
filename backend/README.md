# HotKey Backend

后端直接作为 FastAPI 应用运行，代码统一放在 `backend/src/`，src 下不增加 hotkey 或 app 包装层。uv 管理依赖，不构建独立 wheel。SQLAlchemy 与 RabbitMQ 保持固定。

```text
backend/
├── src/
│   ├── main.py                 # FastAPI 工厂
│   ├── api/                    # HTTP 路由和依赖
│   ├── core/ / db/             # 配置、数据库资源
│   ├── identity/ / monitors/ / jobs/  # 业务模块及任务状态机
│   ├── worker/                 # app.py、messaging.py：Celery 与 RabbitMQ
│   ├── cli/                    # __main__.py、commands.py：管理命令
│   ├── audit/ / migrations/    # 审计、数据库迁移
│   └── tools/                  # 契约生成
├── scripts/                    # 容器验证
├── tests/                      # 单元、集成与架构检查
├── pyproject.toml / uv.lock
└── Dockerfile
```

从仓库根目录执行：

```sh
uv sync --project backend --locked
uv run --project backend ruff check backend
uv run --directory backend mypy
uv run --project backend pytest backend/tests -q
uv run --directory backend/src python -m tools.export_openapi --check
docker compose up -d --build
docker compose exec backend python -m cli owner-init learner
```

Python 运行工作目录为 backend/src，Docker 内为 /app/src。ASGI 入口 `main:create_app`，Worker 入口 `worker.app:app`，管理命令 `python -m cli`。迁移目录随应用复制进镜像；启动应用不自动改库。

业务分层与命名规则见 [AGENTS](../AGENTS.md)，实际服务与浏览器验证见 [Operations](../docs/operations/006-Python运行与验证.md)。未配置测试数据库/vhost 时集成测试会 skip，不能视为完整通过。真实平台采集仍未实现。
