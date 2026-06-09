---
layer: Plan
doc_no: "18"
audience:
  - Tech-Lead
  - Dev
  - QA
feature_area: "area:hotspot"
purpose: "让新增内容可以通过 embedding 与聚类形成真实可用的热点簇结果。"
canonical_path: "docs/plans/18-Embedding热点聚类管线接通实现计划.md"
status: draft
version: "1.0.0"
owner: "StephenQiu30"
inputs:
  - "docs/design/002-热点平台能力补齐与上线接通设计.md"
  - "docs/plans/17-内容标准化去重管线接通实现计划.md"
  - "docs/product/prd/07-Embedding与热点聚合PRD.md"
  - "docs/plans/07-Embedding与热点聚合实现计划.md"
outputs:
  - "Embedding 热点聚类管线接通实现任务"
triggers:
  - "embedding 或聚类策略变化"
downstream:
  - "docs/plans/19-热点评分与榜单API主线接通实现计划.md"
---

# 18-Embedding热点聚类管线接通实现计划

## 1. 目标

让新增内容可以通过 embedding 与聚类形成真实可用的热点簇结果。

## 2. 文件清单

- 修改：`internal/service/embedding/`
- 修改：`internal/service/hotspot/`
- 修改：`internal/worker/`

## 3. 任务拆解

1. 为 embedding 失败、未配置和成功聚类写失败测试。
2. 串联 embedding 任务与聚类任务。
3. 写入 embedding 状态、聚类结果和失败原因。
4. 确认结果可供评分和报告消费。
5. 运行验证并回写结果。

## 4. TDD 与验证

- embedding 和聚类已进入真实主链路。
- 失败和降级状态可追踪。
- 热点簇结果可以作为后续评分输入。

## 5. 执行顺序

1. `test:` embedding 失败、未配置、成功聚类测试。
2. `impl:` 任务串联与状态落库。
3. `refactor:` 统一 embedding 与聚类 job 类型。

## 6. 回滚策略

回滚 worker 任务串联，保留 embedding 与 hotspot service 独立能力；已生成 embedding 与簇数据保留。

## 7. 验收命令

```bash
go test ./internal/service/embedding/...
go test ./internal/service/hotspot/...
gofmt -w cmd internal
go test ./...
python3 -m unittest discover -s tests
```

## 8. Symphony / Linear 要求

任务状态、标签和流转规则完全以本仓库 `WORKFLOW.md` 和本地 Symphony 实现为准。Plan 不定义额外状态、不发明额外标签、不覆盖 Symphony 的状态机。

Linear issue 只承载任务内容：PRD 路径、Plan 路径、任务范围、禁止范围、TDD 验收命令和回写要求。Symphony 负责监听 active states、创建 workspace、运行 Codex，并按 `WORKFLOW.md` prompt 驱动执行。

## 9. 变更记录

| 日期 | 作者 | 版本 | 变更说明 |
| --- | --- | --- | --- |
| 2026-06-09 | StephenQiu30 | 1.0.0 | 按文档规范重写，编号调整为 18 |
