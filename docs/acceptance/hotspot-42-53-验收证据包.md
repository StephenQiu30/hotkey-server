---
layer: acceptance
doc_no: "hotspot-42-53"
audience:
  - PM
  - Dev
  - QA
  - Ops
purpose: "记录 #42-#53 热点判定、证据链、LangChain/LangGraph 专项的可复核验收结果。"
owner: "StephenQiu30"
inputs:
  - docs/product/prd/24-热点事件判定与热度引擎PRD.md
  - docs/product/prd/25-消息来源真伪验证与证据链PRD.md
  - docs/product/prd/26-AI模型接入抽象（LangChain-LangGraph）PRD.md
  - docs/product/prd/27-接入源适配器与替换机制PRD.md
  - docs/plans/28-里程碑与任务领取总控计划.md
outputs:
  - "#42-#53 验收命令与结果"
  - "真实 PostgreSQL/API 回放结果"
  - "Issue 关闭前剩余缺口"
triggers:
  - "Issue #42-#53 关闭前复核"
downstream:
  - GitHub Issue #42-#53
---

# #42-#53 热点专项验收证据包

## 1. 代码侧提交映射

- #42：`2f31084 fix: harden hotness scoring fallback for #42`
- #43：`b80140f fix: add hotness threshold fallback for #43`
- #44：`18bbf08 fix: record source route fallback for #44`
- #45：`38e61dd fix: expose source route fields for #45`
- #46：`01cdea5 fix: detect query pollution evidence for #46`
- #47：`d798b5c fix: guard low trust penalty config for #47`
- #48：`033b23b fix: expose persisted evidence fallback for #48`
- #49：`75599e0 fix: mark low trust source display for #49`
- #50：`80ec569 fix: route query expansion through LangChain for #50`
- #51：`9c6a2fb fix: normalize LangGraph switch for #51`
- #52：`891b29c fix: record LangGraph enhance decision for #52`
- #53：`68b4418 fix: trace provider fallback via LangChain for #53`

## 2. 自动化测试结果

- `python3 -m unittest tests.test_mvp_services`
  - 结果：通过
  - 证据：`Ran 86 tests in 0.376s`，`OK`
- `python3 -m unittest tests.test_ai_provider_acceptance`
  - 结果：通过
  - 证据：`Ran 3 tests`，`OK`
  - 覆盖：无凭据时给出可执行 required env；有真实 provider 返回时必须包含 `trace_id`、`ai_orchestrator_decision` 与 `provider_trace`。
- `python3 -m unittest discover -s tests -p 'test_repository_governance.py'`
  - 结果：通过
  - 证据：`Ran 7 tests`，`OK`
- `bash scripts/checkpoint-gate.sh`
  - 结果：通过
  - 证据：无运行时 `apps/` 残留，治理测试通过，工作区清洁

## 3. 真实 PostgreSQL 与 API 回放

### 3.1 依赖服务

- 命令：`docker compose -f docker-compose.prod.yml up -d postgres redis`
- 结果：通过
- 证据：`hotkey-server-postgres-1` 与 `hotkey-server-redis-1` 均为 `healthy`

### 3.2 Schema 初始化

- 命令：`DATABASE_URL='postgresql+psycopg://postgres:postgres@localhost:5432/ai_hotspot_radar' python3 -m server.app.db.init_schema`
- 结果：通过

### 3.3 FastAPI lifespan 健康检查

- 命令：使用真实 `DATABASE_URL` 启动 `TestClient(create_app())` 并请求 `/api/health`
- 结果：通过
- 证据：HTTP `200`，响应 `{"status":"ok"}`

### 3.4 热点列表与详情字段回放

- 命令：写入包含 `hotness_*`、`source_risk_*`、`source_fallback`、`source_evidence_bundle`、AI trace 的热点记录，并通过真实 PostgreSQL 请求 `/api/hotspots` 与 `/api/hotspots/{id}`
- 结果：通过
- 证据：
  - 列表：HTTP `200`
  - 详情：HTTP `200`
  - `hotness_score=88.5`
  - `source_risk_level=low`
  - `source_risk_badge=low_trust_source`
  - `source_selected=Acceptance RSS`
  - `source_fallback.reason=timeout`
  - `source_evidence_bundle.cross_source_count=2`
  - `ai_analysis.raw_response.trace_id=acceptance-trace`

## 4. PRD 对齐结论

- PRD 24：热度评分、阈值回退、hotness 响应字段、来源路由回退已具备自动化测试和真实 API 字段回放证据。
- PRD 25：来源证据采集、重复参数污染识别、低置信降权、证据回读、低置信展示标记已具备自动化测试和真实 API 字段回放证据。
- PRD 26：LangChain 默认路径、LangGraph 默认关闭、增强成功决策、provider fallback trace 已具备自动化测试证据。
- PRD 27：来源路由、失败退化、fallback 审计字段已具备自动化测试和真实 API 字段回放证据。

## 5. Issue 关闭前剩余缺口

- GitHub #42-#49 已回填证据并关闭；#50-#53 仍保持 `OPEN`，由 #55 跟踪真实外部 AI provider 最终验收。
- 真实外部 provider 尚未回放：OpenAI 兼容模型、X/Bing/公开源、SMTP/通知链路。
- 当前 AI 真实外部调用未配置有效 `OPENAI_API_KEY` / `OPENAI_MODEL`，本轮 AI 验证以 fallback/provider trace 自动化测试为准。
- #55 的可执行验收入口已补齐：
  - 命令：`PYTHONPATH=. python3 scripts/verify_ai_provider.py --json`
  - 当前无凭据结果：`status=missing_credentials`
  - 当前 required env：`OPENAI_API_KEY`、`OPENAI_MODEL`
  - 缺凭据退出码：`2`
- 若关闭 #50-#53，需要配置真实 OpenAI 兼容 provider 后执行上述脚本，并回填成功输出中的 `trace_id`、`ai_orchestrator_decision` 与 `provider_trace`。
