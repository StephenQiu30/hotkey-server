---
layer: Plan
doc_no: "01"
audience:
  - Tech-Lead
  - Dev
  - QA
feature_area: "area:governance"
purpose: "把项目治理与 Symphony 编排 PRD 拆成可由 Linear 和 Symphony 执行的工程任务。"
canonical_path: "docs/plans/01-项目治理与Symphony编排实现计划.md"
status: draft
version: "1.0.0"
owner: "StephenQiu30"
inputs:
  - "docs/product/prd/01-项目治理与Symphony编排PRD.md"
outputs:
  - "项目治理与 Symphony 编排实现任务"
triggers:
  - "PRD 或 Symphony 工作流规范变化"
downstream:
  - "Linear project: HotKey Server AI 热点日报"
---

# 01-项目治理与Symphony编排实现计划

## 1. 目标

建立 HotKey Server 的标准执行闭环：PRD -> Plan -> Linear issue -> Symphony harness -> TDD -> 验证 -> 提交 -> Linear 回写。

## 2. 文件清单

- 修改：`WORKFLOW.md`
- 修改：`tests/test_workflow_contract.py`
- 创建：`docs/plans/`
- 创建：`docs/acceptance/01-项目治理与Symphony编排验收.md`

## 3. 任务拆解

1. 编写 workflow contract 测试，校验 `WORKFLOW.md` 包含 tracker、workspace、hooks、agent、codex 和 issue prompt。
2. 扩展 `WORKFLOW.md` prompt，要求 agent 读取 PRD/Plan、执行 TDD、回写 Linear。
3. 增加文档治理测试，校验每个 PRD 都有对应 Plan。
4. 编写验收文档，记录 Linear project、issue 创建结果和 Symphony 监听要求。

## 4. TDD 与验证

- 先写 Python governance 测试验证 PRD/Plan 一一对应。
- 再修改 `WORKFLOW.md` 和文档。
- 验证命令：`make test`。

## 5. 执行顺序

1. `test:` 添加失败的文档治理测试。
2. `impl:` 补齐 workflow prompt 和文档结构。
3. `docs:` 写验收文档。

## 6. 回滚策略

回滚本 PRD 只需撤销 `WORKFLOW.md` prompt 变更和新增 governance 测试，不影响业务代码。

## 7. 验收命令

```bash
make test
python3 -m unittest discover -s tests
```

## 8. Symphony / Linear 要求

任务状态、标签和流转规则完全以本仓库 `WORKFLOW.md` 和本地 Symphony 实现为准。Plan 不定义额外状态、不发明额外标签、不覆盖 Symphony 的状态机。

Linear issue 只承载任务内容：PRD 路径、Plan 路径、任务范围、禁止范围、TDD 验收命令和回写要求。Symphony 负责监听 active states、创建 workspace、运行 Codex，并按 `WORKFLOW.md` prompt 驱动执行。

## 9. 变更记录

| 日期 | 作者 | 版本 | 变更说明 |
| --- | --- | --- | --- |
| 2026-05-31 | StephenQiu30 | 1.0.0 | 初版 |
