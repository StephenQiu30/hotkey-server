# 贡献指南

遵循根 [AGENTS.md](AGENTS.md)。本项目用于个人学习，技术栈以根 [PROJECT.md](PROJECT.md) 为准：Python/FastAPI/SQLAlchemy 2/PostgreSQL/Redis/Kafka，Web 使用 pnpm/Next.js/shadcn/ui/Radix UI/Tailwind CSS/Axios/ESLint/Prettier。

当前已有规范、规划文档及 backend/frontend 工程。后续实现前，按 [文档模板](docs/TEMPLATE.md) 建立对应设计、需求和计划，说明本次切片与实际验收范围，再实现和验证。新增来源必须给出搜索/详情/评论/回复的独立能力状态、可维护的测试样本和有界分页；不提交真实账号会话、用户隐私或未经核验的第三方代码。

每次变更应执行格式、类型、测试、契约及构建检查。涉及数据库/队列必须执行真实服务集成；涉及 UI 执行 Playwright。测试数据只能写入可丢弃测试库。数据库 DDL 只修改 `backend/database/schema.sql`，并与 SQLAlchemy Model 同批验证；禁止使用 metadata.create_all、应用启动建表或其他结构事实源。

API 输入输出为 Pydantic 模型，业务服务拥有事务，持久化使用 SQLAlchemy。客户端从 OpenAPI 生成类型，不增加兼容旧接口层。PR 描述包含具体变化、验证证据、未覆盖范围；不得把目标设计标为已实现。

## Git 提交规范

提交标题统一使用 `type(scope):中文描述`，冒号后不加空格。`scope` 必填，使用稳定的小写英文模块名；`type` 仅使用 `feat`、`fix`、`test`、`refactor`、`docs`、`chore`、`perf`、`build`、`ci` 或 `revert`。描述使用具体的简体中文动宾短语，标题不超过 72 个字符。

```text
feat(api):新增任务状态查询
docs(repo):补充提交规范
```

提交正文和脚注使用简体中文，并说明变更摘要、原因和实际验证结果。不兼容变更使用 `<type>(<scope>)!:`，并以 `BREAKING CHANGE:` 记录中文迁移说明。
