---
layer: Acceptance
doc_no: "014"
audience: [Dev, QA, Ops]
feature_area: 知识库治理
purpose: 记录 PLAN-014 的可复核验收证据
canonical_path: docs/acceptance/archive/014-Obsidian知识提案修订与对账验收.md
status: archived
version: v0.1
owner: HotKey Server Team
result: accepted
---

# Obsidian 知识提案、修订与对账验收

PLAN-014 已完成：稳定路径与符号链接拒绝、人工区域保留、提案版本/冲突校验、PostgreSQL 修订记录、Vault 原子写入、MinIO 快照适配器、三方扫描对账、管理员 API，以及 `project_knowledge`/`reconcile_knowledge` Job 已接入 Worker。

证据：`0daab17`；Knowledge domain/Vault/Application 测试、PostgreSQL 持久化测试、OpenAPI 合同和 Bootstrap Worker 装配测试通过。MinIO 快照在配置有效时启用，未配置时保持本地 P0 可运行。

```bash
go test ./internal/modules/knowledge/... -count=1
HOTKEY_TEST_DSN='postgres:///hotkey_server_dev?sslmode=disable' go run ./test/runner test -tags=integration ./internal/modules/knowledge/infrastructure/postgres -run TestKnowledgeRepositoryPersistsDocumentAndProposal -count=1
HOTKEY_TEST_DSN='postgres:///hotkey_server_dev?sslmode=disable' HOTKEY_TEST_REDIS_URL='redis://127.0.0.1:6379/15' go run ./test/runner test ./internal/bootstrap -count=1
```
