---
layer: Acceptance
scope: shared
doc_no: "000"
title: Acceptance 索引
status: active
version: v1.0
owner: HotKey Team
canonical_path: docs/acceptance/README.md
---

# Acceptance 索引

重新立项基线目前没有整体 `passed` 的 Acceptance。001 已建立 `failed` 的部分验收，只登记证据完整的局部门禁并保留整体未完成结论；旧验收文件属于被清理的历史基线，不得用来证明 001–005 的新需求已经实现。

实现完成后按 `NNN-中文主题验收.md` 新增同编号文件，并至少记录：

- Git revision、构建版本、环境和验证日期；
- 每条 `AC-NNN-*` 的通过/失败及对应 `EV-NNN-*`；
- 实际执行的自动化命令和结果摘要；
- 人工、性能、安全、故障注入和恢复证据；
- Schema、OpenAPI、配置和部署影响；
- 已知限制、偏差批准和回滚验证；
- 验收人与结论。

| 编号 | 交付域 | 计划状态 | Acceptance |
|---:|---|---|---|
| 001 | 产品需求分析与总体架构 | in_progress | [部分验收](001-HotKey产品需求分析与总体架构验收.md)（`failed`） |
| 002 | 监控来源采集与证据链 | in_progress | [部分验收](002-监控来源采集与证据链验收.md)（`failed`） |
| 003 | 智能研判事件热度与人工治理 | in_progress | 未创建 |
| 004 | 通知报告知识投影与检索 | in_progress | 未创建 |
| 005 | 安全运维质量与交付 | in_progress | 未创建 |
