# hotkey-server

`hotkey-server` 是 HotKey AI 实时热点监测小程序的后端仓库，当前处于 Go 后端全面重建阶段。

本仓库是跨仓规范主源，也是未来 OpenAPI 契约事实源。`hotkey-web` 和 `hotkey-miniapp` 必须以后端导出的 OpenAPI 为准生成客户端，不手写后端 API 类型。

## 当前状态

- 旧 FastAPI 运行时、Python 测试、旧 Docker/Compose、旧 SQL 初始化和旧 OpenSpec 实现内容已移除。
- 当前只保留开源治理文件、Go 重建 PRD/Plan、工程设计和 OpenSpec 配置入口。
- 新实现必须从 `docs/product/prd/` 与 `docs/plans/` 的连续编号任务开始推进。

## 目标技术栈

- Go
- PostgreSQL
- pgvector
- Redis
- OpenAPI 生成/导出

## 文档入口

- [AGENTS.md](./AGENTS.md)：跨仓主规范源。
- [AGENTS.local.md](./AGENTS.local.md)：当前仓库局部补充规则。
- [docs/README.md](./docs/README.md)：Go 重建后的长期文档入口。
- [docs/engineering/1-Go后端重建与开源仓库治理设计.md](./docs/engineering/1-Go后端重建与开源仓库治理设计.md)：目标架构与任务编排规则。

## 任务编号

- `1-13`：P0 开源核心闭环。
- `14-16`：P1 平台化能力。
- `17-19`：P2 商业化与规模化能力。
- `20-22`：P3 高级实时与事件图谱。

每个任务必须同时维护：

```text
docs/product/prd/N-能力名称PRD.md
docs/plans/N-能力名称实现计划.md
```

## 本地验证

当前阶段尚未引入 Go 运行时代码。提交前至少执行：

```bash
git status --short
git diff --check
find docs/product/prd docs/plans -maxdepth 1 -type f | sort -V
```

实现 Go 服务后，应补充 `go test ./...`、OpenAPI 导出和端侧客户端生成验证。
