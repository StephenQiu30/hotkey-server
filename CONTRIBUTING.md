# Contributing

感谢参与 HotKey Server。代码、数据库迁移、测试和设计文档必须保持一致，不接受只修改其中一层的功能提交。

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

推荐按职责拆分提交：

- `test:` 测试、fixture 和验收门禁
- `impl:` 或 `feat:` 使测试通过的实现
- `refactor:` 不改变行为的结构调整或旧代码清退
- `docs:` 设计和使用说明
- `chore:` 工具链与维护配置

数据库结构只通过 `db/migrations/` 中的 Goose SQL Migration 修改。API 契约变更必须同步 OpenAPI、Transport 测试和相关设计文档。
