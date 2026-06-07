---
layer: PRD
doc_no: "13"
audience:
  - PM
  - Tech-Lead
  - Dev
  - QA
feature_area: "area:source"
purpose: "定义 B站（Bilibili）来源的视频、动态、专栏采集策略与标准化。"
canonical_path: "docs/product/prd/13-B站授权与采集PRD.md"
status: draft
version: "1.0.0"
owner: "StephenQiu30"
inputs:
  - "docs/product/prd/04-来源与采集策略PRD.md"
  - "docs/product/prd/06-内容标准化与去重PRD.md"
outputs:
  - "B站采集需求边界"
  - "B站采集TDD验收标准"
triggers:
  - "B站采集范围变更"
  - "对应 Linear issue 拆分或合并"
downstream:
  - "B站适配器实现"
---

# 13-B站授权与采集 PRD

## 1. 背景

B站是中国最大的创作者平台之一，包含视频、动态（图文/转发）、专栏文章等多种内容形式。AI 热点检测需要采集 B站的公开内容，包括标题、简介、字幕/转写文本、作者、发布时间、链接和互动指标。

## 2. 目标

建立 B站来源适配器，支持视频列表、动态 feed 的标准化采集，处理字幕缺失降级、视频下架过滤、频控限制等异常场景。

## 3. 范围

- 来源类型：`bilibili`
- 内容类型：视频（热门/用户空间/分区）、动态（图文/转发）
- 字段标准化：标题、描述/简介、作者、发布时间、BV 号、播放/点赞/投币/分享/评论数
- 异常处理：视频下架过滤（code: -404）、字幕缺失降级到描述、频控错误分类（code: -412）

## 4. 非范围

- 登录态采集（需要 Cookie/SESSDATA）
- 视频字幕 API 实时获取（需要 wbi 签名）
- 专栏文章采集（后续迭代）
- 弹幕采集
- 用户关注关系

## 5. 用户故事

**作为** 内容创作者平台管理员
**我想要** 配置 B站来源并自动采集热门视频和动态
**以便** 系统能检测 B站热点话题并生成 AI 摘要

## 6. 数据与 API 边界

### 6.1 视频列表 API

- 热门：`https://api.bilibili.com/x/web-interface/popular?ps=20&pn=1`
- 用户空间：`https://api.bilibili.com/x/space/wbi/arc/search?mid={uid}&ps=30&pn=1`
- 响应格式：`data.list` 为视频数组（热门）或 `data.list.vlist`（空间）

### 6.2 动态 feed API

- 用户动态：`https://api.bilibili.com/x/polymer/web-dynamic/v1/feed/space?host_mid={uid}`
- 响应格式：`data.items` 为动态数组

### 6.3 字段映射

| B站字段 | 标准化字段 | 说明 |
|---------|-----------|------|
| `bvid` | `ExternalID` | BV 号，用于去重 |
| `title` | `Title` | 视频标题 |
| `desc` / `description` | `Snippet` | 视频简介（字幕缺失时的降级来源） |
| `author` | 作者名 | UP 主名称 |
| `pubdate` | `PublishedAt` | 发布时间戳 |
| `stat.view` | `Score` | 播放量 |
| `stat.reply` | `Descendants` | 评论数 |

### 6.4 URL 生成

- 视频：`https://www.bilibili.com/video/{bvid}`
- 动态：`https://www.bilibili.com/dynamic/{id_str}`

## 7. 异常处理

### 7.1 视频下架

- API 返回 `code: -404`，`message: "视频不见了"`
- 处理：跳过该视频，不返回给调用方

### 7.2 频控限制

- API 返回 `code: -412`，`message: "请求过于频繁"`
- 处理：返回错误，由上游决定重试策略

### 7.3 字幕缺失

- 视频无字幕/转写时，降级使用 `desc`（简介）作为 `Snippet`
- 简介为空时，`Snippet` 为空字符串

### 7.4 其他 API 错误

- 非零 `code`（非 -404/-412）：返回通用错误

## 8. 配置影响

- 新增来源类型 `bilibili` 到 `sources.type` CHECK 约束
- 无需额外环境变量，来源 URL 存储在 `sources` 表

## 9. 安全

- 使用公开 API，无需认证
- User-Agent 设置为 `HotKeyBot/1.0` 标识采集来源
- 遵守 B站 robots.txt 和 API 使用条款

## 10. 验收标准

- [ ] `BiliBiliFetcher` 实现 `Fetcher` 接口
- [ ] 视频列表解析（热门 API 和空间 API 双格式）
- [ ] 动态 feed 解析
- [ ] BV 号去重
- [ ] 下架视频过滤（code: -404）
- [ ] 频控错误分类（code: -412）
- [ ] 字幕缺失降级到描述
- [ ] DB 迁移扩展 source type 约束
- [ ] `go test ./internal/platform/fetcher/...` 通过
- [ ] `go test ./...` 全量通过

## 11. TDD 验收标准

- [ ] 视频标准化测试（标题、作者、发布时间、BV ID）
- [ ] 动态标准化测试
- [ ] 字幕缺失降级测试
- [ ] 下架过滤测试
- [ ] BV/URL 幂等测试
- [ ] 频控错误分类测试
- [ ] HTTP 错误处理测试
- [ ] 空标题跳过测试

## 12. 变更日志

| 版本 | 日期 | 变更 |
|------|------|------|
| 1.0.0 | 2026-06-07 | 初始版本 |
