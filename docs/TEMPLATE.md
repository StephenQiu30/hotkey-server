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
purpose: "定义 Go 重构后 HotKey 文档标准，保证 PRD、Plan、Design、Acceptance、Operations 可追踪。"
canonical_path: docs/TEMPLATE.md
status: active
version: "3.0.0"
owner: "StephenQiu30"
inputs:
  - AGENTS.md
outputs:
  - docs 文档格式规范
triggers:
  - 新增长期文档
  - 调整文档目录
  - 调整编号体系
downstream:
  - docs/README.md
---

# HotKey 文档模板

## 1. Frontmatter

正式长期文档必须包含：

```yaml
layer: PRD | Plan | Design | Acceptance | Operations | guide
doc_no: "1"
audience:
  - PM
  - Dev
  - QA
feature_area: "area-name"
purpose: "一句话说明本稿解决什么问题"
canonical_path: "docs/path/file.md"
status: draft | approved | archived
version: "1.0.0"
owner: "StephenQiu30"
inputs:
  - "上游文档"
outputs:
  - "本稿产物"
triggers:
  - "何时更新"
downstream:
  - "下游文档"
```

## 2. PRD 必含章节

- 背景
- 目标
- 范围
- 非目标
- 用户故事
- 数据与 API 边界
- 验收标准
- 风险与降级
- 变更记录

## 3. Plan 必含章节

- 目标
- 文件清单
- 任务拆解
- TDD 与验证
- 执行顺序
- 回滚策略
- 验收命令
- 变更记录

## 4. 质量门禁

- 禁止占位标记和未定稿提示。
- PRD 与 Plan 编号必须一致。
- 文件名必须从 `1` 开始的新体系派生，不沿用旧 FastAPI 编号。
- 过程记录、一次性检查、临时任务清单和中间状态文件不进入 `docs/`。
- 只有最终有长期价值的事实源文档可以进入 `docs/`。
- 任务 issue 必须绑定所属 Epic 对应的里程碑。
