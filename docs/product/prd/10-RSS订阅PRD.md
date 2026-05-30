---
layer: PRD
doc_no: "10"
audience:
  - PM
  - Tech-Lead
  - Dev
  - QA
feature_area: "area:rss"
purpose: "定义公开频道 RSS 与用户私有 RSS 输出。"
canonical_path: "docs/product/prd/10-RSS订阅PRD.md"
status: draft
version: "1.0.0"
owner: "StephenQiu30"
inputs:
  - "README.md"
  - "WORKFLOW.md"
outputs:
  - "RSS订阅需求边界"
  - "RSS订阅TDD验收标准"
triggers:
  - "RSS订阅范围变更"
  - "对应 Linear issue 拆分或合并"
downstream:
  - "docs/plans/10-RSS订阅实现计划.md"
---

# 10-RSS订阅 PRD

## 1. 背景

RSS 是日报的重要订阅方式，既要支持公开频道传播，也要支持用户私有个性化订阅。

## 2. 目标

提供公开频道 RSS 和用户私有 RSS token 输出。

## 3. 范围

- 公开频道 RSS。
- 用户私有 RSS。
- RSS token 管理。
- RSS XML contract。

## 4. 非目标

- 不发送邮件。
- 不生成日报内容。
- 不实现前端订阅页面。

## 5. 用户故事

- 作为读者，我可以订阅公开 AI 模型频道 RSS。
- 作为登录用户，我可以订阅包含自己关键词的私有 RSS。
- 作为用户，我可以重置私有 RSS token。

## 6. 数据与 API 边界

数据表：`rss_feeds`。

API：`GET /rss/channels/{code}.xml`、`GET /rss/users/{token}.xml`、用户 RSS token 管理 API。

## 7. 后台任务影响

RSS 读取已生成 daily_reports，可选 `refresh_rss_cache` 任务预生成缓存。

## 8. 配置影响

RSS base URL、token 长度、缓存 TTL。

## 9. 错误与降级

私有 token 无效返回 `404` 或 `401`，禁用 feed 不输出内容。

## 10. 安全与合规

私有 RSS token 必须不可猜测，可撤销，不在日志中明文输出。

## 11. 验收标准

- Given 频道日报存在，When 请求频道 RSS，Then 返回合法 XML。
- Given 私有 token 有效，When 请求用户 RSS，Then 返回用户个性化日报。
- Given token 被重置，When 使用旧 token，Then 访问失败。

## 12. TDD 验收标准

- Plan 必须先写失败测试，再写最小实现，再运行测试通过。
- 涉及 HTTP API 的任务必须包含 handler/contract 测试。
- 涉及数据库的任务必须包含 migration 和 repository 测试。
- 涉及 Redis 的任务必须包含队列、幂等、重试或降级测试。
- 涉及 DashScope 或 SMTP 的任务必须使用 fake/mock 测试，不依赖真实外部服务通过基础测试。

## 13. Harness 执行要求

每个 Linear issue 必须包含：

```text
1. Read PRD: docs/product/prd/10-RSS订阅PRD.md
2. Read Plan: docs/plans/10-RSS订阅实现计划.md
3. Write failing test first
4. Run expected failing command
5. Implement minimal code
6. Run required verification
7. Update OpenAPI or migrations when needed
8. Commit with Chinese message
9. Report commands, results, risks, and changed files back to Linear
```

Symphony 在本地 `Agents` 目录监听 Linear issue，并在独立 workspace 中执行。HotKey 不重写 Symphony 规范，只在 `WORKFLOW.md` prompt 中约束执行行为。

## 14. PRD 自审清单

- 本 PRD 是否只覆盖一个 feature。
- 用户、管理员或系统任务的输入输出是否明确。
- 范围和非目标是否能阻止越界实现。
- 数据、API、任务和配置影响是否可拆成 Plan。
- 验收标准是否可测试、可自动化、可在 harness 中执行。
- 是否遵循 TDD，且不要求先写生产代码。

## 15. 变更记录

| 日期 | 作者 | 版本 | 变更说明 |
| --- | --- | --- | --- |
| 2026-05-31 | StephenQiu30 | 1.0.0 | 初版，按 server-only AI 热点检测与日报服务 feature 拆分创建 |

