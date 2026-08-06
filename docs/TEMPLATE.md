# 文档模板

## Design

```yaml
---
layer: Design
scope: shared
doc_no: "NNN"
title: 主题
status: proposed
version: v1.0
owner: HotKey Team
canonical_path: docs/design/NNN-主题设计.md
prd: docs/prd/NNN-主题.md
plan: docs/plans/NNN-主题计划.md
---
```

正文至少包含：现状、目标、非目标、核心决策、数据与状态、接口与交互、安全与合规、失败与降级、验收边界。

## PRD

```yaml
---
layer: PRD
scope: shared
doc_no: "NNN"
title: 主题
status: draft
version: v1.0
owner: HotKey Team
canonical_path: docs/prd/NNN-主题.md
design: docs/design/NNN-主题设计.md
plan: docs/plans/NNN-主题计划.md
---
```

正文至少包含：用户问题、目标、范围、用户故事、功能要求、非功能要求、非目标、验收标准。

## Plan

```yaml
---
layer: Plan
scope: shared
doc_no: "NNN"
title: 主题计划
status: planned
version: v1.0
owner: HotKey Team
canonical_path: docs/plans/NNN-主题计划.md
design: docs/design/NNN-主题设计.md
prd: docs/prd/NNN-主题.md
---
```

正文至少包含：前置门禁、变更文件、测试先行步骤、实现步骤、数据与契约、验证命令、迁移与回滚、完成定义。
