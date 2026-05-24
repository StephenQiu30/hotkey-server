---
layer: acceptance
doc_no: "README"
audience:
  - PM
  - Dev
  - QA
  - Ops
purpose: "说明企业级 P0 验收证据目录的存放范围与命名约定。"
owner: "StephenQiu30"
inputs:
  - "docs/plans/23-企业级P0任务一次性拆分与排程.md"
  - "docs/plans/24-企业级P0任务一次性编排清单.md"
outputs:
  - "企业级 P0 验收证据索引"
triggers:
  - "Issue 关闭前需要可复核证据"
downstream:
  - "docs/enterprise-p0-backlog-gap-review.md"
  - "docs/验收差距清单.md"
---

# Acceptance Evidence

本目录存放企业级 P0 的长期验收证据、运行结论、演练记录和运维 SOP。

当前证据包：

- `B1-回放证据包.md`
- `B2-健康可达性问题清单.md`
- `B3-告警演练结论.md`
- `C1-运维手册补充.md`
- `C2-阈值与变更SOP.md`
