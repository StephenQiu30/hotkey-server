# Next.js 前端初始化与容器迁移计划

## 目标

将唯一 Web 项目 `frontend/` 从 React + Vite 初始化为用户指定的 Next.js App Router 技术栈，为后续 HotKey 官网、SEO 页面和工作台设计提供可运行基础。Flutter `app/` 不在本计划范围内。

**状态：** 初始化与容器迁移已完成，详见 [009 Next 前端初始化与容器迁移验收](../acceptance/009-Next前端初始化与容器迁移验收.md)。HotKey 产品首页和工作台视觉改版、可索引 SEO 页面不属于本计划的已完成项。

## 范围与职责

| 文件/区域                                                                               | 变更      | 职责                                                                                                            |
| --------------------------------------------------------------------------------------- | --------- | --------------------------------------------------------------------------------------------------------------- |
| `frontend/package.json`、`package-lock.json`                                            | 修改      | Next.js 运行/构建、Tailwind、shadcn/Radix、ESLint 与格式检查命令                                                |
| `frontend/src/app/`、`frontend/src/proxy.ts`                                            | 新增/修改 | App Router 页面与布局、默认 metadata、严格 nonce CSP；现有工作台维持原路径和交互                                |
| `frontend/public/logo.svg`                                                              | 移动      | HotKey 唯一品牌母版；通过 Next metadata 使用                                                                    |
| `frontend/next.config.ts`、`postcss.config.mjs`、`eslint.config.mjs`、`components.json` | 新增      | 同源 API rewrite、standalone 输出、安全响应头、Tailwind/shadcn 与 lint 配置                                     |
| `frontend/Dockerfile`、`docker-compose.yml`、`frontend/nginx.conf`                      | 修改/删除 | Next 开发服务器与最小 standalone 生产镜像，保留容器端口 `8080` 和 API 路径；移除已不再使用的 Nginx 静态站点配置 |
| `.github/workflows/ci.yml`                                                              | 修改      | 替换失效的旧脚本调用为有效生成契约、format、lint、typecheck 与生产构建门禁                                      |
| `AGENTS.md`、`docs/design/009-*`、本计划与 Acceptance                                   | 修改/新增 | 记录落地状态、路径约定、验证结果和未完成范围                                                                    |

## 实施约束

- 保留 `src/app/App.tsx` 及生成 API 客户端、Axios 请求封装和现有 Playwright 测试；本计划不重做工作台 UI，不选用此前未获确认的视觉候选稿。
- 将现有工作台根路径保持在 `/`，继续服务现有 Playwright 和用户入口。页面暂标记为不索引；公开产品首页和可公开事件详情须独立完成设计与权限验证后再加入 SEO 与 sitemap。
- Next rewrites 保留 `/api/:path*` 同源转发；Docker build 阶段将 `HOTKEY_API_ORIGIN` 写入 Next 路由清单，Compose 默认使用 `http://backend:8080`，本机开发默认 `http://127.0.0.1:8867`。若在 Vercel 部署，Vercel 项目根目录选 `frontend/`，并在构建环境设置可访问的后端 origin；此 origin 必须是服务端可达地址。
- 生产输出采用 Node standalone，监听 `8080`、以非 root 用户运行；不开放容器根文件系统写权限。
- Content Security Policy 使用逐请求 nonce，保留 `script-src` 与 `style-src` 的严格策略。开发模式仅按 Next.js 文档启用 `unsafe-eval`。
- 不运行全新项目生成器覆盖现有目录；不提交、不推送、不创建部署。

## 验证门禁

1. 安装锁定依赖，执行 Prettier、ESLint、TypeScript、Next.js production build。
2. 执行 `docker compose config --quiet` 及测试/生产 overlay 的 Compose 配置检查。
3. 以浏览器验证首页服务端 HTML、唯一标题/description/robots、logo、nonce CSP、无 hydration 错误，并检查 `/api/*` 的同源代理。
4. 后端容器替换后保持 Web 容器 ID 不变，经 Web `/api/v1/session` 仍返回 `401`；在生产只读容器约束下验证 standalone 启动。
5. 完成浏览器验证后记录实际执行和未执行项；未跑门禁不得写成通过。

| 门禁                 | 状态     | 验收范围                                                                                                                              |
| -------------------- | -------- | ------------------------------------------------------------------------------------------------------------------------------------- |
| 前端工具链与契约     | 通过     | Prettier、ESLint（3 条已记录告警）、TypeScript、Next build、客户端生成检查                                                            |
| Compose 配置         | 通过     | base、测试与生产 overlay 的配置解析                                                                                                   |
| 页面与代理浏览器检查 | 部分通过 | 已检查生产页面 DOM、元数据、Logo、nonce CSP、控制台及 API 代理；原始 HTTP HTML 源码和响应头未独立归档，完整业务 Playwright/E2E 未运行 |
| 生产容器与服务替换   | 通过     | 隔离 standalone 容器、只读 rootfs、非 root 用户、登录 API 401、backend replacement 后 Web 未替换                                      |
| 公开页面 SEO         | 未完成   | 当前根路径仍是工作台入口并设为 noindex；公开首页、canonical/OG、sitemap 不在本次实现范围                                              |
| 架构边界门禁         | 未完成   | 旧 Vite 边界脚本已删除，尚无针对 Next 目录结构的替代检查；见 Acceptance 的限制说明                                                    |

## Vercel 项目检查

- 按 Vercel 官方文档核对 Next.js 部署约定：Vercel 项目根目录为 `frontend/`，框架使用 Next.js 自动检测，默认构建命令为 `npm run build`，构建输出使用 Next.js 默认 `.next`。同源 API rewrite 依赖服务端可访问的 `HOTKEY_API_ORIGIN`，部署前需在项目环境中提供该值。
- 使用 Vercel 插件检查团队和项目：StephenQiu 团队当前没有已存在的项目；仓库也没有 `.vercel/project.json`。插件可部署已有项目但未提供仅创建/链接空项目的操作。本次不发布部署，Vercel 云端项目关联待已有项目或项目创建入口可用后完成。
- 本地 Next.js 初始化、配置检查和隔离生产容器验证已完成；具体命令及结果记录在 Acceptance。
