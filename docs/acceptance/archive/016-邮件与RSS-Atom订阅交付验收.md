---
layer: Acceptance
doc_no: "016"
audience: [Dev, QA, Ops]
feature_area: 订阅与交付
purpose: 记录 PLAN-016 的可复核验收证据
canonical_path: docs/acceptance/archive/016-邮件与RSS-Atom订阅交付验收.md
status: archived
version: v0.1
owner: HotKey Server Team
result: accepted
---

# 邮件与 RSS/Atom 订阅交付验收

PLAN-016 已完成：Token 哈希与轮换、RSS/Atom/304、投递幂等与追加尝试、SMTP HTML/纯文本适配器、临时/永久错误退避、已发布报告消息读取，以及 `deliver_email` Job 的 Worker 装配。

证据：`0daab17`；Delivery application/domain/Feed/SMTP 测试、PostgreSQL 投递集成测试和 Bootstrap Worker 装配测试通过。真实 SMTP 发送只在部署环境启用，不在 CI 中发送外部邮件。

```bash
go test ./internal/modules/delivery/... -count=1
HOTKEY_TEST_DSN='postgres:///hotkey_server_dev?sslmode=disable' go run ./test/runner test -tags=integration ./internal/modules/delivery/infrastructure/postgres -run TestDeliveryRepositoryIsIdempotentAndAppendsAttempts -count=1
go run ./test/runner test ./internal/modules/delivery/... -count=1
```
