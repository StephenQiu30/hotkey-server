# Contributing

感谢参与 HotKey Server。代码、数据库结构、测试和设计文档必须保持一致，不接受只修改其中一层的功能提交。

## 开发原则

1. 先阅读 `AGENTS.md` 和任务涉及的 `docs/design/` 文档。
2. 行为变更先补失败测试，再提交最小实现。
3. 模块依赖固定为 `transport/http -> application -> domain` 和 `infrastructure -> domain`。
4. Domain 不导入 Gin、GORM、River、MinIO 或第三方平台 SDK。
5. 每张业务表实现统一 CRUD Repository；运行历史和审计表使用受限 Repository。
6. 公共 HTTP API 只开放安全业务操作，禁止通用表 CRUD API。
7. 禁止重新引入 Kafka、核心 Redis 依赖、微服务或 Git 知识库工作流。
8. 不提交密钥、Vault 内容、MinIO 对象、数据库数据和 `.tools/`。

## 提交流程

提交前至少运行：

```powershell
make lint
make test
make build
make validate
git diff --check
```

涉及 Go 代码、Schema、OpenAPI、依赖或 CI 时，必须复现完整质量门禁。`HOTKEY_TEST_DSN` 必须是可丢弃且具备 `CREATE DATABASE` / `DROP DATABASE` 权限的 PostgreSQL URL；认证测试还需要独立的 Redis DB：

```bash
HOTKEY_TEST_DSN='postgres://USER@localhost:5432/hotkey_test?sslmode=disable' \
HOTKEY_TEST_REDIS_URL='redis://127.0.0.1:6379/15' \
make ci
make clean
```

GitHub Actions 在推送到 `main` 和面向 `main` 的 Pull Request 中执行同一个 `make ci` 门禁。工作流、服务依赖或该命令的前置条件变更时，必须同步更新 [CI 运维手册](docs/operations/001-本地与GitHub%20CI质量门禁.md)、README 和本文档。

推荐按职责拆分提交：

- `test:` 测试、fixture 和验收门禁
- `impl:` 或 `feat:` 使测试通过的实现
- `refactor:` 不改变行为的结构调整或旧代码清退
- `docs:` 设计和使用说明
- `chore:` 工具链与维护配置

数据库结构只通过完整 `db/schema.sql` 修改；不得引入 `db/migrations/`、Goose 或 GORM AutoMigrate。API 契约变更必须同步 OpenAPI、Transport 测试和相关设计文档。
