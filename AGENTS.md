# AGENTS.md

本仓库是 HotKey 的 Go 后端 `hotkey-server`。

## 规范入口

- 协作规范：`CLAUDE.md`（全局）+ `CLAUDE.local.md`（本仓局部）
- Symphony 调度：`WORKFLOW.md`
- 角色与技能：`.claude/agents/`、`.claude/skills/`（**仅此一处**，不维护 `.codex/`、`.agents/`）

## 范围

- 只在本仓实现 server 能力；不要改 `hotkey-web` / `hotkey-miniapp`，除非 ticket 明确要求。
- Linear issue + Workpad 是任务事实源。

## Go 约定

- 布局：`cmd/`、`internal/`、`db/schema.sql`
- HTTP：`internal/transport/http`
- 外部集成：`internal/platform`
- 持久化：`internal/repository/postgres`

## 交付前检查

```bash
gofmt -w cmd internal
make test
```
