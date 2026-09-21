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

## 首个使用者与恢复

首次初始化前，在未跟踪的根 `.env` 中设置唯一且不少于 32 字符的 `HOTKEY_BOOTSTRAP_TOKEN`。受控客户端调用 `POST /api/identity/initialize` 时同时发送该值、`X-HotKey-CSRF: 1` 以及 owner 用户名和不少于 12 字符的密码。初始化完成后从运行环境移除 bootstrap 值并重新创建 API 容器；系统没有默认用户或默认密码。

登录、当前会话和注销分别使用 `POST /api/identity/sessions`、`GET /api/identity/session`、`DELETE /api/identity/session`。浏览器只使用服务端设置的会话 Cookie；非安全方法由 Web 请求层附加 CSRF 请求头，不把 Cookie 值传给业务函数。

密码遗失时由具备部署维护权限的操作者执行交互式恢复；新密码不会放入命令参数，恢复成功会撤销全部旧会话：

```bash
docker compose run --rm cli identity reset-password
```

当前只支持全新空库初始化，不得为给旧库补身份表而直接执行 `schema.sql`。

## 状态

当前底座包含应用工厂、数据库会话、唯一 `schema.sql`、结构化日志、健康检查、Swagger UI、Scalar、Kafka Worker、管理 CLI、身份会话领域、内部任务受理与恢复服务及架构测试。Worker 已具备 outbox 发布、手动 offset、inbox、租约和检查点装配；没有已登记业务 kind 处理器时安全退出。其余业务领域只在对应切片完成 Design 登记后创建，不保留空的未来领域包。
