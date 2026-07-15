---
layer: PRD
prd_no: "008"
doc_no: "008"
title: AI Provider与Embedding基础
audience: [PM, Dev, QA, Ops]
feature_area: AI运行基础
purpose: 定义 AI Provider、模型配置、运行记录与 Embedding 基础
phase: P0
priority: P0
status: review
execution_status: backlog
version: v1.0
owner: HotKey Server Team
depends_on: [PRD-002, PRD-007]
design_refs:
  - docs/design/003-数据库与数据生命周期设计.md
  - docs/design/007-多语言匹配与相关性设计.md
  - docs/design/011-AI任务证据与模型运行设计.md
canonical_path: docs/prd/008-AIProvider与Embedding基础.md
inputs:
  - docs/design/003-数据库与数据生命周期设计.md
  - docs/design/007-多语言匹配与相关性设计.md
  - docs/design/011-AI任务证据与模型运行设计.md
outputs:
  - AI Provider 与 Embedding 基础需求
triggers:
  - Provider、模型、向量或预算契约变化
downstream:
  - docs/plans/008-AIProvider与Embedding基础计划.md
  - docs/acceptance/008-AIProvider与Embedding基础验收.md
---

# AI Provider 与 Embedding 基础

## 目标

建立可替换、可限流、可审计的 AI Provider 与 1024 维 Embedding 基础，为匹配和后续事件智能提供统一运行能力。

## 范围

- 实现 intelligence 模块的模型配置、Provider 端口、运行包和基础设施适配器。
- 实现 ai_model_profiles、ai_runs、内容/Monitor/Event/Topic Embedding 的持久化契约。
- 支持官方 LLM SDK 和可选 ONNX 本地多语言 Embedding，第三方类型不得穿透模块边界。
- 建立静态版本化 JSON Schema 注册、输入哈希、reuse_key、预算、超时、重试和回退选择。
- 首批任务覆盖 embedding 与 term_expansion；事件摘要和主张留给 PRD-012。
- 提供无可用模型时的显式降级结果。

## 非范围

- 不让 LLM 负责采集、基础去重、热度或唯一事实判定。
- 不实现在线训练、自动 Prompt 自优化或独立向量数据库。
- 不在本任务写入 Event 摘要、Claim 或 Vault。

## 功能要求

1. 每个 Embedding 保存模型配置、模型版本、输入哈希和 active 标记。
2. 不同模型版本和向量空间不可混合检索。
3. 相同 reuse_key 的成功运行只复用一次；输入、Schema、Prompt 或模型变化产生新运行。
4. Provider 超时、429、5xx、非法 JSON 和部分结果有稳定状态与重试分类。
5. 凭据只通过配置引用进入基础设施，不写日志或 API。
6. 日预算和单次预算超限时降级，不阻塞非 AI 主链路。
7. 输出必须先通过 JSON Schema，最多执行一次结构化修复。

## 交付物

- AI Provider、Embedding、模型选择和运行记录实现。
- 版本化 Schema 资源、模型配置管理 API 和安全凭据边界。
- pgvector Repository、HNSW 查询和模型空间隔离测试。
- Provider 契约测试、预算/限流测试和 ONNX 可选适配验证。

## 验收标准

- 成功、超时、429、5xx、非法 JSON 和修复失败均可追踪。
- 相同输入不重复调用，模型版本切换后不复用旧向量。
- 无 LLM Provider 时 embedding 可按配置降级，采集与内容查询仍可运行。
- halfvec(1024) 写入、搜索和失效符合完整 Schema。
- 运行记录不保存不必要的完整敏感正文或 Provider 原始响应。

## 完成定义

PRD-009 可通过稳定端口获取多语言向量和扩展词，不依赖具体模型厂商。
