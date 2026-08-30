# Agent Shadow Golden v1 标注说明

本目录用于 `TASK-003-S05-T02` 的固定双轨输入。`golden-dataset.json` 当前只含合成数据，覆盖 5 个获批 P0 结构化契约，不含生产来源正文、用户数据、Prompt、凭据或真实模型输出。

## 当前状态

- `annotation_protocol_version=pending-dual-review-v1`、`annotator_count=0` 表示尚未完成人工复核；这不是可批准的质量基线。
- 普通 CI 只验证数据集、并发执行、Schema/Evidence 约束、指标计算、错误脱敏和 `candidate` 状态。
- 评估报告永远返回 `approval_ready=false`，不能激活 Agent、切换 Model Profile 或勾选 `CHK-003-G5-001`。

## 受信复核流程

1. 在固定数据集与固定模型版本上运行旧 Provider/Codex 和 Python Agent。默认命令只保存脱敏报告；需要人工复核时，额外传入绝对路径 `--review-bundle /absolute/private/path/review.json`，在同一次执行中生成本机私有复核包。
2. 复核包只包含已通过完整业务 Schema 与 Evidence Policy 的候选输入/输出，不包含 Prompt、凭据或 Provider 错误；创建权限固定为 `0600`，拒绝相对路径和覆盖已有文件。不得把复核包提交到仓库、CI Artifact、日志或 `docs/acceptance/evidence`。
3. `input_raw_json` 与 `output_raw_json` 是精确原始 UTF-8 JSON 字节的字符串表示；`output_sha256` 必须按解码后的 `output_raw_json` 字节复算。先核对报告与复核包的 `dataset_sha256` 相同，再分发给受信复核者。
4. 每条输出由至少 2 名独立复核者按同一协议判断是否可接受；复核者分别记录轨道、输出 SHA-256、接受结论与理由，在提交前不得查看另一位复核者的标签。存在分歧时增加独立复核或进入书面裁决，不得由执行命令自动决定。
5. Evidence 标签必须来自输入允许的 `content_id+locator` 或原文中的 `exact_quote`；不允许补写输入外证据。复核汇总只把轨道、输出 SHA-256、最终接受结论和实际复核人数写入新版本数据集，不把原始模型输出复制进脱敏报告。
6. 私有复核包按批准的数据保留策略处置；工具不会自动删除它。只有结构有效率、Evidence Precision/Recall、人工接受率、P50/P95/最大延迟、成本和失败分类都有真实数据，且阈值另行批准后，才可形成 G5 审批材料。

后续真实数据集必须另起版本，保留旧文件与 Hash；不得静默改写本版本后继续沿用原报告。
