---
layer: Acceptance
doc_no: "016"
audience: [Dev, QA, Ops]
feature_area: 订阅与交付
purpose: 记录 PLAN-016 的可复核验收证据
canonical_path: docs/acceptance/016-邮件与RSS-Atom订阅交付验收.md
status: review
version: v0.1
owner: HotKey Server Team
result: pending
---

# 邮件与 RSS/Atom 订阅交付验收

当前已实现 Token 哈希、SMTP 临时错误分类、RSS 2.0/Atom 输出、内容 ETag/Last-Modified 条件请求、私有 Feed 传输契约，以及订阅/投递/尝试 PostgreSQL 持久化和报告+订阅幂等键。当前用户可创建、查询、修改自己的邮件或 RSS 订阅；RSS Token 只在创建或轮换时返回一次，数据库、普通 API 和审计均不保存明文，轮换在同一事务内立即使旧 Token 失效并写入安全审计元数据。

尚未完成：SMTP 发送与退避 Job、投递运行查询、生产 Fx 邮件装配和独立复核；保持 `pending`。

```bash
go test ./internal/modules/delivery/... -count=1
HOTKEY_TEST_DSN='postgres:///hotkey_plan010_test?sslmode=disable' go test -tags=integration ./internal/modules/delivery/infrastructure/postgres -run TestDeliveryRepositoryIsIdempotentAndAppendsAttempts -count=1
HOTKEY_TEST_DSN='postgres:///hotkey_plan010_test?sslmode=disable' go test -tags=integration ./internal/modules/delivery/application -run TestSubscriptionServiceRotatesOnlyHashedTokenAndAudits -count=1
go test ./internal/modules/delivery/transport/http -run TestFeedSupportsETagAndLastModified -count=1
```
