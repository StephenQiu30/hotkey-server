# HotKey Frontend

本目录是 HotKey 唯一 Next.js Web 前端，技术基线见 [PROJECT.md](../PROJECT.md)，工程规则见 [AGENTS.md](../AGENTS.md)。

固定使用 pnpm、Next.js App Router、React、TypeScript、shadcn/ui、Radix UI、Tailwind CSS、Axios、ESLint、Prettier。采用 Radix 版 shadcn/ui 组件；客户端通过 `@umijs/openapi` 生成。

计划目录：页面 `src/app/`，业务功能 `src/features/`，基础组件 `src/components/ui/`，生成客户端 `src/api/`，Axios 封装 `src/request.ts`。初始化时提交 `pnpm-lock.yaml` 并固定 `packageManager`。

当前仅建立目录与职责说明，尚未生成 Next.js 应用、依赖或运行脚本。独立 Flutter 客户端在同级仓库 `hotkey-app`，不在此处实现。
