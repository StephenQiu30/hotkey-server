---
layer: guide
doc_no: "TEMPLATE"
audience:
  - PM
  - Tech-Lead
  - Dev
  - QA
  - Ops
feature_area: documentation
purpose: "定义 HotKey 文档标准格式，保证 PRD/Plan/验收/运维文档可追溯、可交付。"
canonical_path: docs/TEMPLATE.md
status: active
version: "2.0.0"
owner: "StephenQiu30"
inputs:
  - AGENTS.md
  - AGENTS.local.md
downstream:
  - docs/README.md
  - 各类 PRD 与 Plan 文档
---

# 文档模板（HotKey v2.0）

## 0. 适用范围

本模板用于本仓库 `docs/` 下的正式长期文档（PRD、Plan、Design、Acceptance、Operations）。  
临时记录请写到 OpenSpec 的 `tasks`，不可作为 `docs/` 长期文档。

## 1. 文档头部（YAML）

所有正式文档必须保留以下字段：

```yaml
layer: PRD | Plan | Design | Acceptance | Operations | guide
doc_no: "XX"
audience:
  - PM
  - Dev
  - QA
  - Ops
feature_area: "领域简称"
purpose: "一句话说明本篇解决什么问题"
canonical_path: "docs/xxx/..."
status: draft | approved | archived
version: "1.0.0"
owner: "StevehnQiu30"
inputs:
  - "上游文档路径"
outputs:
  - "本稿会产出的交付物"
triggers:
  - "什么条件下必须更新本稿"
downstream:
  - "下游文档路径"
```

## 2. 必含章节（按文档类型）

- PRD
  - 背景、目标（SMART）、非目标、输入、功能边界、数据字段、验收、风险、变更记录。
- Plan
  - 目标、文件清单、任务拆解、TDD验收清单、依赖关系、顺序、回滚点、变更记录。
- Acceptance
  - Given/When/Then 场景、执行脚本、证据、失败回退、残余风险。
- Operations
  - 运行约束、发布、回滚、监控、交接规则。

## 3. 文档质量门禁

- 禁止中间产物（TBD、TODO、待补充占位）入库。
- 标题和文件名必须可读，可追溯地表达领域与序号。
- 测试标准与验收项必须可执行（测试名、预期字段、日志项可核验）。
- 每份文档须包含变更记录。

## 4. 与本仓库协作约束

- PRD 先于 Plan。
- Plan 与 Issue 一一映射，且每个 issue 包含 Given/When/Then、回退与回滚点。
- 仅在计划和验收通过后创建 PR，PR 模板必须包含 tests/commands/result 说明。

## 5. 示例结构

```text
docs/
  product/
    prd/
  plans/
    28-...
  acceptance/
  engineering/
  archive/
```

## 6. 更新记录

| 日期 | 作者 | 版本 | 变更说明 |
| --- | --- | --- | --- |
| 2026-05-24 | StephenQiu30 | 2.0.0 | 替换为执行用模板，明确测试与交付门禁 |
