# 为 HotKey 贡献

HotKey 单体仓库包含 `backend/`、`frontend/` 与统一 `docs/`。开始修改前，请阅读仓库根目录唯一的 `AGENTS.md`；参与协作时保持尊重并聚焦可验证事实。

## 从哪里开始

- 小型修复、测试、文案和文档改进可以直接提交 Pull Request。
- 新功能、Schema/API 变化、跨页面交互或大型重构，请先创建 Feature Request 对齐问题、范围和验收标准。
- 新数据来源只能使用官方 API、RSS、Atom 或授权 Feed，并说明访问政策、限流与失败策略。
- UI 改动请说明目标用户、桌面与移动视口、交互状态和可访问性影响。
- 安全问题不得公开披露，请按 [安全策略](SECURITY.md) 使用私密报告渠道。

## 本地开发

```bash
git clone https://github.com/StephenQiu30/hotkey-server.git
cd hotkey-server

# 后端
cd backend
cp .env.example .env
go run ./cmd/hotkey db init --empty-only --confirm-empty
go run ./cmd/hotkey

# 前端（另开终端）
cd frontend
npm ci
cp .env.example .env
npm run dev
```

后端需要 Go 1.26+、PostgreSQL 16 + pgvector、Redis 7 和 MinIO；前端使用仓库锁定的 Node.js 依赖。只使用本地或可丢弃凭据。

## 开发约束

- 后端遵守模块依赖 `transport/http -> application -> domain` 与 `infrastructure -> domain`，公共 API 不提供通用表 CRUD。
- PostgreSQL 是业务事实源，MinIO 保存原始证据，Vault 是人类可读投影；所有 `*_test.go` 位于 `backend/test/`。
- 前端使用 Next.js App Router、React、TypeScript 和现有 UI 组合组件；测试位于 `frontend/test/`。
- API 类型与请求函数只由根 [`docs/openapi/swagger.json`](docs/openapi/swagger.json) 生成，不手写后端 DTO 或接口路径。
- 不提交 `.env`、Token、个人数据、数据库 dump、Vault、对象存储内容、构建产物或本地工具目录。

## OpenAPI 协作

1. 在 `backend/` 修改接口并执行 `make openapi`，同步运行时注册代码与根发布契约。
2. 在 `frontend/` 执行 `npm run openapi:generate` 并审查生成差异。
3. 执行 `npm run openapi:check`，确认发布契约和客户端没有漂移。
4. 业务代码只调用 `frontend/src/services/hotkey/hotkey-server/` 中的生成函数。

## 提交前验证

按变更范围至少运行：

```bash
cd backend
make lint
make test
make build
make validate

cd ../frontend
npm run openapi:check
npm run typecheck
npm run test:unit
npm run build

cd ..
git diff --check
```

涉及 Go 代码、Schema、OpenAPI、依赖或 CI 时，在配置可丢弃 PostgreSQL 和 Redis 测试环境后运行 `make ci`。Pull Request 必须说明用户影响、实现边界、测试结果、Schema/OpenAPI/配置/部署影响、必要的截图或日志，以及仍未验证的风险。

长期影响架构、需求、验收或运维的内容放入根 `docs/` 对应分类，使用 `scope` 标识前端、后端或共享范围，并遵守 frontmatter 与索引规则。
