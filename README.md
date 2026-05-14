# ai-hotspot-radar

`ai-hotspot-radar` 是一个可自部署的 AI 热点监控工具 MVP。

当前项目从零重建：旧实现、旧目录结构、旧数据库结构、旧 OpenAPI 契约和旧示例数据均不保留。后续实现以 `docs/plans/` 下的执行计划为准。

## 技术栈

- 前端：`Next.js + TypeScript`
- 后端：`Python + FastAPI`
- 数据库：`PostgreSQL`
- ORM：`SQLAlchemy 2.0`
- Schema 管理：`sql/*.sql` + `SQLAlchemy 2.0` models（无迁移）
- 邮件：`SMTP`
- AI：OpenAI 兼容模型 API
- 部署：本机 PostgreSQL + 本地进程；Docker Compose 仅作为可选的 API/Web 容器启动方式

## 新目录说明

- `apps/api/`：FastAPI 后端（含 SQLAlchemy models 与初始化入口）
- `apps/web/`：Next.js 控制台
- `packages/core/`：轻量共享常量、类型或规则说明
- `sql/`：PostgreSQL 表结构 SQL，当前以 `001_init_schema.sql` 为事实源
- `infra/`：环境变量、Docker 可选配置和部署配置
- `docs/plans/`：拆分后的执行计划
- `docs/product/`：PRD 与产品事实源
- `docs/engineering/`：技术方案与验收标准

## 文档入口

- [协作规范](./AGENTS.md)
- [文档导航](./docs/文档说明.md)
- [产品需求](./docs/product/产品需求文档.md)
- [执行计划导航](./docs/product/执行计划导航.md)
- [技术方案](./docs/engineering/技术方案.md)
- [验收标准](./docs/engineering/验收标准.md)
- [执行计划](./docs/plans/00-基础工程计划.md)

## 本地开发（说明）

```bash
pip install -e .
npm --prefix apps/web install
```

启动 API：

```bash
npm run api:dev
```

启动 Web：

```bash
npm run web:dev
```

运行时依赖（本仓库首选）：

- Python、Node 不要求虚拟环境，使用系统可执行环境直接运行。
- PostgreSQL/Redis 使用本机 Homebrew 安装的服务（默认 `localhost`）；不要在仓库内再次创建数据库容器。

数据库连接：

```bash
cp infra/env/.env.example infra/env/.env
```

然后把 `infra/env/.env` 中 `DATABASE_URL` 改成你本机 PostgreSQL 的连接串，例如：

```bash
DATABASE_URL=postgresql+psycopg://你的用户:你的密码@localhost:5432/ai_hotspot_radar
REDIS_URL=redis://localhost:6379/0
```

本机 PostgreSQL 与 Redis 可通过 Homebrew 安装启动，例如：

```bash
brew install postgresql redis
brew services start postgresql
brew services start redis
```

本机 PostgreSQL 可以使用你已经创建的 `root` 角色；Redis 不需要创建额外实例；真实密码只写入本地 `infra/env/.env`，不要提交到 GitHub。

需要填写的环境变量如下。可选项不使用时保持为空，系统会自动降级或跳过对应能力：

| 变量 | 是否必填 | 用途 | 你需要填写 |
| --- | --- | --- | --- |
| `DATABASE_URL` | 必填 | 连接本机 PostgreSQL | PostgreSQL 用户、密码、主机、端口和数据库名 |
| `OPENAI_API_KEY` | 可选 | 启用模型查询扩展与热点分析 | OpenAI 兼容模型 API Key |
| `OPENAI_BASE_URL` | 可选 | OpenAI 兼容接口地址 | 例如 `https://api.openai.com/v1` 或你的代理地址 |
| `OPENAI_MODEL` | 可选 | 模型名称 | 例如 `gpt-4o-mini` 或你实际使用的模型 |
| `X_API_BEARER_TOKEN` | 可选 | 启用 X/Twitter Recent Search | X API v2 Bearer Token |
| `BING_SEARCH_API_KEY` | 可选 | 启用 Bing Search 来源 | Bing Search API Key |
| `SMTP_HOST` | 可选 | 启用事件邮件和报告邮件 | SMTP 服务器地址 |
| `SMTP_PORT` | 可选 | SMTP 端口 | 通常是 `587` |
| `SMTP_USERNAME` | 可选 | SMTP 登录用户 | SMTP 用户名 |
| `SMTP_PASSWORD` | 可选 | SMTP 登录密码 | SMTP 密码或应用专用密码 |
| `SMTP_FROM_EMAIL` | 可选 | 邮件发件人 | 发件邮箱 |
| `SMTP_TO_EMAIL` | 可选 | 邮件收件人 | 收件邮箱 |
| `NEXT_PUBLIC_API_BASE_URL` | 本地前端必填 | 前端访问后端 API | 本地默认 `http://localhost:8000` |

`infra/env/.env.example` 和 `infra/env/.env` 已保留占位注释。为了避免占位文本被当成真实密钥，可选密钥变量默认保持空值。

数据库初始化：

```bash
npm run db:init
```

如果需要重置数据库，直接在本机 PostgreSQL 中删除并重建 `ai_hotspot_radar` 数据库，再执行：

```bash
npm run db:init
```

可选 Docker 启动 API/Web：

```bash
npm run docker:up
```

数据库表结构：

- 表结构事实源位于 `sql/001_init_schema.sql`。
- API 启动时会执行该 SQL 文件初始化本机 PostgreSQL 中的空数据库。
- SQLAlchemy models 只负责运行时访问数据库，必须与 SQL 文件保持一致。

## 后端能力

- 热点检查：`POST /api/check-runs`
- 热点列表：`GET /api/hotspots`
- 全网搜索：`POST /api/search`
- 单条热点邮件通知：SMTP 配置存在时自动发送
- 日报/周报生成：`POST /api/reports`
- 日报/周报发送：`POST /api/reports/{report_id}/send`
- 日报/周报列表：`GET /api/reports`

日报/周报默认不自动发送；如需简单定时发送报告，可在本地 `.env` 中开启：

```bash
DAILY_REPORT_ENABLED=true
DAILY_REPORT_HOUR=8
WEEKLY_REPORT_ENABLED=true
WEEKLY_REPORT_WEEKDAY=1
WEEKLY_REPORT_HOUR=8
```

当前后端支持 RSS、Hacker News、X/Twitter、Bing、Bilibili、Sogou-style 多源 adapter。X/Twitter 使用官方 X API v2 Recent Search，需要配置：

```bash
X_API_BEARER_TOKEN=你的XBearerToken
```

Bing 搜索源需要配置：

```bash
BING_SEARCH_API_KEY=你的BingSearchKey
```

低于 `RELEVANCE_THRESHOLD` 或被 AI 判定为不真实的热点会保留为 `filtered`，但不会发送事件邮件，也不会进入日报/周报。

## 当前状态

- 已移除旧实现结构。
- 已建立新项目骨架。
- 已写入 `docs/plans/` 执行计划。
- 后端与控制台 MVP 已按 OpenSpec 计划推进，后续继续按计划文件小步完善。
