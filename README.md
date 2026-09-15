# HotKey

个人热点监控学习项目。服务端已统一为 **Python / FastAPI / SQLAlchemy / RabbitMQ**，采用 PostgreSQL、Alembic、Celery；浏览器端使用 React、TypeScript 和 Vite。

目前可以初始化单用户账号、登录和退出、保存/编辑多来源关键词监控草稿、提交/查询/取消诊断任务。诊断经 PostgreSQL Outbox → RabbitMQ → Celery prefork → PostgreSQL 结果表，支持幂等、租约、重投与过期执行拒绝。

新增 Bluesky 查询预览和有界 CLI 来源探测，可读取公开线程并区分受限、空结果和部分结果。**持久化关键词采集、国内来源、事件归并、观点分析与报告尚未实现。** 草稿不会启动采集，诊断不会生成社交数据。当前实现进度见 [006 Plan](docs/plans/006-社交媒体关键词监控与评论分析计划.md)。

2026-09-15 新规划：**监控主题 → 发现收件箱 → 评论追踪 → 事件档案 → 分析与知识库**。已形成 [007 产品需求](docs/prd/007-热点事件与评论知识库.md)、[信息模型与技术设计](docs/design/007-热点事件与评论知识库设计.md)、[实施计划](docs/plans/007-热点事件与评论知识库计划.md)，均为待实施规划；复用当前 Python/React 与可靠任务基础，按真实来源与数据闭环逐片交付。

## 启动

需要 Docker Compose v2.24.4+。从仓库根目录执行：

```sh
docker compose up -d --build
docker compose exec backend python -m cli owner-init learner
```

第二条命令交互输入至少 12 位密码，无默认账号。打开 http://localhost:8010。首次运行迁移服务先建立新库，API/Worker 启动不修改表。开发端口仅绑定本机回环地址，可复制 `.env.example` 修改端口；改变网页端口时也要修改 Compose 中的允许来源。

## 工程与验证

- `backend/src/`：main 应用工厂、api 协议层、identity/monitors/jobs/sources 业务模块、core/db 公共设施、worker/ 队列执行、cli/ 管理命令及应用迁移。目录规范见 [backend README](backend/README.md)。
- `frontend/src/`：工作台及生成的 API 类型。
- `docs/openapi/openapi.json`：唯一发布契约。
- 根 Compose：唯一运行编排；生产使用覆盖文件。

```sh
uv sync --project backend --locked
uv run --project backend ruff check backend
uv run --directory backend mypy
uv run --project backend pytest backend/tests
uv run --directory backend/src python -m tools.export_openapi --check
npm ci --prefix frontend
npm run check:contract --prefix frontend
npm run build --prefix frontend
```

完整数据库/消息测试需要可丢弃的 `hotkey_test` 数据库和同名 RabbitMQ vhost，否则明确跳过；不能把跳过视为完整验证。设置步骤、浏览器测试和生产覆盖见 [运行手册](docs/operations/006-Python运行与验证.md)。

旧实现、旧契约和旧验收文档已从工作树删除，可通过 Git 历史查阅；新服务不兼容旧接口、不读取旧库、不自动迁移旧账号。已有持久卷不会被清理。使用与贡献请参阅 [规范](AGENTS.md)、[安全说明](SECURITY.md) 和 [LICENSE](LICENSE)。
