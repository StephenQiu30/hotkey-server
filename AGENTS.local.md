# AGENTS.local.md

## 说明

本文件承接当前项目在 `AGENTS.md` 基础上的局部规范。当前项目已将原始 `AGENTS.md` 约束收敛为局部规范，用于覆盖 MVP 与阶段化实现要求。

## 原项目 AGENTS 迁移到 Local

# AGENTS

本文件定义本仓库文档、实现和协作约束。当前阶段以“轻量 MVP、开源自部署、可配置关键词的 AI 热点监控工具”为最高优先级。

## 1. 文档主事实源

- 产品需求与范围以 `docs/product/产品需求文档.md` 及其子文件为准。
- 执行计划以 `docs/product/执行计划导航.md` 为准。
- 技术约束以 `docs/engineering/技术方案.md` 为准。
- API 契约由新 FastAPI 实现自动生成；旧 `contracts/openapi` 不再是实现约束。
- 验收口径以 `docs/engineering/验收标准.md` 为准。
- 具体执行任务以 `docs/plans/` 下的 PLAN 文件为准。
- AI 热点监控 MVP 功能范围以 `docs/plans/10-AI热点监控MVP计划.md` 为准。
- 当前后端检测、即时搜索、日报/周报任务以 `docs/plans/11-后端热点检测与报告计划.md` 为准。
- 当前 SaaS 前端平台任务以 `docs/plans/12-SaaS前端平台计划.md` 为准。

## 2. 产品实现方向

- 第一阶段定位为“可自部署、可配置关键词的 AI 热点监控工具”，不是重型舆情平台。
- 功能实现围绕本项目轻量 MVP 能力：关键词管理、多源抓取、AI 查询扩展、真假识别、相关性分析、热点列表、筛选排序、即时搜索、邮件通知、日报/周报、手动触发和定时触发。
- 本项目自主设计功能闭环；本仓库使用 `Next.js + TypeScript` 前端、`Python + FastAPI` 后端、`PostgreSQL` 数据库、`SQLAlchemy 2.0` ORM、`SMTP` 邮件。
- P0 必须围绕可运行闭环：配置关键词 -> 手动或定时触发 -> AI 查询扩展 -> 抓取多源内容 -> 去重入库 -> AI 判断相关性/真实性 -> 阈值过滤 -> 热点展示/即时搜索 -> 事件邮件通知 -> 日报/周报。
- MVP 功能闭环继续使用 `Python + FastAPI + PostgreSQL + SQLAlchemy + Next.js`。
- 第一阶段数据源范围为 RSS、Hacker News、X/Twitter、Bing、Bilibili、Sogou-style；新增来源必须走统一 adapter 和 `Candidate` 输出。
- X/Twitter 必须使用官方 X API v2 Recent Search，通过 `X_API_BEARER_TOKEN` 注入凭据；不得引入页面爬取作为默认实现。
- 低于 `RELEVANCE_THRESHOLD` 或被判定为不真实的热点必须标记为 `filtered`，不得发送事件邮件，不得进入日报/周报。
- 达到 `RELEVANCE_THRESHOLD` 且 `is_real is not False` 的热点标记为 `active`，允许进入热点流、事件邮件和日报/周报。
- 后端检测、即时搜索、日报/周报生成阶段已完成后，当前阶段允许实现单用户私有部署 SaaS 前端平台。
- SaaS 前端首版以用户自己直接使用为目标，不实现多用户、租户隔离、真实登录认证、真实计费或 Stripe。
- SaaS 前端产品形态为 `/` 官网首页、`/pricing` 定价占位页、`/app` 工作台。
- 日报/周报生成采用本地 Markdown 模板作为 P0 必需能力；AI 只作为后续可选增强，不作为当前验收前提。
- `/api/daily-reports` 已移除，不做兼容别名；报告 API 统一收敛到 `/api/reports`。
- P0 不做多租户、复杂权限、计费、复杂工作流、实时推送、向量库、复杂队列治理和企业级数据平台。

## 2.1 当前实现状态口径

- 已实现并可用现有自动化验证的能力：基础目录结构、SQL schema、关键词/来源/热点/任务/通知/报告模型、AI 查询扩展降级、AI 分析降级、阈值过滤、`/api/search`、`/api/reports`、SMTP 缺失降级、SaaS 前端页面骨架和主要工作台页面。
- 需配置真实凭据或外部服务后验收的能力：X/Twitter Recent Search、Bing Search、SMTP 真实发送、OpenAI 兼容模型真实调用。
- 暂不进入 P0 的能力：真实登录认证、多用户/多租户、真实计费、实时推送、Agent/Skill 自动化、复杂通知策略、复杂报告模板系统、向量检索和独立任务队列平台。

## 3. 改动规范

- 新增需求必须先落 PRD 对应章节，再映射到技术文档和验收项。
- 不得在未更新 PRD 的情况下扩展新来源、规则或能力范围。
- 每次实现改动应可追踪到某个 PRD P0/P1 条目。
- 新增字段或新增 API 时，必须同步更新 OpenAPI、主技术文档和验收项，不得仅在代码层面变更。
- 当前旧实现、旧目录结构、旧数据库结构、旧 OpenAPI 契约、旧示例数据均不保留。
- 后续实现以 `docs/plans/` 为执行依据，不需要兼容旧代码、旧表结构或旧内存仓库。

## 4. 架构底线

- `server` 承载 FastAPI 后端入口、路由、依赖注入、数据库访问和任务触发入口。
- `hotkey-web`（独立仓）承载 Next.js SaaS 前端平台。
- `packages/core` 承载跨应用共享的轻量类型、常量或文档化规则；不得重新引入旧 `backend/core` 分层。
- `sql/` 是数据库表结构事实源；`server/app/models` 的 SQLAlchemy models 必须与 `sql/001_init_schema.sql` 保持一致。
- `migrations` 已废弃，不引入数据库迁移工具；数据库初始化优先执行 `sql/001_init_schema.sql`，重置时通过清空数据库重建。
- 外部平台、模型服务、邮件服务和数据库访问必须放在 infrastructure/adapter 层或等价隔离层中。
- 当前架构中不利于 MVP 落地的部分已允许删除，包括静态样例数据主链路、启动时隐式初始化业务数据、内存仓库作为生产默认实现。
- 不做旧 schema 迁移，不做旧数据迁移，不读取旧表、旧 bootstrap 数据或旧内存仓库。

## 5. 配置与运行约束

- 依赖与凭据仅允许通过环境变量注入。
- 环境变量只保留必要配置：PostgreSQL 连接、模型 API Key、可选 X/Twitter Key、SMTP 配置和服务端口。
- 任何需要 API Key、Token、SMTP 密码或模型凭据的功能，都必须先在根目录 `.env.example` 和本地 `.env` 中以安全占位注释登记；实际值只填写到本地 `.env`，不得提交真实密钥。
- 可选凭据默认值必须保持为空，不能用非空假值占位，避免系统误判为已配置并触发真实外部调用。
- PostgreSQL 默认使用用户本机已有实例；不要为 P0 默认开发链路重新创建 Docker PostgreSQL 环境。
- 邮件未配置时系统仍必须可运行，只是不发送邮件。
- X/Twitter 未配置时系统仍必须可运行，只跳过该来源。
- Bing 未配置时系统仍必须可运行，只跳过该来源。
- 单个数据源失败不能中断整个热点检查任务。
- 手动触发和定时触发都必须走同一条业务编排链路。
- 日报/周报可通过 API 手动触发；定时触发复用轻量 scheduler，不引入 Celery、Redis、向量库或复杂任务平台。

## 6. 代码与测试

- 新增行为默认配套测试；核心流程至少覆盖正向主链路。
- 关键词管理、热点筛选、去重入库、AI 分析降级、邮件未配置降级必须有测试或可验收用例。
- 文档更新、API 更新、代码提交应同步进行，不出现“先写代码后补文档”。
- 每次实现改动必须先补测试并执行测试通过后再提交，提交前必须确认 `git status --short` 为空，确保工作区无中间产物。
- 后端实现统一以 `server` 目录为入口，不维护独立 `apps` 运行时目录；历史历史归档中的 `apps/...` 参考可保留。

## 7. 评审标准

- 评审时优先看“是否可运行、是否可配置、是否可回滚、失败是否可追踪”，其次看设计优雅性。
- 能用简单配置解决的问题，不引入复杂平台能力。
- 能围绕本项目 MVP 功能闭环直接落地的问题，不提前抽象为企业级平台。
- 能通过 PostgreSQL + SQLAlchemy + FastAPI 直接完成的能力，不引入 Prisma、SQLite、Celery、Redis 或旧 OpenAPI 约束。
