# n8n Workflow 模板目录

本目录存放可导入 n8n 的 workflow 模板，用于 hotkey-server 的外部自动化编排。

## 目录结构

```
n8n/
  README.md           # 本文件
  workflows/          # workflow JSON 模板
    fact_source_collector.json
    signal_source_collector.json
    daily_ai_hotspot_email_digest.json
```

## 本地 n8n 地址

- 默认地址：`http://localhost:5678`
- 可通过 `.env` 中的 `N8N_BASE_URL` 修改

## 导入步骤

1. 启动 n8n 实例
2. 进入 n8n 界面，点击 **Import from File**
3. 选择 `workflows/` 目录下的 JSON 文件
4. 配置 Credentials（见下方凭证说明）
5. 激活 workflow

## 凭证配置

### hotkey-server Internal API

在 n8n 中创建 **Header Auth** 类型的 Credential：

- **Credential Name**: `hotkey-internal-api`
- **Header Name**: `X-HotKey-Internal-Key`
- **Header Value**: 你的 `HOTKEY_INTERNAL_API_KEY` 值

workflow 中还需要设置以下 header：

| Header | 说明 | 示例值 |
| --- | --- | --- |
| `X-HotKey-Internal-Key` | Internal API 共享密钥 | 见 `.env` 中 `HOTKEY_INTERNAL_API_KEY` |
| `X-HotKey-Tenant-ID` | 租户 ID | `tenant-default` |
| `Idempotency-Key` | 幂等键，防重复执行 | 使用 `{{$runId}}` 或自定义 |

### SMTP（日报邮件发送）

在 n8n 中创建 **SMTP** 类型的 Credential：

- **Credential Name**: `hotkey-smtp`
- **Host**: `smtp.example.com`
- **Port**: `587`
- **User**: 你的 SMTP 用户名
- **Password**: 你的 SMTP 密码
- **From**: `HotKey <no-reply@example.com>`

## 安全边界

- **workflow 模板不包含真实凭证**，所有敏感值使用占位变量
- 凭证只在 n8n 实例或部署环境中维护，不入库
- `.env.example` 中的变量名与本 README 保持一致

## Internal API 端点

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| POST | `/api/v1/internal/workflow/status` | 回写 workflow 执行状态 |
| POST | `/api/v1/internal/ingest/contents` | 批量采集内容入库 |
| GET | `/api/v1/internal/daily/candidates` | 查询日报候选事件 |
| POST | `/api/v1/internal/daily/reports` | 保存日报 |

## 环境变量对照

| 变量 | 用途 | 默认值 |
| --- | --- | --- |
| `HOTKEY_INTERNAL_API_KEY` | Internal API 共享密钥 | 无，必须配置 |
| `HOTKEY_DEFAULT_TENANT_ID` | 默认租户 ID | `tenant-default` |
| `N8N_BASE_URL` | n8n 实例地址 | `http://localhost:5678` |
| `N8N_FACT_SOURCE_WORKFLOW` | 事实源采集 workflow 名 | `fact_source_collector` |
| `N8N_SIGNAL_SOURCE_WORKFLOW` | 传播源采集 workflow 名 | `signal_source_collector` |
| `N8N_DAILY_REPORT_WORKFLOW` | 日报 workflow 名 | `daily_ai_hotspot_email_digest` |
| `DASHSCOPE_API_KEY` | 阿里云灵积 API Key | 无，必须配置 |
| `DASHSCOPE_BASE_URL` | DashScope OpenAI 兼容接口地址 | `https://dashscope.aliyuncs.com/compatible-mode/v1` |
| `DASHSCOPE_CHAT_MODEL` | 聊天/总结模型 | `qwen-plus` |
| `DASHSCOPE_EMBEDDING_MODEL` | 向量模型 | `text-embedding-v2` |
| `SMTP_HOST` | SMTP 服务器地址 | `smtp.example.com` |
| `SMTP_PORT` | SMTP 端口 | `587` |
| `DAILY_REPORT_RECIPIENTS` | 日报收件人（逗号分隔） | `admin@example.com` |
