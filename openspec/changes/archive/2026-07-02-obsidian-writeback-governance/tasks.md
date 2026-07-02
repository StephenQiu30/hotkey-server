## 1. 冻结白名单字段与审计表

- [x] 1.1 新增 knowledge_writeback_logs 审计表到 db/schema.sql
- [x] 1.2 实现审计 repo 失败测试（Red: TestKnowledgeWritebackRepo_RecordAttempt）
- [x] 1.3 实现最小审计 repo（KnowledgeWritebackRepo 的 RecordAttempt 方法）
- [x] 1.4 运行测试确认通过（Green: go test ./internal/database -run TestKnowledgeWritebackRepo）
- [x] 1.5 Commit（5f62316 chore: 初始化知识回写白名单与审计模型）

## 2. 实现白名单解析器与校验器

- [x] 2.1 实现白名单解析失败测试（Red: TestParseWritebackFields）
- [x] 2.2 实现白名单解析器（ParseWritebackFields）
- [x] 2.3 实现非白名单字段拒绝测试（Red: TestValidateWriteback_RejectsMachineField）
- [x] 2.4 实现字段校验器（ValidateWriteback）
- [x] 2.5 运行所有 obsidian/knowledge 测试确认通过（Green）
- [x] 2.6 Commit（a165772 feat: 冻结知识回写白名单字段）

## 3. 实现冲突检测与回写应用服务

- [x] 3.1 实现 revision 冲突检测失败测试（Red: TestDetectConflict_OnStaleRevision）
- [x] 3.2 实现冲突检测器（DetectConflict）
- [x] 3.3 实现 event_annotation_repo
- [x] 3.4 实现 topic_annotation_repo
- [x] 3.5 实现 theme_membership_repo
- [x] 3.6 实现回写应用服务（ApplyChange 方法，串联 validate + conflict + sidecar write）
- [x] 3.7 运行知识层测试确认通过（Green）
- [x] 3.8 Commit（ea02a59 feat: 新增知识回写冲突检测与 sidecar 应用服务）

## 4. 实现批量回写 job 与 roundtrip 集成回归

- [x] 4.1 实现批量回写 job 失败测试（Red: TestApplyKnowledgeWritebackJob_Run）
- [x] 4.2 实现 ApplyKnowledgeWritebackJob
- [x] 4.3 实现 roundtrip 集成测试（TestKnowledgeWritebackRoundtrip）
- [x] 4.4 在 scripts/validate-repository.sh 追加 roundtrip 回归
- [x] 4.5 运行完整验证套件（Green: go test 覆盖内层包与集成测试）
- [x] 4.6 Commit（a574e3b test: 增加知识回写闭环回归）

## 5. Agent Review Rework（Code Review 修复）

- [x] 5.1 Parser 返回 `[]*WritebackChange` 而非单个指针，修复非确定性丢失问题
- [x] 5.2 theme_ref 改用 `change.ObjectType` 而非硬编码 `"topic"`
- [x] 5.3 审计错误由静默丢弃改为 log 输出
- [x] 5.4 GORM v2 INSERT 改用 `db.Exec()` 替代 `db.Raw()`（roundtrip 测试根本原因）
- [x] 5.5 新增 `TestValidateWriteback_AllowsWhitelistedField` 成功路径测试
- [x] 5.6 roundtrip 集成测试增加 DB 持久化断言
- [x] 5.7 移除废弃的 `WritebackResult.SkippedCount` 字段
- [x] 5.8 更新架构验证错误信息（14→18 表）
- [x] 5.9 Commit（81b52bb feat: 修复回写 parser 单字段返回、theme_ref 硬编码及 GORM Exec 问题）
