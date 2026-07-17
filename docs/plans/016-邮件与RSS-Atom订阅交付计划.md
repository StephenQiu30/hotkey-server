---
layer: Plan
doc_no: "016"
audience: [Dev, QA, Ops]
feature_area: 订阅与交付
purpose: 实施邮件投递及 RSS、Atom 私有订阅
canonical_path: docs/plans/016-邮件与RSS-Atom订阅交付计划.md
status: review
execution_status: in_progress
review_status: pending
version: v1.0
owner: HotKey Server Team
inputs:
  - docs/prd/016-邮件与RSS-Atom订阅交付.md
  - docs/plans/014-Obsidian知识提案修订与对账计划.md
  - docs/plans/015-日报周报与发布快照计划.md
outputs:
  - delivery 模块
  - 邮件与 Feed 交付
triggers:
  - PRD-016 accepted 且 ready
downstream:
  - docs/acceptance/016-邮件与RSS-Atom订阅交付验收.md
depends_on: [PLAN-014, PLAN-015]
---

# 邮件与 RSS/Atom 订阅交付计划

## 计划目标

对已发布报告提供最多一次成功邮件和支持条件请求、Token 轮换的私有 Feed。

## 开工条件

- 当前 Plan 的 status 为 accepted、review_status 为 approved、execution_status 为 ready
- 对应 PRD 的 status 为 accepted，execution_status 为 ready
- frontmatter 中 depends_on 列出的 Plan 全部为 done
- main 已同步，工作区只包含当前任务相关文件

## 执行文件

| 动作 | 路径 | 目的 |
|---|---|---|
| 创建 | internal/modules/delivery/domain/*.go | Subscription、Delivery、Attempt |
| 创建 | internal/modules/delivery/application/subscription.go | 订阅管理 |
| 创建 | internal/modules/delivery/application/email.go | 邮件投递与重试 |
| 创建 | internal/modules/delivery/application/feed.go | RSS/Atom 输出 |
| 创建 | internal/modules/delivery/infrastructure/postgres/*.go | 投递持久化 |
| 创建 | internal/modules/delivery/infrastructure/smtp/*.go | MailSender 适配 |
| 创建 | internal/modules/delivery/templates/* | HTML 与纯文本邮件 |
| 创建 | internal/modules/delivery/transport/http/*.go | 订阅、Feed 与运行 API |
| 创建 | internal/modules/delivery/infrastructure/jobs/deliver_email.go | 邮件 Job |
| 修改 | db/schema.sql | subscriptions、deliveries、attempts |
| 创建 | internal/modules/delivery/**/*_test.go | SMTP、幂等、304 与 Token 测试 |

## 执行步骤

1. 先写投递幂等、临时/永久错误、Feed 304 和 Token 轮换红灯测试。
2. 同步订阅、投递和尝试记录 Schema。
3. 实现订阅范围、时区、渠道、启停与 Token 哈希。
4. 实现 SMTP HTML/纯文本、退避与永久失败。
5. 实现 RSS 2.0、Atom、ETag 和 Last-Modified。
6. 接入 deliver_email Job、运行查询和 OpenAPI。

## 验收命令

| 阶段 | 命令 | 通过标准 |
|---|---|---|
| 红灯 | go test ./internal/modules/delivery/... -count=1 | 投递与 Feed 测试失败 |
| 绿灯 | go test ./internal/modules/delivery/... -count=1 | 全部通过 |
| SMTP | go test -tags=integration ./internal/modules/delivery/... -run SMTP -count=1 | 临时与永久错误通过 |
| Feed | go test ./internal/modules/delivery/... -run Feed -count=1 | RSS、Atom 与 304 通过 |
| 全量 | make ci | 全部通过 |

## 验收清单

- 同一报告与订阅最多一次成功邮件
- 每次尝试追加不可变记录
- 永久错误不无限重试
- Feed 只含允许范围的已发布报告
- ETag 与 Last-Modified 稳定
- Token 轮换立即使旧地址失效
- 日志和 API 不泄露 SMTP 凭据或 Token

## 提交边界

- test: 定义投递与 Feed 门禁
- impl: 实现订阅、邮件和 Feed
- feat: 接入 deliver_email Job 与公共 Feed


## 风险与回滚

- 实现需要改变 Design 或 PRD 契约时立即停止，先更新正式文档并重新复核
- 下游 Plan 开工前，可整体回退本任务提交并同步恢复 Schema、OpenAPI 和测试
- 下游 Plan 已开工后，使用新的前向修复 Plan，不恢复旧双轨或兼容实现
