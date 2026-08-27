---
layer: PRD
scope: shared
doc_no: "000"
title: PRD 索引
status: draft
version: v1.0
owner: HotKey Team
canonical_path: docs/prd/README.md
---

# PRD 索引

本目录定义 HotKey 重新立项后的稳定产品范围。五份 PRD 当前均为 `draft`，只表达待评审目标，不证明代码已经实现；实施规格与执行检查清单位于同编号 Plan，完成事实只能由 Acceptance 证明。

## 交付项

| 编号 | 产品范围 | Design | PRD | Plan | 状态 |
|---:|---|---|---|---|---|
| 001 | HotKey 产品需求分析与总体架构 | [Design](../design/001-HotKey产品需求分析与总体架构设计.md) | [PRD](001-HotKey产品需求分析与总体架构.md) | [Plan](../plans/001-HotKey产品需求分析与总体架构计划.md) | `draft` |
| 002 | Monitor、授权来源、采集与证据链 | [Design](../design/002-监控来源采集与证据链设计.md) | [PRD](002-监控来源采集与证据链.md) | [Plan](../plans/002-监控来源采集与证据链计划.md) | `draft` |
| 003 | Codex 智能研判、事件、Heat 与人工治理 | [Design](../design/003-智能研判事件热度与人工治理设计.md) | [PRD](003-智能研判事件热度与人工治理.md) | [Plan](../plans/003-智能研判事件热度与人工治理计划.md) | `draft` |
| 004 | 通知、日报、Obsidian 知识投影与全文检索 | [Design](../design/004-通知报告知识投影与检索设计.md) | [PRD](004-通知报告知识投影与检索.md) | [Plan](../plans/004-通知报告知识投影与检索计划.md) | `draft` |
| 005 | 安全、运维、质量门禁与 24 周 P0 交付 | [Design](../design/005-安全运维质量与交付设计.md) | [PRD](005-安全运维质量与交付.md) | [Plan](../plans/005-安全运维质量与交付计划.md) | `draft` |

## 共通边界

- V1 P0 保持 Go 模块化单体、现有 PostgreSQL/River/MinIO/Redis、JWT/RBAC、统一 Result 和生成式 OpenAPI 契约。
- Kafka、Temporal、Python 微服务、Elasticsearch、Keycloak、增量迁移目录和生产高可用不是 V1 要求。
- 新知识检索不依赖向量、Embedding 或 RAG；仓库现有 Provider、ONNX、Embedding 与 pgvector 路径是待评估迁移现状，不得描述为已清理。
- 24 周只承诺完成通过门禁的 P0；评论、周报、更多授权来源、邮件和生产高可用均属于 P1。
- 每条 P0 FR/NFR 必须映射 Given-When-Then 验收，量化候选指标必须先补齐数据规模、环境、统计窗口与排除条件。
