# HotKey Server 交接

更新日期：2026-09-21。

## 当前结构

- `backend/`：Python 3.12、FastAPI、SQLAlchemy 2、PostgreSQL、Redis、Kafka；底座可运行，依赖由 uv 锁定，数据库 DDL 由单一 SQL 文件管理。
- `frontend/`：Next.js App Router、shadcn/ui、Radix UI、Tailwind CSS、Axios；工程可构建。
- `hotkey-app/`：独立 Flutter 客户端仓库。

前端页面位于 `src/app/`。页面专属组件放在所属路由的 `components/`，跨页面复用组件按功能领域放在 `src/components/<feature>/`，shadcn 组件放在 `src/components/ui/`。不使用 `features`、`common`、`patterns`、`shared` 或 `scripts` 目录。

## 前端基础

- `src/request.ts` 是唯一 Axios 请求封装，统一处理凭据、超时、响应数据和错误；业务 HTTP、网络、超时、取消及协议错误保持可判别。
- Umi OpenAPI 读取后端自动生成的 `/openapi.json`，生成文件直接写入 `src/api/`；命令环境变量 `HOTKEY_OPENAPI_URL` 可覆盖默认地址。
- `src/proxy.ts` 只处理页面 CSP nonce；`src/app/api/[[...path]]/route.ts` 负责同源 `/api/*` 转发及可控 502/504 错误契约。
- 页面采用组件优先的无边框设计，只使用 Tailwind 命名尺度及 `sm/md/lg/xl/2xl`。
- App Router 已配置 loading、error、global-error、not-found 和 `/health`。
- Dockerfile 定义 standalone、非 root 用户和健康检查；生产只读文件系统仍需根 Compose 落地并实际验证。

## 后端基础

- `app/main.py` 是唯一 FastAPI 应用工厂，lifespan 管理 SQLAlchemy Engine 与 Session 工厂。
- `app/api/` 统一管理路由、依赖、中间件、异常与文档；提供 `/api/health`、数据库 `/api/ready`、`/openapi.json`、`/docs` 和 `/scalar`。
- `app/core/` 管理 `HOTKEY_` 配置、结构化日志、公共错误和 Pydantic 基类；`app/db/` 管理唯一 DeclarativeBase、Engine、Session 和运行时模型元数据。
- `database/schema.sql` 是唯一数据库结构事实源，仅用于全新空库。项目没有 Alembic、revision 目录或应用启动建表逻辑。
- 业务领域目录不提前创建空包；`identity`、`monitors`、`jobs`、`sources`、`evidence`、`ai`、`audit` 的主责遵循 `PROJECT.md`，候选领域细分见 `docs/plans/001-热点事件监控平台总计划.md`，具体切片落地时再创建实际文件。
- `python -m worker` 是 Kafka Worker 入口。尚无业务消息处理器时安全退出，不订阅或提交任何消息；处理器只能在对应任务设计完成后注册。
- `python -m cli` 是 Typer 管理入口。`tests/unit`、`tests/integration`、`tests/architecture` 分别承载规则、HTTP 契约和依赖边界验证。
- 本机默认 PostgreSQL 库包含旧系统历史表，而当前 Python ORM 尚无业务模型。不得对该旧库执行 `database/schema.sql`；需要保留数据时先备份，再用新库完整建表并校验导入。

## 待完成

**[046 前置计划](docs/plans/046-全局异常与响应契约前置计划.md) 已完成并通过 Acceptance。** 未知异常请求标识、5xx/校验信息泄漏、OpenAPI 错误模型、Web 错误读取、同源代理失败及生成客户端漂移门禁均已修复和验证。后续接口继续复用该契约；下一执行点是 B00 总体/先行 Design 与来源条件登记，随后进入 B01/042 S01。

后端采用模块化单体与按业务领域分组的分层结构，完整目录、文件职责、API 契约、事务和依赖方向固定在根目录 [PROJECT.md](PROJECT.md)；执行入口、实现门禁和验证命令见 [AGENTS.md](AGENTS.md#fastapi-目录与命名必须执行)。

1. 建立根 Compose，并把现有 OpenAPI 客户端生成与差异检查扩展到真实依赖环境。
2. 明确旧 PostgreSQL 数据的保留、重建和校验导入策略，再同步实现首个业务领域 Model 与 `database/schema.sql` DDL。
3. 接入隔离的 PostgreSQL、Redis、Kafka 完成真实集成验证。
4. 按业务切片实现页面并完成桌面、窄屏和端到端验收。

## 检查

后端在 `backend/` 执行 `uv run ruff format --check .`、`uv run ruff check .`、`uv run mypy` 和 `uv run pytest`。前端执行 `pnpm test`、`pnpm lint`、`pnpm typecheck`、`pnpm format:check` 和 `pnpm build`；运行中的后端配合 `pnpm openapi:check` 校验生成漂移。产品进度以 `BACKLOG.md` 和对应 Acceptance 为准。

## 本轮产品文档复核

2026-09-21 先基于 HEAD `9093ed47` 静态复核工程，随后从 `37064d2a` 执行 046。BACKLOG 已补完整交付内容、跨计划批次、平台扩面与 App 队列；001/042 为 in_progress，只表示规划及底座工作已开始。046 技术前置 8/8 AC 已通过，但所有产品 AC 仍未通过，根 Compose、真实集成、业务流程和验收仍待完成。

本轮已执行应用测试、构建、OpenAPI 生成/漂移探针、真实同源代理和桌面/窄屏浏览器验证；未执行来源探测、数据库变更、真实消息集成或部署。证据与边界见 046 Acceptance，不继承过去运行结论作为当前实测。
