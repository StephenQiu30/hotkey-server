# HotKey Server 交接

更新日期：2026-09-18。交接范围：仓库结构、技术选型与 Web 前端初始化。

## 1. 阅读入口

[PROJECT](PROJECT.md) → [AGENTS](AGENTS.md) → [BACKLOG](BACKLOG.md) → [文档台账](docs/README.md) → 当前切片的 Design/PRD/Plan。PROJECT 是本仓库技术选型主文档，BACKLOG 是产品进度的唯一台账。

## 2. 本次完成

- 将误放于 `.codex/PROJECT.md` 的技术文档移至仓库根 `PROJECT.md`，并重写根 `HANDOVER.md`，修正相对引用。
- 建立 `backend/README.md`、`frontend/README.md`，让两个工程目录可随 Git 保存。
- 固定 Web 为 pnpm + Next.js + shadcn/ui + Radix UI + Tailwind CSS + Axios + ESLint + Prettier，补充 React/TypeScript 和已有 OpenAPI 生成链。
- 固定后端为 Python + SQLAlchemy 2 + FastAPI + PostgreSQL + Redis + Kafka；保留 Alembic、Pydantic、MinIO 和质量工具。
- 同步 AGENTS、贡献规范、问题模板、BACKLOG 和受影响的 Design/Plan，清除旧消息栈的现行要求；保留所有需求编号、验收目标及未完成状态。
- Web 归本仓库 `frontend/`；同级 `hotkey-web` 本地重命名为 `hotkey-app`，固定 Flutter，前端归属冲突已关闭。
- 使用官方 `create-next-app` 建立 Next.js 16.3.5 / React 19.2.8 / pnpm 12.3.4 工程，并用官方 shadcn CLI 的 Radix Nova 预设建立基础组件。
- 按根 `DESIGN.md` 映射 Geist、颜色、间距与 Hero 渐变；固定 HotKey 无边框规则，默认 Card 通过表面和留白分层，输入、焦点、错误态与浮层保留必要轮廓。
- 建立 `src/app`、`src/components/<feature>`、`src/components/ui`、`src/api`、`src/request.ts` 的目录边界；不建立 `src/features` 或含糊的公共层，Umi OpenAPI 生成物直接进入 `src/api/`。
- 建立 Axios 传输层、`@umijs/openapi` 配置、Next.js 16 `proxy.ts` 同源 API 转发与逐请求 CSP nonce，以及 Node.js 24.19.0 standalone 非 root 生产镜像。
- 建立可响应的基础页面、Metadata/manifest 和唯一图标母版；页面明确区分工程基础与尚未交付的产品能力。
- 将设计固定为组件优先的无边框系统，把基础页拆为独立 Header、Hero、能力概览、设计原则与状态组件；统一使用 `sm/md/lg/xl/2xl` 和 Tailwind 命名尺度。
- 首页专属组件归入 `src/app/components/`，跨页面状态组件按 system 功能领域归入 `src/components/system/`；后续可复用组件按明确 feature 进入 `src/components/<feature>/`，并在切片 Design 阶段先记录归属、复用范围、目标路径、数据来源和状态覆盖。
- 前端不建立独立 `scripts/` 目录；设计和目录约束由规范、组件封装及 ESLint、TypeScript、Prettier、生产构建共同维护。shadcn 基础组件中的固定布局值已归一为命名尺度。
- 增加 loading、error、global-error、not-found、PageState 与 `/health`，降低路由失败产生空白页的风险；Docker 镜像增加进程健康检查。

## 3. 当前状态与边界

当前分支为 `main`，前置技术规范基线已提交并推送到 `39deb15e`，Web 初始化已提交并推送到 `c54358ad`。仓库当前没有 `.github/workflows`，因此该提交没有对应的 Actions 运行记录。

43 项逐项 PRD 与 45 份 Plan 仍为规划基线；总体设计及 Redis/Kafka 详细执行设计待补齐，产品验收未推进。`frontend/` 工程基础已经初始化，`backend/` 仍只有目录说明；根 Compose、后端应用、迁移、真实 OpenAPI 快照与集成依赖仍不存在。本次没有连接现有数据、实采或声称任何业务功能可用。

Kafka 的选型已经确定，但不会自动提供定时调度、任务取消、数据库幂等和重试策略；这些仍由 031/042 实施。Redis 与 Kafka 均需隔离测试，不能用旧 RabbitMQ/Celery 证据替代。

## 4. 下一步

1. 按 001 S01 完成总体设计，登记 server Web 与 Flutter App 的契约边界；不重复询问已确定的技术栈。
2. 在 031/042 中冻结 Kafka topic/分区键/消费组/提交位点、重试/死信/定时任务、Redis 失效策略、客户端及依赖版本。
3. 按 042 S01 初始化 backend、迁移、根 Compose 和真实 OpenAPI 快照；随后运行已配置的 Web 客户端生成链，再推进受控任务与真实业务切片。
4. Flutter 初始化由 `hotkey-app` 承接；目标平台、Dart 生成器与鉴权传输需在对应切片落实。

## 5. 本轮检查

已通过：`pnpm install`、ESLint、Prettier、TypeScript 和 `next build`；standalone HTTP 返回 200，CSS 资源返回 200，CSP nonce 每个请求更新，同源 `/api/*` 通过临时本地服务验证保留路径与查询参数；Chrome 实际检查了 Hero 和无边框卡片区域。生产 Docker 镜像已构建，并以只读文件系统、UID/GID 1001 非 root 用户、8080 端口和健康检查运行。

未验证：真实后端/OpenAPI 生成、数据库/Redis/Kafka、根 Compose、远端部署、窄屏设备与任何产品来源。浏览器检查只覆盖当前桌面视口；响应式类已进入构建，但不能替代后续窄屏验收。

后续交接更新日期、提交、改动、实际命令与结果、阻塞和下一动作；技术变更同步 PROJECT，产品进度回写 BACKLOG 与对应 Plan。不要将本轮目录/文档完成记为产品功能完成。
