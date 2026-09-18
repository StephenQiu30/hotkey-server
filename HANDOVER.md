# HotKey Server 交接

更新日期：2026-09-18。

## 当前结构

- `backend/`：Python、FastAPI、SQLAlchemy 2、PostgreSQL、Redis、Kafka；应用代码尚未建立。
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

## 待完成

后端采用模块化单体与按业务领域分组的分层结构，完整目录、文件职责、事务、依赖方向、执行入口和设计交付要求固定在根目录 [AGENTS.md](AGENTS.md#fastapi-目录与命名必须执行)。

1. 建立后端应用、迁移、根 Compose 和注解驱动的 OpenAPI 端点，并在 CI 接入客户端自动生成与差异检查。
   使用 uv 锁定依赖，按 AGENTS 中的通用工具清单接入配置、HTTP、日志、重试、CLI 和基础设施客户端，注册 `/docs`、`/scalar` 与 `/openapi.json`。
2. 生成前端 API 客户端并验证真实请求与错误契约。
3. 接入 PostgreSQL、Redis、Kafka 后完成隔离集成验证。
4. 按业务切片实现页面并完成桌面、窄屏和端到端验收。

## 检查

前端执行 `pnpm lint`、`pnpm typecheck`、`pnpm format:check` 和 `pnpm build`。产品进度以 `BACKLOG.md` 和对应 Acceptance 为准。
