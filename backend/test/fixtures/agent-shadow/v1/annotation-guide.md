# Agent Shadow Golden v1 标注说明

本目录用于 `TASK-003-S05-T02` 的固定双轨输入。`golden-dataset.json` 当前只含合成数据，覆盖 5 个获批 P0 结构化契约，不含生产来源正文、用户数据、Prompt、凭据或真实模型输出。

## 当前状态

- `annotation_protocol_version=pending-dual-review-v1`、`annotator_count=0` 表示尚未完成人工复核；这不是可批准的质量基线。
- 普通 CI 只验证数据集、并发执行、Schema/Evidence 约束、指标计算、错误脱敏和 `candidate` 状态。
- 评估报告永远返回 `approval_ready=false`，不能激活 Agent、切换 Model Profile 或勾选 `CHK-003-G5-001`。

## 受信复核流程

1. 在固定数据集与固定模型版本上分别运行旧 Provider/Codex 和 Python Agent，保存报告及其 `dataset_sha256`。
2. 每条输出由至少 2 名独立复核者按同一协议判断是否可接受；不得在复核前查看另一条轨道的标签。
3. 复核记录只写轨道、输出 SHA-256、接受结论和复核人数，不把原始模型输出复制进报告。
4. Evidence 标签必须来自输入允许的 `content_id+locator` 或原文中的 `exact_quote`；不允许补写输入外证据。
5. 只有结构有效率、Evidence Precision/Recall、人工接受率、P50/P95/最大延迟、成本和失败分类都有真实数据，且阈值另行批准后，才可形成 G5 审批材料。

后续真实数据集必须另起版本，保留旧文件与 Hash；不得静默改写本版本后继续沿用原报告。
