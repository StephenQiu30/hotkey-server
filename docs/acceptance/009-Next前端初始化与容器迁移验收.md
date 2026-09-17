# Next 前端初始化与容器迁移验收

**验收日期：** 2026-09-17

**范围：** `frontend/` 从 React + Vite 初始化为 Next.js App Router，迁移容器运行方式，并固定 HotKey Logo 与设计规范。
**结果：** Next.js 初始化与隔离生产容器核心门禁通过；部分浏览器门禁未完全核验，架构边界检查、公开产品首页、工作台界面改版和可索引 SEO 页面仍未实现。

## 验收结果

| 项目                 | 结果             | 证据                                                                                                                           |
| -------------------- | ---------------- | ------------------------------------------------------------------------------------------------------------------------------ |
| OpenAPI 客户端契约   | 通过             | `npm run check:contract` 重新生成成功，工作区无生成差异                                                                        |
| Prettier             | 通过             | `npm run format:check`                                                                                                         |
| TypeScript           | 通过             | `npm run typecheck`                                                                                                            |
| ESLint               | 通过，有既存告警 | `npm run lint` 退出成功；3 条告警涉及匿名默认导出、`App.tsx` ref cleanup 和 `request.ts` 未使用的 `requestType`                |
| Next.js 生产构建     | 通过             | `npm run build`，Next.js 16.3.5 App Router 构建成功                                                                            |
| shadcn/ui 配置识别   | 通过             | `shadcn info` 识别 Next.js App Router、TypeScript、Tailwind v4、Radix、Lucide 与路径别名                                       |
| Compose 配置         | 通过             | 开发配置、`.env.env.example` 测试叠加配置、`.env.prod.example` 生产叠加配置均通过 `docker compose config --quiet`              |
| Logo 资源            | 通过             | 唯一母版为 `frontend/public/logo.svg`；SVG XML 解析有效，Next Metadata API 引用 `/logo.svg`                                    |
| 生产容器约束         | 通过             | 隔离 production Compose 构建成功；Next 容器使用 `node` / UID 1000，root filesystem 为只读，服务健康                            |
| 页面与同源 API       | 通过             | 隔离生产站点 `/`、`/logo.svg`、`/robots.txt` 均为 200；`/api/v1/session` 经 Next rewrite 到后端，未认证时返回预期 401          |
| 后端替换后代理       | 通过             | `verify_proxy_replacement.py` 返回 `web_replaced=false`、`backend_replaced=true`、`proxied_status=401`                         |
| 浏览器生产检查       | 通过             | 替换后检查页面标题、描述、`noindex`、logo、登录表单；脚本带 nonce；无 Next 错误遮罩且 `window.__consoleErrors` 为空            |
| 依赖漏洞（生产依赖） | 通过             | `npm audit --omit=dev --audit-level=high` 未发现生产依赖高危漏洞                                                               |
| 全量依赖审计         | 有已知限制       | 全量 `npm audit` 报告 2 个 dev-only 高危项，链路来自 `@umijs/openapi` 引入的 `mockjs <=1.1.0` 原型污染问题；当前无可用修复版本 |
| 仓库差异格式         | 通过             | `git diff --check`                                                                                                             |

隔离生产验证使用独立 Compose 项目 `hotkey-next-init`、临时端口和新建数据卷；验收完成后已停止并删除该项目及其卷。已有的 `hotkey-local-main` 容器保持未修改。

## 验收限制与待办

- 计划要求检查首页原始服务端 HTML 和响应头；本次验证了浏览器 DOM 中的 title、description、robots 和 nonce，但没有单独保存或断言原始 HTTP HTML/headers，因此该子项未完全验收。
- 本次没有运行完整 Playwright E2E、完整后端测试/CI、响应式截图检查或远端 GitHub Actions。浏览器证据仅覆盖隔离生产环境中的页面加载、登录表单、metadata、logo、nonce 与控制台状态。
- 原 Vite 的 `check:boundaries` 与 `test:boundaries` 脚本当前已删除，CI 改为执行 Next lint、typecheck 和 build，但没有补充针对 Next.js `src/app`、`src/lib`、`src/api` 的架构边界检查。不要把 lint/typecheck 当成架构边界检查的等价替代。
- 新 `check:contract` 命令执行生成器后检查 `git diff --exit-code -- src/api`；它能发现已跟踪文件生成差异，但不负责独立比较预期与实际文件清单，未跟踪的新生成文件也不保证被该检查发现。
- 只验证了 Docker rootfs 只读、进程非 root 和容器健康；未单独留存所有运行时 mount（例如 cache 临时挂载）的 inspect 证据。

## Vercel 检查

- 通过 Vercel 插件文档搜索核对 Next.js 部署设置与项目框架约定。部署目标目录为 `frontend/`，构建命令为 `npm run build`，使用 Next.js 默认 `.next` 输出；同源后端转发需要配置服务端可访问的 `HOTKEY_API_ORIGIN`。
- 已查看当前账号下 StephenQiu 团队的项目清单，项目数为 0；仓库未链接 `.vercel/project.json`。现有 Vercel 工具提供部署与已有项目管理，不提供只创建/关联空项目的操作，因此本次完成的是本地项目初始化和 Vercel 规范检查，没有创建云端项目或发布部署。

## 未包含在本次验收内

- 本次不代表 HotKey 营销首页或工作台 UI 已完成设计改版；已有工作台业务仍由现有组件承载。
- 根页面当前是登录/工作台入口并保持 `noindex`。公开产品首页、真实且获准公开的事件详情、canonical/Open Graph、`sitemap.xml` 和面向搜索引擎的内容尚未实现。该入口不得加入 sitemap。
- 本次未运行完整后端单元/集成测试、全量 CI 或 Playwright 套件；浏览器检查只证明所列本地生产页面和交互状态。
- 隔离服务验证之前，已有本地 `hotkey-local-main` 后端的 OpenAPI 路径仍与当前 checkout 不一致（运行实例暴露 `/api/session`，当前契约是 `/api/v1/session`）。最终代理验收改用隔离项目中的当前代码并通过；不要把旧实例结果当作当前 checkout 的生产验收。

## 仓库同步与交付边界

- 初始同步前，`main` 相对 `origin/main` 落后 46 个提交；原工作区改动已保存至 `stash@{0}`，随后本地分支快进至 `25dc52ef` 并与 `origin/main` 对齐。恢复工作区时仅两份 README 发生冲突，已保留远端较新的业务内容并合并 Next 与 Compose 说明；stash 备份保留，其他并行改动仍在工作区。
- 原有未跟踪的 `frontend/vite.config.ts` 保留且未修改；当前 Next lint、TypeScript、Docker build context 均忽略它，它不再作为前端运行入口。
