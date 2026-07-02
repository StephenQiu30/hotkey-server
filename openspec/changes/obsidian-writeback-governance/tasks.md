## 1. 冻结白名单字段与审计表

- [ ] 1.1 新增 knowledge_writeback_logs 审计表到 db/schema.sql
- [ ] 1.2 实现审计 repo 失败测试（Red: TestKnowledgeWritebackRepo_RecordAttempt）
- [ ] 1.3 实现最小审计 repo（KnowledgeWritebackRepo 的 RecordAttempt 方法）
- [ ] 1.4 运行测试确认通过（Green: go test ./internal/database -run TestKnowledgeWritebackRepo）
- [ ] 1.5 Commit（chore: 初始化知识回写白名单与审计模型）

## 2. 实现白名单解析器与校验器

- [ ] 2.1 实现白名单解析失败测试（Red: TestParseWritebackFields）
- [ ] 2.2 实现白名单解析器（ParseWritebackFields）
- [ ] 2.3 实现非白名单字段拒绝测试（Red: TestValidateWriteback_RejectsMachineField）
- [ ] 2.4 实现字段校验器（ValidateWriteback）
- [ ] 2.5 运行所有 obsidian/knowledge 测试确认通过（Green）
- [ ] 2.6 Commit（feat: 冻结知识回写白名单字段）

## 3. 实现冲突检测与回写应用服务

- [ ] 3.1 实现 revision 冲突检测失败测试（Red: TestDetectConflict_OnStaleRevision）
- [ ] 3.2 实现冲突检测器（DetectConflict）
- [ ] 3.3 实现 event_annotation_repo
- [ ] 3.4 实现 topic_annotation_repo
- [ ] 3.5 实现 theme_membership_repo
- [ ] 3.6 实现回写应用服务（ApplyChange 方法，串联 validate + conflict + sidecar write）
- [ ] 3.7 运行知识层测试确认通过（Green）
- [ ] 3.8 Commit（feat: 新增知识回写冲突检测与 sidecar 应用服务）

## 4. 实现批量回写 job 与 roundtrip 集成回归

- [ ] 4.1 实现批量回写 job 失败测试（Red: TestApplyKnowledgeWritebackJob_Run）
- [ ] 4.2 实现 ApplyKnowledgeWritebackJob
- [ ] 4.3 实现 roundtrip 集成测试（TestKnowledgeWritebackRoundtrip）
- [ ] 4.4 在 scripts/validate-repository.sh 追加 roundtrip 回归
- [ ] 4.5 运行完整验证套件（Green: go test 覆盖内层包与集成测试）
- [ ] 4.6 Commit（test: 增加知识回写闭环回归）
