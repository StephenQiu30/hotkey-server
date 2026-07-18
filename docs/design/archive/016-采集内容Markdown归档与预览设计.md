---
layer: Design
doc_no: "016"
audience: [PM, Dev, QA, Ops]
feature_area: 内容归档与阅读
purpose: 定义授权 Feed 内容的 Markdown 投影与安全读取契约
canonical_path: docs/design/archive/016-采集内容Markdown归档与预览设计.md
status: accepted
version: v1.1
owner: HotKey Server Team
inputs:
  - AGENTS.md
  - docs/design/archive/005-数据来源查询规划与采集设计.md
  - docs/design/archive/006-内容标准化去重与证据设计.md
outputs:
  - 采集内容 Markdown 归档边界
  - 内容文档读取契约
  - Web 预览消费所需的服务端边界
triggers:
  - 新增或修改内容正文保存、Markdown 预览或 PDF 导出
downstream:
  - docs/prd/archive/019-采集内容Markdown归档与预览.md
  - docs/plans/archive/019-采集内容Markdown归档与预览计划.md
---

# 采集内容 Markdown 归档与预览设计

## 1. 目标与事实边界

本设计将来源 Feed 已经提供且来源连接明确允许保存的 `description`/`content`/`summary` 组织为 Markdown，供 HotKey 内阅读和通过浏览器打印对话框保存 PDF。它是“采集内容/摘要归档”，不等于 canonical URL 指向的完整网页或论文全文。

当 `allow_body_storage=false` 时，Source 不持久化 body，Content 层不得从标题、URL 或旧记录伪造正文。已发布连接的采集语义仍不可变；唯一例外是管理员按来源条款明确执行一次 `false` → `true` 的正文/摘要授权升级。该升级不改变 Feed 地址、过滤条件或采集范围，只影响后续采集。

## 2. 非目标

- 不请求 canonical URL，不绕过 robots、付费墙、登录或授权条款。
- 不回填无 body 的历史 Content，不将 metadata-only 内容标记为已归档。
- 不新增第二份 Schema、`content_documents` 表、PDF 对象或异步 PDF 任务。
- 不将“打印 / 保存 PDF”文案伪装成服务端一键 `.pdf` 下载。

## 3. 采集与 Markdown 投影

RSS/Atom 解析在 `content` 非空时优先使用它，否则退回 `description`/`summary`。CapturePolicy 仍是唯一授权门：只有 `allow_body_storage=true` 才把 body 交给 Ingestion。

Ingestion 使用固定版本 `github.com/JohannesKaufmann/html-to-markdown/v2@v2.5.2` 将 UTF-8 Feed HTML 转为 CommonMark + GFM table。转换只处理已授权的捕获字段，不发起网络请求。相对链接以 canonical URL 为基准解析；链接只允许相对地址以及 `http`、`https`、`mailto` 协议，其他协议和远程图片被删除，`script`/`style`/`iframe`/`form` 不进入 Markdown。转换失败时整条内容安全失败，不写入半成品。

归档继续复用 `content_assets.asset_type='text'` 和 `evidence/v1` 确定性 MinIO 对象，数据库 MIME 与 MinIO `Content-Type` 均固定为 `text/markdown; charset=utf-8`，SHA-256、大小、删除和对账语义不变。重放同一 Markdown SHA 只得到同一对象/一个 asset；内容变化产生新 SHA 时，读模型只选 `object_status='available' AND asset_type='text' AND mime_type='text/markdown; charset=utf-8'`，并按 `captured_at DESC, id DESC` 选唯一最新产物。历史 `text/plain` 不伪装成 Markdown ready。授权升级对应的已发布来源约束已同步写入 `db/schema.sql`。

## 4. 文档读取契约

`GET /api/v1/contents/{id}/document` 供 viewer、editor 和 admin 读取，仍使用统一 Result，由 Swagger 注解生成 OpenAPI。响应是明确 allowlist：`content_id`、`title`、`source_name`、`canonical_url`、`language`、`published_at`、`availability: ready | not_captured`、`markdown`、`sha256`、`captured_at`。

`ready` 只在存在 available Markdown text asset，并且对象读取后的 SHA-256、大小和 `Content-Type` 与数据库一致时返回。`not_captured` 是正常 200 空态，`markdown` 为空。稳定错误映射为：非法 id → `invalid_request`/400；Content 缺失、已删除或来源已删除 → `not_found`/404；存在 asset 但 MinIO 不可用、超过大小上限、SHA/大小/`Content-Type` 不一致 → `unavailable`/503。任何错误均不暴露 object key、MinIO endpoint 或正文。

EvidenceStore 增加受限读取端口：只接受 Repository 从当前 Content 的 asset 查出的 `evidence/v1` key，限制最大字节数，并校验对象元数据、数据库 asset 和实际内容的 SHA-256/大小。HTTP 参数不能控制 object key。

## 5. 下游消费边界

Server 只提供安全 Markdown 契约和真实可用性，不返回 HTML 或 PDF 产物。Web 预览、工作台布局和浏览器打印属于 `../hotkey-web` 自身文档与实施契约，不由本仓 Plan 修改。

## 6. 后续扩展门禁

完整网页提取需另立 Design，必须包含版权/robots/付费墙策略、SSRF 防护、跳转/大小/MIME/超时限制和资源预算。服务端一键 PDF 需另立 `DocumentExporter` 和异步缓存契约；可优先评估 Gotenberg，但不在本期引入外部运行时。

## 7. 变更记录

| 版本 | 日期 | 变更 |
|---|---|---|
| v0.1 | 2026-07-18 | 建立授权 Feed 内容、Markdown 归档、受限读取和浏览器 PDF 边界，等待独立审核。 |
| v0.2 | 2026-07-18 | 按 Plan Review 补齐 document API 的 400/404/503 稳定错误映射。 |
| v0.3 | 2026-07-18 | 按独立复审分离 Server 和 Web 实施边界。 |
| v0.4 | 2026-07-18 | 冻结 Markdown asset 的 MIME、幂等、多版本选择与历史 text/plain 隔离。 |
| v0.5 | 2026-07-18 | 对齐 CommonMark + GFM table，冻结安全链接协议及对象 Content-Type 完整性。 |
| v1.0 | 2026-07-18 | 经非主要编写者独立复核通过，设计 accepted。 |
| v1.1 | 2026-07-18 | PLAN-019 已按设计实现并通过独立复审；真实外部 MinIO 演练风险保留在验收记录。 |
