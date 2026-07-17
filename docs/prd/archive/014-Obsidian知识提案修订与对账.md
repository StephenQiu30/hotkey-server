---
layer: PRD
prd_no: "014"
doc_no: "014"
title: Obsidian知识提案修订与对账
audience: [PM, Dev, QA, Ops]
feature_area: 知识库治理
purpose: 定义 Obsidian 提案、审核、修订、冲突与对账任务
phase: P1
priority: P1
status: archived
execution_status: done
version: v1.0
owner: HotKey Server Team
depends_on: [PRD-010, PRD-012, PRD-013]
design_refs:
  - docs/design/archive/003-数据库与数据生命周期设计.md
  - docs/design/008-Obsidian知识库治理与报告交付设计.md
  - docs/design/archive/011-AI任务证据与模型运行设计.md
canonical_path: docs/prd/archive/014-Obsidian知识提案修订与对账.md
inputs:
  - docs/design/archive/003-数据库与数据生命周期设计.md
  - docs/design/008-Obsidian知识库治理与报告交付设计.md
  - docs/design/archive/011-AI任务证据与模型运行设计.md
outputs:
  - Obsidian 知识治理需求
triggers:
  - Vault、提案、修订、冲突或对账规则变化
downstream:
  - docs/plans/archive/014-Obsidian知识提案修订与对账计划.md
  - docs/acceptance/archive/014-Obsidian知识提案修订与对账验收.md
---

# Obsidian 知识提案、修订与对账

## 目标

把 Event 和长期 Topic 投影到本地 Obsidian Vault，同时通过提案、审核、修订和冲突机制保护人工内容。

## 范围

- 实现 knowledge 模块、VaultStore 端口和本地文件系统适配器。
- 实现 knowledge_documents、change_proposals、annotations、revisions、vault_sync_runs。
- 生成稳定目录、文件名、Frontmatter、自动区域和人工区域。
- 实现 knowledge_proposal AI 任务，但只创建提案，不直接写 Vault。
- 支持审核、拒绝、冲突、应用、归档和恢复为新修订。
- 实现路径锁、内容哈希、临时文件、flush、原子重命名和 MinIO 修订快照。
- 实现数据库、MinIO 和 Vault 的双向扫描与对账。

## 非范围

- 不依赖 Git 提交、分支或远程仓库完成知识流程。
- 不静默覆盖人工备注、未知 Frontmatter 或外部编辑。
- 不把 Vault 作为业务事实源。

## 功能要求

1. 事件与主题文档具有稳定路径和 Frontmatter。
2. 提案携带 base_revision、base_hash、结构化 Frontmatter 和正文差异。
3. 应用前重新校验数据库版本和当前文件哈希；不一致进入 conflict。
4. 自动区域可更新，人工区域默认保留。
5. 每次成功修改创建可恢复修订和 MinIO 快照。
6. 路径必须限制在配置 Vault 根目录，拒绝遍历和符号链接逃逸。
7. 崩溃残留临时文件可识别、清理或恢复，不形成半文件。

## 交付物

- Knowledge 领域、提案审核、Vault 写入、修订和对账用例。
- 事件/主题模板、Frontmatter Schema 和管理员 API。
- Schema、记录模型、OpenAPI、P1 River Job 和指标。
- 路径逃逸、并发写、人工编辑、崩溃、恢复和跨存储故障测试。

## 验收标准

- 事件笔记写入固定目录且 Frontmatter 可稳定查询。
- 人工修改后旧提案不能直接应用。
- 自动更新不覆盖人工备注。
- 每次修改可从数据库与 MinIO 追溯并恢复。
- 无 Git 环境下全部知识流程可运行。
- 对账能识别孤儿对象、丢失文件、哈希冲突和过期投影。

## 完成定义

PRD-015 可引用已发布知识投影，但报告事实仍以冻结 Event 快照为准。
