# CLAUDE.local.md

本文件记录 `hotkey-server` 的局部规范，与 `CLAUDE.md` 的全局协作规则区分。

## 使用边界

1. `CLAUDE.md` 存放长期稳定的 Claude 全局规则。
2. `CLAUDE.local.md` 存放本仓库特有的路径、命令、技术栈与环境约束。
3. 角色与流程只维护在 `.claude/agents/` 与 `.claude/skills/`。

## 技术栈

- Go 1.25 + Gin
- PostgreSQL + pgvector
- Redis
- 阿里云 DashScope（Qwen、text-embedding-v2）
- Docker Compose（本地 dev / 生产全栈）

## 项目结构

```text
cmd/hotkey-api/              # 服务入口
internal/
  app/                       # runtime 装配
  config/                    # 环境配置
  transport/http/            # Gin 路由与 handler
  service/                   # 领域服务
  repository/postgres/       # 持久化
  platform/                  # 外部集成（Redis、DashScope、SMTP、MinIO…）
  worker/、scheduler/、queue/
migrations/                  # 数据库迁移
db/schema.sql                # 生产 compose 初始化 schema
docs/                        # PRD、Plan、Design、OpenAPI 静态产物
openspec/                    # SDD 规范层
tests/                       # workflow 校验与 E2E
```

## 常用命令

```bash
cp .env.example .env
make test          # go test + 仓库治理校验
make run           # go run ./cmd/hotkey-api
make fmt           # gofmt
make compose-up    # 本地 Docker（API + Web，DB/Redis 在宿主机）
curl http://127.0.0.1:8080/healthz
```

## 文档与 OpenSpec

- PRD：`docs/prd/`
- Plan：`docs/plans/`
- Design：`docs/design/`
- OpenAPI 静态产物：`docs/openapi.yaml`
- OpenSpec 变更：`openspec/changes/`；已接受规范：`openspec/specs/`

## 环境变量

关键项见 `.env.example`：`HOTKEY_HTTP_ADDR`、`HOTKEY_DATABASE_URL`、`HOTKEY_REDIS_URL`、`HOTKEY_DASHSCOPE_API_KEY`。

## 跨仓边界

- 本仓是 API 契约事实源；接口变更先稳定 `docs/openapi.yaml` 与实现，再通知 `hotkey-web` / `hotkey-miniapp` 重新生成客户端。
- 默认开发顺序：`server -> web -> miniapp -> 回归`。
