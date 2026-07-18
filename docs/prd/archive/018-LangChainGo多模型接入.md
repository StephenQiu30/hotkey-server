---
layer: PRD
prd_no: "018"
doc_no: "018"
title: LangChainGo多模型接入
audience: [PM, Dev, QA, Ops]
feature_area: AI运行基础
purpose: 交付 DeepSeek、Ollama 与 Qwen Embedding 的可配置、可验证 Provider 接入
phase: P1
priority: P0
status: archived
execution_status: done
version: v1.4
owner: HotKey Server Team
depends_on: [PRD-008, PRD-017]
design_refs:
  - docs/design/archive/015-LangChainGo多Provider与本地模型设计.md
canonical_path: docs/prd/archive/018-LangChainGo多模型接入.md
inputs:
  - docs/design/archive/015-LangChainGo多Provider与本地模型设计.md
  - docs/prd/archive/008-AIProvider与Embedding基础.md
outputs:
  - DeepSeek 与 Ollama LangChainGo Provider
  - Qwen3 1024 维 Embedding profile 能力
  - 配置、Schema、OpenAPI、升级和连接验证
triggers:
  - Design-015 accepted
downstream:
  - docs/plans/archive/018-LangChainGo多模型接入计划.md
  - docs/acceptance/archive/018-LangChainGo多模型接入验收.md
---

# LangChainGo 多模型接入

## 目标

在不复制或绕过既有预算、重试、模型档案、复用、审计和 Schema 校验的前提下，让管理员可创建 `deepseek` 与 `ollama` 模型档案，并让 Ollama 的 `qwen3-embedding:0.6b` 进入现有 1024 维 Embedding 链路。

## 范围

1. 增加 `github.com/tmc/langchaingo`，以 OpenAI 兼容 adapter 接 DeepSeek，以原生 Ollama adapter 接本地生成与 Embedding。
2. 增加 `HOTKEY_DEEPSEEK_API_KEY`、`HOTKEY_OLLAMA_ENABLED`、`HOTKEY_OLLAMA_BASE_URL`，并保持 `.env`/`.env.prod`/进程环境优先级；`HOTKEY_DEEPSEEK_API_KEY` 是数据库 `env:DEEPSEEK_API_KEY` 的唯一解析来源。
3. 扩展 Domain、Schema、记录模型输入、管理员 API 与 OpenAPI 的 provider whitelist、能力和凭据约束。
4. Qwen Embedding 固定 `qwen3-embedding:0.6b` 与 1024 维；所有 Ollama profile 的 model version 固定为 `/api/tags` digest 去掉 `sha256:` 后的 64 位小写 hex，每次模型调用前校验名称/digest。错误维度、非有限值、digest 漂移或其他 Qwen3 Embedding tag 安全失败且不调用模型端点。
5. 保留 Application 的预算、最大尝试、退避、一次修复、稳定错误、运行审计和向量写入流程；Provider SDK 不自行重试。
6. 提供既有数据库约束升级/回退手册，以及无密钥 fixture 与可选实机连接探测。

## 非范围

- 不把现有 OpenAI adapter 改为 LangChainGo。
- 不实现 Agent、Chain、Memory、Tool calling、通用 RAG 或动态 Provider 插件。
- 不自动安装 Ollama、不自动 pull 模型、不提交真实密钥。
- 不修改 `halfvec(1024)`，不允许 Qwen3 Embedding 4B/8B 直接进入当前空间。
- 不增加数据库 endpoint/parameters 或任意 Base URL API。

## 功能与安全要求

- DeepSeek profile 只允许非 Embedding 任务，凭据必须是 write-only `env:DEEPSEEK_API_KEY`，且只映射 `config.AI.DeepSeekAPIKey`/`HOTKEY_DEEPSEEK_API_KEY`；配置空时 Provider 不可用。
- Ollama profile 可执行生成和 Embedding，凭据必须为 `NULL`；只有显式启用且 URL 合法时注册。数据库事实源必须拒绝非 Qwen 0.6B 的 Ollama Embedding、非 1024 维及不符合 digest 格式的 Ollama model version，同时允许其他 Ollama 生成模型。
- Ollama URL 不得通过模型档案、HTTP API、日志、指标或审计泄露；DeepSeek 固定官方 endpoint。
- DeepSeek 与 Ollama 结构化结果必须经过现有静态 Schema 校验和至多一次修复。
- 429、5xx、deadline、模型不存在、非法 JSON 和向量非法均映射为既有稳定错误码，第三方原始正文不得出现在 Result 或日志。
- Profile 查询与响应仍不返回 credential reference、endpoint、parameters、Prompt 或 raw response。

## 验收标准

1. DeepSeek fixture 验证 Authorization、固定 Base URL 路径、显式模型、JSON 输出、usage、修复、429、5xx、deadline 与错误脱敏。
2. Ollama fixture 验证 `/api/tags` digest、`/api/chat` 与 `/api/embed`、显式模型、批量顺序、生成 usage、Embedding 零 usage、1024 维 Qwen 输出、1023/1025/NaN/Inf 拒绝、tag 漂移、模型缺失和 deadline；固定 LangChainGo 版本不暴露 Embedding token 数，不得估算。
3. 配置测试证明 `.env`、`.env.prod` 与进程环境覆盖，且无效 Ollama URL 不会触发 fatal、panic 或外呼。
4. Profile Domain、数据库 CHECK、API/OpenAPI enum 与 write-only credential 一致；升级、verify 和拒绝有新 Provider 数据的回退 preflight 可复现。
5. `make ci`、Provider/config/bootstrap/architecture 的 `go test -race` 相关范围、`git diff --check` 与 `make clean` 通过；fixture 请求计数证明 LangChainGo 每次生成只外呼一次、Embedding 每个输入只外呼一次且错误不触发内部重试。
6. 若提供可用 DeepSeek key 与本机 Ollama/Qwen 模型，完成实机最小生成和 1024 维 Embedding 探测；缺失外部条件时明确记录未执行项和风险。

## 完成定义

管理员可安全创建 DeepSeek 生成 profile 和 Ollama 生成/`qwen3-embedding:0.6b` Embedding profile；fixture 和质量门禁证明协议、错误、Schema、配置与数据约束正确，实机状态按实际环境如实记录。

## 变更记录

| 版本 | 日期 | 变更 |
|---|---|---|
| v0.1 | 2026-07-18 | 建立 DeepSeek、Ollama 与 Qwen Embedding 交付范围，等待独立审核。 |
| v0.2 | 2026-07-18 | 按独立审核冻结凭据映射、Ollama digest、数据库约束和零内部重试验收。 |
| v1.0 | 2026-07-18 | 非主要编写者复核范围与验收完整，PRD accepted/ready。 |
| v1.1 | 2026-07-18 | 按数据库 varchar(64) 红灯将 Ollama model version 规范为去前缀 digest，重新审核。 |
| v1.2 | 2026-07-18 | 非主要编写者复核 digest 变更 APPROVED，恢复 accepted/in_progress。 |
| v1.3 | 2026-07-18 | 冻结 Ollama 生成 usage 与 Embedding 零 usage 契约，避免伪造 SDK 未暴露的 token 事实。 |
| v1.4 | 2026-07-18 | 实施、全量门禁与独立最终复审完成，execution done 并归档。 |
