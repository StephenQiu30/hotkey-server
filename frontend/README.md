# HotKey Web frontend

HotKey 的唯一 Web 前端。工程由官方 `create-next-app` 与 shadcn CLI 初始化，固定使用 pnpm、Next.js App Router、React、TypeScript、shadcn/ui（Radix）、Tailwind CSS、Axios、ESLint 和 Prettier。

## 本地运行

要求使用 `package.json` 声明的 pnpm 版本。首次运行复制环境变量示例，只设置服务端可见的后端来源：

```bash
cp .env.example .env.local
pnpm install
pnpm dev
```

浏览器访问 `http://localhost:3000`。浏览器 API 请求固定使用同源 `/api/*`，`src/proxy.ts` 将其转发到 `HOTKEY_API_ORIGIN`；客户端代码不得读取或拼接后端来源。

## 固定目录

```text
frontend/
├── Dockerfile               # Node 24、standalone、非 root 生产镜像
├── src/
│   ├── app/                 # App Router 路由、布局、Metadata
│   ├── api/                 # @umijs/openapi 直接生成，禁止手改
│   ├── components/ui/       # shadcn CLI 管理的基础组件
│   ├── features/<feature>/  # 业务切片；组件、模型与状态就近组织
│   ├── lib/                 # 无业务语义的纯工具
│   ├── proxy.ts             # CSP nonce 与同源 API 代理
│   └── request.ts           # 唯一 Axios 传输封装
├── DESIGN.md                # DESIGN.md 到代码令牌的实现约束
├── components.json          # shadcn Radix 配置
├── next.config.ts           # standalone 等 Next.js 配置
└── openapi2ts.config.ts     # FastAPI OpenAPI 客户端生成配置
```

不建立 `shared/`、手写 API 端点或第二套 HTTP 客户端。`app` 只负责路由组合，业务代码进入 `features`；功能间不直接互相导入。前端不维护单独的 `scripts/` 目录，工程检查统一使用框架和工具链的标准命令。

设计采用组件优先的无边框体系：页面优先组合 `features`、`components/patterns` 和 `components/ui`，默认信息表面不使用装饰性边框。布局只使用 Tailwind 命名尺度以及 `sm`、`md`、`lg`、`xl`、`2xl` 响应式层级；不写原始像素值或任意布局尺寸。

## OpenAPI 客户端

FastAPI/Pydantic 是唯一 HTTP 契约源。后端生成并提交 `../docs/openapi/openapi.json` 后执行：

```bash
pnpm openapi:generate
```

生成结果直接进入 `src/api/` 并复用 `src/request.ts`，不再增加 `generated/` 中间层。当前仓库尚未建立 OpenAPI 快照，因此初始化阶段不伪造端点或 DTO。

## 质量门禁

```bash
pnpm lint
pnpm typecheck
pnpm format:check
pnpm build
```

生产镜像固定监听 `8080`，使用 standalone 输出并以 UID 1001 的非 root 用户运行：

```bash
docker build -t hotkey-frontend .
docker run --rm -p 8080:8080 -e HOTKEY_API_ORIGIN=http://host.docker.internal:8867 hotkey-frontend
```

视觉实现遵循本目录 [DESIGN.md](DESIGN.md)；产品能力仍以根目录 BACKLOG 与各切片 Acceptance 为准，基础页面不代表采集、分析或提醒能力已上线。
