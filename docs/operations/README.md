---
layer: Operations
doc_no: "000"
audience: [Dev, QA, Ops]
feature_area: 项目运行与发布
purpose: 定义 HotKey Server 发布、运行、回滚和故障手册的归档规则
canonical_path: docs/operations/README.md
status: review
version: v2.4
owner: HotKey Server Team
inputs:
  - docs/README.md
outputs:
  - Operations 编写规范
triggers:
  - 新增发布、部署、回滚或运行流程
  - 运行方式或依赖恢复流程变化
downstream: []
---

# 运维文档规范

Operations 保存可重复执行的协作和运行流程，包括 Git/PR、发布、部署、备份、恢复、回滚、告警和故障处置。

## 必需内容

- 适用范围、前置权限和依赖
- 可复制命令与预期信号
- 失败判断、停止条件和回滚步骤
- 数据、密钥、日志和审计边界
- 验证方式和最后演练日期

## 收录边界

- 不放产品需求、架构设计或测试报告主体
- 不记录单次发布流水；只记录可重复流程
- 运行手册不得包含真实密钥、Token 或个人环境绝对路径
- 当前尚未设计部署拓扑，因此只建立规范入口，不虚构部署手册

## 当前手册

| 文档 | 说明 | 状态 | 最近演练 |
|---|---|---|---|
| [CI 质量门禁](001-本地与GitHub-CI质量门禁.md) | `make ci` 的本地复现、GitHub Actions 流程及测试依赖边界 | accepted | 2026-07-17（PLAN-007 受控验收） |
| [PLAN-007 内容标准化与 MinIO 证据升级](002-内容标准化去重与MinIO证据升级.md) | PLAN-006 既有数据的备份、回填、legacy-zero、验证与回退 | accepted | 2026-07-17（[Acceptance-007](../acceptance/archive/007-内容标准化去重与MinIO证据验收.md) accepted） |
| [PLAN-008 AI Provider 与 Embedding 升级](003-AIProvider与Embedding升级.md) | 空 AI 历史库的备份、严格 preflight、add-only 升级、验证与历史 release 回退 | accepted | 2026-07-17（[Acceptance-008](../acceptance/archive/008-AIProvider与Embedding基础验收.md) accepted） |
| [PLAN-009 相关性审核升级](004-多语言相关性匹配与反馈升级.md) | relevance_review、相关性快照/反馈/建议及有界召回索引、固定 PLAN-008 历史库演练与回退 | accepted | `d4efda5` 完整门禁与独立复审已通过 |
| [PLAN-010 事件治理升级](005-事件聚类生命周期与治理升级.md) | Event 决策/治理审计表、当前成员归属唯一索引及固定 PLAN-009 历史库演练与回退 | review | 实施前模板，尚未演练 |
| [系统收口与恢复](006-运行治理容量与恢复操作.md) | 运行总览、容量游标基线、清理、对账和备份恢复操作 | accepted | 2026-07-17（PLAN-017） |

## 文件命名

- Operations 正式文档统一使用 `序号-主题.md`，序号必须与 frontmatter 的 `doc_no` 一致。
- 序号表示 Operations 目录自身的文档顺序，不直接复用关联 PRD/Plan 编号；关联的 PLAN 编号写在标题、正文和索引说明中。
- 主题必须同时表达操作对象和动作，例如“内容标准化去重与 MinIO 证据升级”，禁止使用 `schema-upgrade.md`、`runbook.md` 或仅有序号的文件名。

## test/tools 目录

`test/tools/` 只保留被 Makefile、测试或 CI 调用的架构、仓库、Schema 和数据库门禁工具；统一测试套件的执行由 `test/runner` 管理。PLAN-007 专用的 MinIO 手工 fixture 仅服务历史验收、没有自动调用方，已移除。

## 变更记录

| 版本 | 日期 | 变更 |
|---|---|---|
| v1.2 | 2026-07-17 | 记录 PLAN-007 受控升级/回退与 CI 的最近演练；独立最终复核已通过。 |
| v1.3 | 2026-07-17 | 增加待实施的 PLAN-008 Schema 升级/回退手册，不表示 AI 功能已交付。 |
| v1.4 | 2026-07-17 | 记录 PLAN-008 的固定历史 worktree 升级/回退演练及 Acceptance-008 accepted 结论。 |
| v1.5 | 2026-07-17 | 新增 PLAN-009 Task 1 的 relevance_review Profile 升级/回退手册；验收前保持 review。 |
| v1.6 | 2026-07-17 | 扩展为 PLAN-009 Task 1–2 的完整 Schema 升级/回退演练，覆盖相关性快照、反馈与建议。 |
| v1.7 | 2026-07-17 | 扩展 PLAN-009 手册为 Task 1–3，纳入受限 source/lexical 召回索引和历史升级/回退验证。 |
| v1.8 | 2026-07-17 | 同步 PLAN-009 Task 5 的相关性 API、反馈与建议审核验证状态；Schema 手册本身仍只覆盖升级/回退，不替代最终验收。 |
| v1.9 | 2026-07-17 | 同步 PLAN-009 Task 6 的完整 fixture 自动门禁已通过、独立最终复核仍待完成的真实状态。 |
| v2.0 | 2026-07-17 | 同步 PLAN-009 独立复核的 P1 整改已通过定向验证；完整门禁和复审仍待完成。 |
| v2.1 | 2026-07-17 | 同步 `d4efda5` relevance-review 序列化修复及完整 CI/MinIO integration race 通过；独立复审仍待完成。 |
| v2.2 | 2026-07-17 | 创建 PLAN-010 决策/治理审计 Schema 升级与回退手册；仅为实施前流程，尚未演练。 |
| v2.3 | 2026-07-17 | 首次按主题重命名操作手册并同步引用。 |
| v2.4 | 2026-07-17 | 按仓库文档标准改为序号加中文主题命名，修正 `doc_no` 与文件序号，并同步全部引用。 |
