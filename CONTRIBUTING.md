# 为 HotKey Server 贡献

感谢你愿意参与 HotKey。我们欢迎 Bug 修复、来源连接器、测试、可观测性、性能、文档和真实使用反馈。

参与社区即表示你同意遵守 [行为准则](CODE_OF_CONDUCT.md)。发现安全问题时，请不要创建公开 Issue，改用 [安全策略](SECURITY.md) 中的私密渠道。

## 从哪里开始

- 小型修复或文档改进可以直接提交 Pull Request。
- 新功能、Schema/API 变化或大型重构，请先创建 Feature Request 对齐问题、范围和验收标准。
- 新来源必须来自官方 API、RSS、Atom 或授权 Feed，并说明访问政策、限流和失败策略。
- 如果还不确定方案，先描述使用场景和期望结果，不必一开始就给出完整设计。

## 开发环境

1. Fork 并克隆仓库。
2. 安装 Go 1.26+、PostgreSQL 16 + pgvector、Redis 7 和 MinIO。
3. 复制 `.env.example` 为 `.env`，只使用本地或可丢弃凭据。
4. 从空数据库初始化完整 Schema。

```bash
cp .env.example .env
go run ./cmd/hotkey db init --empty-only --confirm-empty
go run ./cmd/hotkey
```

## 开发原则

1. 先阅读与改动相关的 Design、PRD、Plan、Acceptance 和 Operations 文档。
2. 行为变更遵循测试先行；纯文档和机械配置变更可以说明为什么不需要新增测试。
3. 模块依赖固定为 `transport/http -> application -> domain` 和 `infrastructure -> domain`。
4. Domain 不导入 Gin、GORM、River、MinIO 或第三方平台 SDK。
5. 公共 HTTP API 只开放安全业务操作，不提供通用表 CRUD。
6. PostgreSQL 是业务事实源，MinIO 保存原始证据，Vault 是人类可读投影。
7. 不提交密钥、个人数据、Vault 内容、MinIO 对象、数据库 dump 或本地工具目录。
8. 所有 `*_test.go` 位于 `test/`；包级测试通过项目 runner 执行。

完整仓库约束见 [AGENTS.md](AGENTS.md)，长期技术决策见 [docs/](docs/README.md)。

## 提交前验证

按改动风险至少运行：

```bash
make lint
make test
make build
make validate
git diff --check
```

涉及 Go 代码、Schema、OpenAPI、依赖或 CI 时，运行完整门禁：

```bash
HOTKEY_TEST_DSN='postgres://USER@localhost:5432/hotkey_test?sslmode=disable' \
HOTKEY_TEST_REDIS_URL='redis://127.0.0.1:6379/15' \
make ci
make clean
```

`HOTKEY_TEST_DSN` 必须指向可丢弃数据库，所用角色需要 `CREATE DATABASE` / `DROP DATABASE` 权限。

## 提交与 Pull Request

推荐使用清晰、单一目的的提交：

- `test:` 测试、fixture 和验收门禁
- `feat:` / `fix:` 功能或修复
- `refactor:` 不改变行为的重构
- `docs:` 文档与示例
- `chore:` 工具链和维护配置

Pull Request 请包含：

- 解决的问题与用户影响
- 主要实现和边界选择
- 新增或更新的测试
- 实际运行的命令与结果
- Schema、OpenAPI、配置或部署影响
- 必要的截图、日志或验收证据（请移除敏感信息）

维护者会重点审查契约兼容性、数据安全、来源合规、错误可观测性和测试证据。

## 文档贡献

修正文案、示例、链接和上手流程非常有价值。长期影响架构、需求、验收或运维的内容，请放入对应 `docs/` 分类并遵守该目录的 frontmatter 和索引规则。

再次感谢你帮助 HotKey 变得更可靠、更易用。
