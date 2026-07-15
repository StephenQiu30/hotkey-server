---
layer: Acceptance
doc_no: "000"
audience: [Dev, QA, Ops]
feature_area: 文档治理
purpose: 定义长期验收证据的结构、结论和归档边界
canonical_path: docs/acceptance/README.md
status: review
version: v1.0
owner: HotKey Server Team
inputs:
  - docs/README.md
  - docs/plans/README.md
outputs:
  - Acceptance 编写规范
triggers:
  - Plan 完成实施并准备验收
  - 回归测试改变既有验收结论
downstream:
  - docs/operations/README.md
---

# 验收文档规范

Acceptance 保存可长期复核的完成证据，不保存完整终端流水或临时调试记录。

## 必需内容

1. 被验收 PRD、Plan、Design 和准确提交 SHA
2. 验收环境、依赖版本和数据 Fixture
3. 红灯命令、失败信号和对应验收项
4. 绿灯命令、结果摘要和证据路径
5. Schema、OpenAPI、运行时或浏览器证据
6. 未执行项目、原因和残余风险
7. 最终结论：accepted、rejected 或 accepted_with_risk

## 命名

使用 `序号-主题-验收.md`。编号必须与被验收 PRD 和 Plan 一致。

## 收录边界

- 可收录稳定测试结果、回归基线、截图索引、性能基线和故障恢复结论
- 不收录重复日志、临时命令输出、会议讨论或无法关联提交的结果
- PR 中的短期 CI 结果只有形成长期质量门禁时才进入本目录
