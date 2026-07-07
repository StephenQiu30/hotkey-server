# Engineering Upgrade — Proposal

**Date:** 2026-07-07

## Summary

对 hotkey-server 进行全面工程化改造，从当前手写 SQL + 手动拼装架构升级为 **Go + Gin + GORM + Fx + PostgreSQL + Redis** 工程化体系，并在改造过程中修复全部 20 个代码审查发现的问题（含 3 CRITICAL + 6 HIGH）。

## Motivation

1. 当前 66 处 Raw SQL 散落在 28 个 database 文件中，维护成本高
2. `monitorrepo.go` 手工拼接 `$N` 占位符字符串，存在 SQL 注入风险
3. 无 DI 容器，依赖关系靠人脑维护顺序
4. 代码审查发现 3 个 CRITICAL bug（趋势不存、打分失效、无 context 传播）
5. 无缓存层、无版本化迁移、无测试体系

## Scope

- 目录分层重构（model/repository/service/handler/worker/cache）
- 引入 Uber Fx 依赖注入
- 全部 Raw SQL 改为 GORM builder
- 引入 goose 版本化迁移
- 引入 Redis 缓存层（Cache-Aside）
- 引入测试体系（gomock + testcontainers）
- 修复 20 个代码审查发现的问题

## Non-goals

- 不更换 HTTP 框架（保留 Gin）
- 不引入微服务拆分
- 不改变现有 API 契约
- 不改变业务功能

## Normative Files Changed

- `CLAUDE.md` — 新增 Fx/GORM 使用规范
- `openspec/specs/` — 新增工程化架构 specs
