# HotKey Server 项目与技术选型

更新日期：2026-09-18。本文是 `hotkey-server` 的技术选型入口，按用户本次决定固定。工程规则见 [AGENTS.md](AGENTS.md)，交接见 [HANDOVER.md](HANDOVER.md)，产品进度见 [BACKLOG.md](BACKLOG.md)。

## 1. 定位与仓库边界

HotKey 面向热点事件监控、作品与评论采集、事件时间线、讨论证据分析、提醒和报告。本仓库同时维护 Python 后端与 Web 前端；独立客户端仓库为同级 `hotkey-app`，固定 Flutter + Dart。两端消费同一 FastAPI OpenAPI 契约。

```text
HotKey/
├── hotkey-server/
│   ├── PROJECT.md
│   ├── HANDOVER.md
│   ├── AGENTS.md
│   ├── BACKLOG.md
│   ├── backend/          # Python API、后台执行与数据访问
│   ├── frontend/         # Next.js Web 工作台
│   └── docs/             # 调研、需求、设计、计划与后续验收
└── hotkey-app/            # 独立 Flutter 客户端仓库
    ├── PROJECT.md
    ├── HANDOVER.md
    └── AGENTS.md
```

`frontend/` 已用官方脚手架建立可构建的 Web 工程、依赖锁、设计令牌、同源 API 代理、OpenAPI 生成配置和架构门禁；当前基础页只证明工程与视觉基线，不代表产品功能已实现。`backend/` 仍只有目录说明，后端源码、迁移、Compose 与运行依赖尚未初始化。

## 2. 固定技术栈

### Web 前端

**pnpm + Next.js + shadcn/ui + Radix UI + Tailwind CSS + Axios + ESLint + Prettier。**

| 技术 | 职责 |
|---|---|
| pnpm | 唯一 Web 包管理器；提交 `pnpm-lock.yaml`，在 `package.json` 中固定 `packageManager` |
| Next.js App Router + React + TypeScript | 页面、布局、交互、服务端代理与类型约束 |
| shadcn/ui + Radix UI | shadcn/ui 采用 Radix primitives 的组件方案，不切换其他底层组件实现 |
| Tailwind CSS + CSS Variables | 样式与统一设计令牌 |
| Axios | 集中处理请求、认证与错误；封装固定在 `frontend/src/request.ts` |
| `@umijs/openapi` | 从后端 OpenAPI 生成类型与端点函数至 `frontend/src/api/` |
| ESLint + Prettier | 代码检查与格式化 |

页面归 `frontend/src/app/`，业务功能归 `frontend/src/features/`，基础 UI 归 `frontend/src/components/ui/`。浏览器调用同源 `/api/*`；`HOTKEY_API_ORIGIN` 仅供 Next.js 服务端代理使用。具体认证与错误契约由后端统一维护。

Web 设计固定为组件优先的无边框系统：App Router 页面只组合 feature、pattern 与 shadcn/Radix 基础组件；默认信息表面通过留白、排版和语义背景分层。布局只使用 Tailwind 命名尺度和 `sm/md/lg/xl/2xl` 标准响应式层级，不使用原始像素值或任意布局尺寸。输入、焦点、错误与浮层保留必要轮廓；加载、路由错误、全局错误、404 与进程健康状态都有统一边界。

### Python 后端与基础设施

**Python + SQLAlchemy 2 ORM + FastAPI + PostgreSQL（PGSQL）+ Redis + Kafka。**

| 技术 | 职责 |
|---|---|
| Python 3.12 | 沿用已定语言基线；业务代码位于 `backend/src/` |
| FastAPI + Pydantic | API、验证、错误契约及唯一 OpenAPI 源 |
| SQLAlchemy 2 + Alembic | ORM 与数据库迁移；保持单一模型和迁移体系 |
| PostgreSQL | 业务事实、权限、任务、进度、幂等记录与 Outbox 的持久存储 |
| Redis | 缓存、限流和可重建临时状态；关键权限、预算与任务状态仍有数据库依据 |
| Kafka | 任务事件与异步消息传输，由 Python Worker 消费 |
| MinIO | 复用既有对象存储，保存有权限与保留期约束的文件及证据 |
| Docker Compose | 根目录唯一运行编排；开发/生产差异通过配置叠加 |
| Ruff + mypy + pytest | 格式/静态检查、类型、单元/集成/架构验证 |

必要配套包含 ASGI 服务、PostgreSQL/Redis/Kafka 的 Python 客户端及配置管理；具体客户端库和兼容版本在 042 底座切片锁定。Web 当前锁定 Node.js 24.19.0、Next.js 16.3.5、React 19.2.8 与 pnpm 12.3.4；Python、Flutter/Dart 与其他服务镜像版本仍需在对应初始化切片记录，不把“最新”写作可复现版本。

## 3. 数据与任务执行边界

1. API 路由负责 HTTP、认证与输入输出，经依赖注入调用领域服务；服务管理事务，SQLAlchemy 模型负责持久化。
2. 业务变更与 Outbox 写入同一 PostgreSQL 事务。独立发布器可靠地发送到 Kafka；消费者允许重复读取，以消息 ID、数据库唯一约束和业务状态保证幂等。
3. 消费者在业务事务提交后提交连续完成位置的 offset；处理并发时不得越过尚未完成的记录。Kafka 事务不能直接保证 PostgreSQL 副作用的原子性。[Kafka 消息交付语义](https://kafka.apache.org/41/design/design/)
4. Redis 的数据丢失不能导致任务或证据丢失。缓存设有效期与失效规则；限流故障时采用明确的保守策略。执行权、不可超额预算与撤权不能只依赖 Redis 锁或缓存。
5. `worker/` 维护 Kafka 客户端和消费者生命周期，`jobs/` 维护任务状态机；拟定入口 `python -m worker`。在 031/042 设计中明确 topic、partition key、consumer group、重试、死信、延迟/周期调度和再均衡处理，不能把 Kafka 当作已有任务调度器。
6. API、Worker 各自创建数据库连接池和消息客户端，Session 不跨线程/任务共享。同步数据库调用不直接放入异步路由。
7. 唯一 HTTP 契约为 FastAPI/Pydantic；计划快照 `docs/openapi/openapi.json`，Web 和 Flutter 均由该契约生成客户端。Web 已配置 `@umijs/openapi` 生成链，但快照尚不存在，因此没有伪造端点或 DTO；Flutter 生成器在 App 初始化时确定。

**RabbitMQ 与 Celery 已退出当前技术基线。** 原来的 Celery 入口、broker 配置与任务结果后端不再沿用；Redis 不额外承担第二套任务消息队列。当前没有运行中的旧实现，本次不涉及生产消息或数据库迁移。

## 4. 产品约束与未定事项

- 复用现有 MinIO；内容来源采用免费或自建方案，只采集公开或获授权数据。X 为首版必需来源，国内评论范围面向 B 站、微博、小红书、抖音，首批组合按总体设计冻结。
- twscrape 仍为 [002](docs/design/002-X免费采集与热点监控设计.md) 的验证候选；模型供应商、SDK、模型与设备仍按 [043](docs/design/043-模型服务接入与模型配置设计.md) 等专项处理，不能写成已接入。
- 内容预算不构成付费 AI 授权；现有模型方案继续按本地优先、外部默认关闭、零付费约束推进。
- Flutter 的目标平台、状态管理、路由及 Dart 客户端生成器尚未冻结；它们不影响 Flutter 技术方向或 Web 目录归属。

## 5. 实施与验证

按总体 Design → 需求/Plan → 失败验证 → 实现 → 回归/Acceptance 推进。先依据 [001 总计划](docs/plans/001-热点事件监控平台总计划.md) 完成总体设计，再通过 [042](docs/plans/042-容量与部署可重复性计划.md) 建立底座；[031](docs/plans/031-可靠执行与幂等计划.md) 承接 Kafka 消费和恢复语义。

初始化后必须执行后端 Ruff/mypy/pytest、OpenAPI 漂移与客户端生成检查、前端 ESLint/Prettier/类型/构建和浏览器验证。Web 已提供并通过 ESLint、Prettier、TypeScript、目录边界及生产构建入口；后端与完整 OpenAPI 漂移门禁仍待建立。集成测试使用隔离的真实 PostgreSQL、Redis、Kafka；验证重复事件、提交后中断、消费者再均衡、Redis 失效和任务恢复。

## 6. 决策与维护

| 日期 | 决策 | 影响 |
|---|---|---|
| 2026-09-18 | 用户固定 pnpm/Next.js/shadcn/ui/Radix UI/Tailwind CSS/Axios/ESLint/Prettier | Web 唯一归属 `frontend/` |
| 2026-09-18 | 用户固定 Python/ORM/FastAPI/PGSQL/Redis/Kafka；ORM 沿用 SQLAlchemy 2 | 同步工程规范、当前 Design/Plan、真实依赖验证要求；停用旧 RabbitMQ/Celery 规划 |
| 2026-09-18 | `hotkey-web` 本地重命名为 `hotkey-app`，固定 Flutter | 关闭前端归属冲突，两个仓库分别维护根 PROJECT/HANDOVER |
| 2026-09-18 | 用官方 Next.js 与 shadcn Radix 脚手架初始化 `frontend/` | 固定 Web 版本、目录边界、无边框令牌、CSP nonce、同源代理和 OpenAPI 生成入口；产品功能仍未开始验收 |
| 2026-09-18 | 固定组件优先的无边框设计与命名响应式尺度 | 使用 `sm/md/lg/xl/2xl`，禁止原始像素值和任意布局尺寸；增加自动设计门禁、统一页面状态与健康检查 |

修改选型时同步本文、AGENTS、相关设计/计划和 HANDOVER；只有真实实施与验证后才更新 BACKLOG 完成状态。本文不占正式 doc_no，也不代替完整总体设计。独立客户端 GitHub 仓库已同步更名为 `hotkey-app`。
