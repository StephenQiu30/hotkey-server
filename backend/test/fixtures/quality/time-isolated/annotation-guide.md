# 032 跨模块质量标注指南 v1

本数据集只含确定性生成的合成文本、数值特征、受控关系枚举和时间戳，不含真实用户正文、URL、账号、Prompt、凭据或来源载荷。它用于回放生产中的 ContentFamily、MicroEvent、TextQuoteSelector 与 EventHeat 领域算法，不用于判断任何真实报道的真假或来源等级。

## 隔离规则

- `2026-01-01T00:00:00Z` 是冻结时间边界，所有评测样本位于边界之后；训练/调参不读取本目录。
- 样本 ID 在全部模块间唯一；转载家族与微事件隔离清单分别形成独立 SHA-256。
- 语言切片覆盖中文、英文、跨语言和相反表述；来源类型覆盖 feed、platform、search 与 discussion；事件规模覆盖 small/large。
- 两名标注人按本指南独立复核，分歧由第三次规则复核解决；本 fixture 的一致性事实写入数据集元数据。

## 标签口径

- `duplicate=true` 只表示 exact copy 或保守近重复。相同主题但不同标识符、动作或时间的样本标为 false；hard negative 必须禁止自动合并。
- `same_event=true` 表示两个内容家族指向同一微事件；实体、动作、地点、标识符或时间硬冲突标为 false。
- evidence locator 必须在 NFC plaintext 的 UTF-8 字节边界上完全复现 exact/prefix/suffix 和 plaintext SHA-256；Citation 出处字段完整率单独计数。
- evidence relation 只使用 `asserts/attributes_to/mentions/contradicts/corrects/withdraws/unknown`，指标只代表该版本离线输出与标注的一致程度。
- hotspot 标签只评价 Heat v2 是否越过受控阈值以及发现延迟，不代表事实可信度。

## 上线门禁

- 内容家族：Precision ≥ 99.5%、Recall ≥ 90%、错误合并率 < 0.5%。
- 微事件：Pairwise Precision ≥ 95%、B-Cubed F1 ≥ 90%。
- 证据定位：准确率 ≥ 98%、可引用出处字段完整率 100%。
- 热点发现：Precision ≥ 80%、中位发现延迟 < 5 分钟。
- 分类型自动 profile 至少 200 正例、200 负例；每个承诺切片至少 50 个样本。任何门禁未通过时报告必须输出 `automatic_decision_allowed=false`。
