# HotKey Server 交接

更新日期：2026-09-18。

## 当前结构

- `backend/`：Python 3.12、FastAPI、SQLAlchemy 2、PostgreSQL、Redis、Kafka；底座可运行，依赖由 uv 锁定，数据库 DDL 由单一 SQL 文件管理。
- `frontend/`：Next.js App Router、shadcn/ui、Radix UI、Tailwind CSS、Axios；工程可构建。
- `hotkey-app/`：独立 Flutter 客户端仓库。

前端页面位于 `src/app/`。页面专属组件放在所属路由的 `components/`，跨页面复用组件按功能领域放在 `src/components/<feature>/`，shadcn 组件放在 `src/components/ui/`。不使用 `features`、`common`、`patterns`、`shared` 或 `scripts` 目录。

## 前端基础

- `src/request.ts` 是唯一 Axios 请求封装，统一处理凭据、超时、响应数据和错误。
- Umi OpenAPI 读取后端自动生成的 `/openapi.json`，生成文件直接写入 `src/api/`；命令环境变量 `HOTKEY_OPENAPI_URL` 可覆盖默认地址。
- `src/proxy.ts` 处理 CSP nonce 和同源 `/api/*` 转发。
- 页面采用组件优先的无边框设计，只使用 Tailwind 命名尺度及 `sm/md/lg/xl/2xl`。
- App Router 已配置 loading、error、global-error、not-found 和 `/health`。
- Docker 镜像使用 standalone、非 root 用户、只读文件系统和健康检查。

## 后端基础

- `src/main.py` 是唯一 FastAPI 应用工厂，lifespan 管理 SQLAlchemy Engine 与 Session 工厂。
- `src/api/` 统一管理路由、依赖、中间件、异常与文档；提供 `/health`、数据库 `/ready`、`/openapi.json`、`/docs` 和 `/scalar`。
- `src/core/` 管理 `HOTKEY_` 配置、结构化日志、公共错误和 Pydantic 基类；`src/db/` 管理唯一 DeclarativeBase、Engine、Session 和运行时模型元数据。
- `database/schema.sql` 是唯一数据库结构事实源，仅用于全新空库。项目没有 Alembic、revision 目录或应用启动建表逻辑。
- `identity`、`monitors`、`jobs`、`sources`、`evidence`、`ai`、`audit` 已建立领域包边界。具体模型、Schema、服务和适配器随业务切片添加。
- `python -m worker` 是 Kafka Worker 入口。尚无业务消息处理器时安全退出，不订阅或提交任何消息；处理器只能在对应任务设计完成后注册。
- `python -m cli` 是 Typer 管理入口。`tests/unit`、`tests/integration`、`tests/architecture` 分别承载规则、HTTP 契约和依赖边界验证。
- 本机默认 PostgreSQL 库包含旧系统历史表，而当前 Python ORM 尚无业务模型。不得对该旧库执行 `database/schema.sql`；需要保留数据时先备份，再用新库完整建表并校验导入。

## 待完成

后端采用模块化单体与按业务领域分组的分层结构，完整目录、文件职责、事务、依赖方向、执行入口和设计交付要求固定在根目录 [AGENTS.md](AGENTS.md#fastapi-目录与命名必须执行)。

1. 建立根 Compose，并在 CI 接入 OpenAPI 客户端自动生成与差异检查。
2. 明确旧 PostgreSQL 数据的保留、重建和校验导入策略，再同步实现首个业务领域 Model 与 `database/schema.sql` DDL。
3. 生成前端 API 客户端并验证真实请求与错误契约。
4. 接入隔离的 PostgreSQL、Redis、Kafka 完成真实集成验证。
5. 按业务切片实现页面并完成桌面、窄屏和端到端验收。

## 检查

后端在 `backend/` 执行 `uv run ruff format --check .`、`uv run ruff check .`、`uv run mypy` 和 `uv run pytest`。前端执行 `pnpm lint`、`pnpm typecheck`、`pnpm format:check` 和 `pnpm build`。产品进度以 `BACKLOG.md` 和对应 Acceptance 为准。
