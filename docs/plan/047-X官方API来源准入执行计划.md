---
layer: Plan
scope: issue
doc_no: "047"
title: X官方API来源准入执行计划
status: blocked
version: v1.0
date: 2026-09-26
owner: HotKey Team
canonical_path: docs/plan/047-X官方API来源准入执行计划.md
prd: docs/prd/007-扩展能力需求.md
design: docs/design/007-扩展能力设计.md
source_task: 新增缺口；旧M6的X能力缺官方API逐端点准入
architecture_prerequisite: "046 S03"
depends_on: ["029"]
---

# Plan 047：X 官方 API 来源准入

FR-007-004、OPEN-001-104、AC-007-003。已有 `sources/adapters/x_api.py`、`x_user_lookup.py` 及MockTransport测试，其现有代码、受控测试与维护责任归本卡研究台账；没有凭据/月度硬上限和当期接口许可证据，不能执行真实请求。此卡只完成X准入与实施契约，不能以已有适配器替代真实接入；获准后须另建逐能力实施Plan并写入索引，不能直接将047改为实现卡。

| SPEC | 本卡输出 |
|---|---|
| SPEC-047-SEC-001 | 在Design007记录官方API产品/授权范围、现行定价与配额来源日期、月度硬上限和每类请求计费单位；预算预留必须在请求前，未知账单结果不返还。凭据只写私有环境引用，不写值。 |
| SPEC-047-DATA-001 | 对照当前适配器确认搜索、作者解析、作者新帖、评论各端点是否获准，固定分页token、原生ID、指标null、删除/保护账号/429与错误语义。不能假设搜索权限自动包含作者或评论权限。 |
| SPEC-047-OPS-001 | 只读审当前 `backend/app/sources/adapters/x_api.py`、`x_user_lookup.py`、`jobs/schemas.py` 的费用模型及unit测试，输出差距；获准能力分别建立实现Plan及真实验收预算，每次真实probe也计账本。 |

- [ ] CHK-047-001 → SEC-001：官方资料和用户预算决定齐全；该步骤完成前零真实API调用。
- [ ] CHK-047-002 → DATA-001：逐端点确认能力、数据授权/保留与分页边界，形成拒绝/允许/未知三态清单。
- [ ] CHK-047-003 → OPS-001：补实施Plan、请求前预算断言和逐能力原帖/入库/覆盖验收；本卡完成只关闭准入工作，不关闭AC-007-003。

用户负责凭据和月预算决定，实施者负责官方证据及代码差距；输出归M6 Acceptance研究小节。未获准保持关闭，不用第三方网页或其他平台替代官方API。

## 阶段门禁

- [ ] CHK-047-G0-001：核对046 S03、029、OPEN-001-104和现有两个适配器/测试。
- [ ] CHK-047-G1-001：冻结SEC/DATA/OPS逐端点许可、月预算、身份和分页研究问题。
- [ ] CHK-047-G2-001：保存无凭据/无月上限时零真实请求的受控拒绝证据。
- [ ] CHK-047-G3-001：完成CHK-047-001—003及获准能力的后续实施卡合同。
- [ ] CHK-047-G4-001：核对官方资料日期、受控MockTransport和文档/敏感信息门禁。
- [ ] CHK-047-G5-001：研究卡不适用真实采集；凭据、许可、月上限未确认前保持blocked。
- [ ] CHK-047-G6-001：只记录AC-007-003的X准入状态，真实接入由未来实施卡关闭。
