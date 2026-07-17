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

当前已实现 Token 哈希、SMTP 临时错误分类、RSS 2.0/Atom 输出和内容 ETag（提交 `5fc28a2`），Feed 单元测试已通过。

尚未完成：订阅/Delivery/Attempt Repository、SMTP 发送与退避 Job、私有 Token API、304/Last-Modified HTTP 契约和独立复核；保持 `pending`。

```bash
go test ./internal/modules/delivery/... -count=1
```
