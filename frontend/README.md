# HotKey Web

技术栈：pnpm、Next.js App Router、React、TypeScript、shadcn/ui、Radix UI、Tailwind CSS、Axios、ESLint、Prettier。

## 运行

```bash
cp .env.example .env.local
pnpm install
pnpm dev
```

浏览器请求统一使用同源 `/api/*`，`src/app/api/[[...path]]/route.ts` 根据服务端 `HOTKEY_API_ORIGIN` 转发；`src/proxy.ts` 只负责 CSP nonce。

## 目录

```text
src/
├── app/                  # 路由；页面专属组件放对应路由的 components/
├── api/                  # Umi OpenAPI 生成文件
├── components/ui/        # shadcn 基础组件
├── components/<feature>/ # 跨页面复用组件
├── lib/                  # 纯工具
├── proxy.ts              # CSP nonce
└── request.ts            # Axios 请求封装
```

不创建 `features`、`common`、`patterns`、`shared` 或 `scripts` 目录。复用组件按功能领域分类；页面组件保留在所属路由中。组件归属、复用范围、目标路径、数据来源和状态覆盖必须在 Design 阶段确定。

## API

启动后端后，直接读取其自动生成的 `/openapi.json` 生成客户端：

```bash
pnpm openapi:generate
```

默认地址为 `http://127.0.0.1:8867/openapi.json`；其他环境使用 `HOTKEY_OPENAPI_URL=https://api.example.com/openapi.json pnpm openapi:generate`。该变量需传入命令环境，生成器不自动加载 Next.js 的 `.env.local`。

生成结果直接写入 `src/api/`，统一调用 `src/request.ts`。请求封装负责凭据、超时、响应数据提取以及错误标准化。

## 检查

```bash
pnpm lint
pnpm typecheck
pnpm format:check
pnpm build
```

生产镜像监听 `8080`，使用 standalone 输出、非 root 用户和只读文件系统。

仓库根 `compose.yaml` 是唯一完整运行入口；Web 默认绑定 `127.0.0.1:3000`，并在容器网络中代理至 API。端口可通过根 `.env` 调整。
