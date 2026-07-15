# HotKey Server

HotKey Server 是一个本地优先、面向个人与小团队的 AI 热点事件监控和 Obsidian 知识库治理后端。

项目当前正在按 `docs/design/001`–`012` 进行绿色重建。旧的 Kafka、Redis 核心任务、Topic/Event/HotEvent 重复模型、Git 知识库提交链路及旧数据库结构已经移除，不提供兼容运行路径。

## 目标能力

- 通过 RSS/Atom、GDELT/新闻 API、Reddit、Hacker News、YouTube 和 X 官方 API 采集候选内容
- 完成标准化、去重、多语言相关性判断、事件聚类、热度与趋势分析
- 将原始证据写入 MinIO，将可阅读知识投影写入本地 Obsidian Vault
- 生成日报、周报，并通过邮件和 RSS/Atom 交付
- 使用 PostgreSQL 保存业务事实和 River 后台任务状态
- 使用同一个 Go 二进制以 `all`、`api` 或 `worker` 角色运行

## 架构约束

- 模块化单体，不拆分微服务
- PostgreSQL 是唯一业务事实源
- 业务模块采用 `domain -> application -> transport/infrastructure` 边界
- 每张业务表提供统一 Repository CRUD，公共 API 仅开放安全业务操作
- 知识库运行链路不依赖 Git
- 当前阶段不提供 Docker 或线上部署编排

## 开发状态

当前分支先建立可编译骨架、完整 `db/schema.sql` 和统一 Repository，再按监控、采集、事件、热度、AI、知识、报告和投递顺序实现业务能力。目标设计从 [设计索引](docs/design/README.md) 开始阅读，任务边界见 [PRD 索引](docs/prd/README.md)，具体文件、步骤与验证命令见 [Plan 索引](docs/plans/README.md)。

常用验证命令：

```bash
make lint
make test
make build
make validate
make database-runtime-verify
make ci
```

完整 Schema 的空库验证使用可丢弃的 PostgreSQL 16+ 与 pgvector 数据库：

```bash
HOTKEY_TEST_DSN='postgres://hotkey:hotkey@localhost:5432/hotkey_test?sslmode=disable' make database-runtime-verify
```

该命令会重建目标测试库的 `public` schema，再执行嵌入 Schema 的空库初始化、只读兼容性检查、真实 PostgreSQL 集成测试与游标计划验证。每个 Go 集成测试与容量 fixture 都会创建并删除自己的数据库，因此 `HOTKEY_TEST_DSN` 必须是可丢弃的 PostgreSQL URL，且其角色需要 `CREATE DATABASE` / `DROP DATABASE` 权限；容量 fixture 不与命令或测试共享数据库。运行服务时则使用 `HOTKEY_DATABASE_URL`：

```bash
HOTKEY_DATABASE_URL='postgres://hotkey:hotkey@localhost:5432/hotkey?sslmode=disable' go run ./cmd/hotkey db verify
HOTKEY_DATABASE_URL='postgres://hotkey:hotkey@localhost:5432/hotkey_new?sslmode=disable' go run ./cmd/hotkey db init --empty-only --confirm-empty
```

`make ci` 也会执行该验证，因此 CI 必须提供 `HOTKEY_TEST_DSN`。

本地 Go 工具链可以放在未跟踪的 `.tools/go` 目录，或直接使用系统中的 Go 1.26+。

## 许可证

[MIT](LICENSE)
