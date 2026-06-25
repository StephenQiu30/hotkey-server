# hotkey-server

X 热点监控平台后端服务：API、采集、热点评分、主题聚合、趋势分析、告警通知与 OpenAPI 契约事实源。

## 架构

- `cmd/hotkey` — 唯一入口（启动 API + Worker）
- `internal/app` — 应用启动逻辑
- `internal/auth` — 账号注册/登录
- `internal/monitor` — 监控任务 CRUD
- `internal/content` — 内容标准化与 hits 评分
- `internal/topic` — 主题聚合
- `internal/trend` — 趋势分析
- `internal/alert` — 告警模型
- `internal/notify` — 通知服务（站内 + 邮件）
- `internal/platform/http` — Gin HTTP API（唯一 HTTP 主线）
- `internal/database` — GORM 持久化（API 与 Worker 共用）
- `internal/jobs` — 后台任务（poll/aggregate/snapshot/dispatch）
- `internal/scoring` — 热点评分
- `internal/platform/x` — X 平台采集客户端
- `internal/observability` — 结构化日志
- `internal/config` — 环境变量配置加载
- `db/schema.sql` — PostgreSQL 数据库 schema

## 本地开发

```bash
# 一条命令启动（API + Worker，自动建库并初始化 schema）
make dev

# 或构建后直接运行
make build
./hotkey-server

# Docker 环境（PostgreSQL + 应用）
bash scripts/start-local.sh
```

## 验证

```bash
bash scripts/validate-repository.sh
```

## Agent 规范

- `CLAUDE.md` — Claude 协作规范
- `CLAUDE.local.md` — 本项目局部配置
- `WORKFLOW.md` — Symphony / Linear 调度契约
- `.claude/agents/` — 角色定义
- `.claude/skills/` — 可复用工作流
