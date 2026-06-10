# HotKey 文档中心

本目录存放 Go 后端长期事实源文档。临时任务清单、过程流水账和一次性排查材料不进入 `docs/`。

## 当前目录

- `prd/`：产品需求文档（编号 `001` 起）
- `plans/`：实现计划（编号全局连续）
- `design/`：技术设计
- `engineering/`：工程治理与架构设计
- `acceptance/`：验收证据入口
- `operations/`：运维手册入口
- `openapi.yaml`：OpenAPI 静态产物（契约参考）

## 编号规则

- Plan 编号独立连续：`docs/plans/N-能力名称实现计划.md`
- PRD 使用 `docs/prd/NNN-能力名称.md`
- Plan `1-12` 对应核心闭环能力；`13+` 可承接 Design 拆出的上线接通计划
- PRD 与 Plan 的关联以各自 frontmatter `inputs` / `downstream` 为准，不强制数量一一对应

## Agent 与 OpenSpec

- Claude 角色：`.claude/agents/`
- Claude 技能：`.claude/skills/`
- SDD 规范层：`openspec/specs/`（已接受）、`openspec/changes/`（进行中）

## 模板

正式长期文档使用 `docs/TEMPLATE.md` 作为 frontmatter 与章节模板。
