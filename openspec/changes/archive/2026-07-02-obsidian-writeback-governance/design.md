## Context

知识中台目前仅有 `HotKey → Obsidian` 的单向导出链路（通过 `PublishDailyTopicsJob`）。人工在 Obsidian Vault 中对 `manual_tags`、`analyst_conclusion`、`theme_ref`、`material_status` 等 frontmatter 字段的结构化编辑无法回流到 HotKey。

现有基础设施：
- `internal/obsidian/` 已具备 YAML frontmatter 渲染（`render.go`）、slug 生成（`slug.go`）和原子写入（`writer.go`）
- `internal/database/` 已有 21 个 GORM 仓库，但无侧车（sidecar）模型
- `internal/jobs/` 已有作业框架（`runner.go`）和 8 个独立作业
- OpenSpec specs 中已有 `daily-digest` 规格，exports 回归由 `scripts/validate-repository.sh` 驱动

约束：
- 机器事实表（`events`、`topics`、`platform_posts`、`monitor_post_hits` 等）不可被回写直接修改
- 人工知识只能落入 sidecar 模型（`event_annotations`、`topic_annotations`、`theme_memberships`）
- 无 approval workflow、无多人协同编辑

## Decisions

### D1: Sidecar 架构而非直接写入机器表

**决策**：所有回写字段写入独立的侧车表，而非直接修改 `topics`、`events` 等机器事实表。

**原因**：
- 保持机器事实层的单一写入源（仅平台采集和自动聚合）
- 防止人工操作意外覆盖自动计算结果（如 `current_heat_score`、`trend_direction`）
- 符合 PRD 第 5 条权限边界要求
- 回写字段有独立的审计和版本追踪

**被否方案**：在机器表上加 `jsonb` 扩展字段。被否理由：会导致模型层职责模糊，难以区分自动计算与人工输入。

### D2: Revision digest 冲突检测

**决策**：使用 Vault 对象的 `sha256(revision_digest)` 进行比较。回写时要求传入期望的 revision，若数据库当前 revision 不匹配则拒绝写入。

**原因**：
- 时间戳比较无法应对时区漂移和并发问题
- digest 比较轻量，不需要分布式锁
- 与现有 `monitor_runs` 的幂等设计逻辑一致

**被否方案**：乐观锁基于 `updated_at`。被否理由：updated_at 精度不足，且被回写和自动更新共享，不符合职责隔离。

### D3: 白名单字段在前端 parser 和后端 validator 双重校验

**决策**：`internal/obsidian/writeback_parser.go` 只解析 YAML frontmatter 并提取字段名；`internal/knowledge/validator.go` 独立校验字段是否在白名单内、值类型是否正确。

**原因**：
- 解析与校验解耦，parser 可复用（例如未来用于只读展示）
- 校验逻辑在 knowledge 层统一，单一失败路径

### D4: 审计日志与回写行为绑定，非异步解耦

**决策**：`RecordAttempt` 与 `ApplyChange` 在同一事务中执行。

**原因**：
- 保证审计记录不丢失
- 简化失败恢复（审计记录已包含失败原因）
- 批量回写 job 下性能可接受（回写是低频率操作）

### D5: Sidecar Repository 按对象类型拆分

**决策**：`event_annotation_repo.go`、`topic_annotation_repo.go`、`theme_membership_repo.go` 各自独立。

**原因**：
- 每个 sidecar 表的 schema 和查询模式不同
- 避免单一 repo 的 if-else 分支膨胀
- 后续可以独立测试各部分

## Risks / Trade-offs

| Risk | Mitigation |
|------|------------|
| 白名单字段扩展时需修改 parser 和 validator 两处 | 已定义 `allowedWritebackFields` maps，扩展只需修改一个常量 |
| Revision 碰撞极低概率 | sha256 碰撞概率可忽略；冲突时返回明确错误消息 |
| Sidecar 表数膨胀 | 首版只创建 3 个 sidecar 模型 + 1 个审计表 |
| 回写失败处理复杂 | 审计表记录完整失败链（detected → validated → applied/conflicted/rejected） |
