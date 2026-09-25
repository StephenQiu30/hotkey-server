# HotKey Server 交接

更新日期：2026-09-25。本文件只记录当前实现快照，≤ 5 KB。v1.x 的详细交接日志见 Git 提交 `3a72611a`。

## 当前状态

- **方向**：2026-09-25 按代码评审转向 v2.0，目标是日报、周报、知识库。需求、设计、计划见 [docs/README.md](docs/README.md)。
- **分支**：文档整理在 `docs/pivot-daily-report`，尚未提交。
- **产品验收**：v2.0 AC 0/9；尚无真实社交来源数据，尚无分析、报告、推送、知识库代码。
- **测试**：后端单元测试 357 passed（2026-09-25，`uv run pytest tests/unit`）。

## 已实现（可复用）

| 能力 | 位置 | 说明 |
|---|---|---|
| 单 owner 登录、会话、CSRF | `identity/`、`api/` | 首次初始化需 `HOTKEY_BOOTSTRAP_TOKEN` |
| 主题与关键词规则 | `monitors/` | any/all/exclude、版本、预览；Web `/monitors` |
| 来源连接与秘密 | `connections/` | Web `/sources` |
| 持久任务执行 | `jobs/`、`worker/` | Outbox → Kafka → Worker；重试、取消、租约、恢复；Web `/jobs` |
| 内容存储 | `content/` | 帖子身份、版本、指标观察；Web `/content` |
| 关键词搜索执行器 | `content/discovery_execution.py` | 已写好，未注册到 Worker |
| 网页正文采集 | `sources/adapters/firecrawl.py` | 唯一已注册的处理器 `webpage.collect` |
| 浏览器服务 | `backend/browser/`、Compose `browser` | Playwright 1.63.0；出口代理只放行 `example.com` |
| X 官方 API | `sources/adapters/x_api.py` | 仅 MockTransport；无 Token，未发真实请求 |
| 预算账本 | `jobs/`（resource_budget_*） | 可复用为模型与付费来源的成本上限 |
| 备份 | `backups/` | PostgreSQL + MinIO 备份与隔离恢复 |

## 已知缺口（Design 001 第 2 节）

Worker 只有 `webpage.collect`；评论无法入库；无周期调度；无模型/分析/事件/报告/推送/知识库；来源契约缺标题、昵称、浏览量、热榜；无全文与向量索引。

## 运行

- 编排：根 `compose.yaml`（PostgreSQL 17.11、Redis 7.2.16、Kafka 4.1.2、backend、worker、frontend、browser、browser-egress）。API 端口 8867，Web 端口 3000。
- 本机开发：未跟踪的 `backend/.env`、`frontend/.env.local` 复用本机 Homebrew PostgreSQL（`127.0.0.1:5432`，库名 `hotkey-server`）、Redis、Kafka、MinIO。该库 owner_count=0、job_count=0，可按规则重建。不要对旧库 `hotkey`、`hotkey_dev`、`hotkey_test` 执行 `schema.sql`。
- 入口（在 `backend/app/` 下）：`uvicorn main:create_app --factory`、`python -m worker`、`python -m cli`。
- 检查：后端 `uv run ruff check`、`uv run mypy`、`uv run pytest`；前端 `pnpm lint`、`pnpm typecheck`、`pnpm build`、`pnpm openapi:check`。

## 下一步

1. 用户确认 OPEN-001-101/102/106。
2. 提交本次文档整理。
3. 从 P1-T01—T05 开始（不依赖模型预算），并行推进 P1-T06 选型。
