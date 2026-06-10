# Contributing

感谢改进 HotKey Server。

## 贡献范围

1. 修正 `CLAUDE.md`、`CLAUDE.local.md`、`WORKFLOW.md` 中的协作规范。
2. 补充 `.claude/agents/` 角色与 `.claude/skills/` 流程。
3. 补充 `docs/`、`openspec/` 下的长期文档与规范。
4. 改进 Go 实现、测试与部署配置。

## 原则

1. MVP 优先，不引入当前无使用场景的复杂流程。
2. 保持 TDD、SMART、OpenSpec、Git/PR 规范一致。
3. 文档必须基于真实目录结构，不描述不存在的路径。
4. Agent 配置只落在 `.claude/`，不新增并行规范目录。

## 提交流程

1. 提交前检查改动范围，避免混入无关文件。
2. 功能 PR 推荐顺序：`test:` → `impl:`/`feat:` → `refactor:` → `docs:`/`chore:`。
3. 提交信息使用中文。
4. PR 使用中文标题，并填写 Test-first Evidence、Commands run、Result、Agent Usage、Reviewer Checklist。
5. 合并前为目标分支打 tag 作为回滚点。
