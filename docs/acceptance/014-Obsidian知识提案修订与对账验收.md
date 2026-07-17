---
layer: Acceptance
doc_no: "014"
audience: [Dev, QA, Ops]
feature_area: 知识库治理
purpose: 记录 PLAN-014 的可复核验收证据
canonical_path: docs/acceptance/014-Obsidian知识提案修订与对账验收.md
status: review
version: v0.1
owner: HotKey Server Team
result: pending
---

# Obsidian 知识提案、修订与对账验收

当前已实现安全稳定路径、内容哈希、提案 base revision 校验、knowledge_documents/proposals PostgreSQL 持久化和临时文件 flush/原子 rename，路径逃逸测试及持久化集成测试已通过。

尚未完成：三方对账、MinIO 快照、提案 HTTP/Job、冲突恢复和独立复核；保持 `pending`。

```bash
go test ./internal/modules/knowledge/... -count=1
HOTKEY_TEST_DSN='postgres:///hotkey_plan010_test?sslmode=disable' go test -tags=integration ./internal/modules/knowledge/infrastructure/postgres -run TestKnowledgeRepositoryPersistsDocumentAndProposal -count=1
```
