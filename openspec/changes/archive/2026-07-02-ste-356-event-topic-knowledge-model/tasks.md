## 1. Schema & Database Layer

- [x] 1.1 在 `db/schema.sql` 中新增 events, topic_events, knowledge_runs, themes, export_bundles, event_annotations, topic_annotations, theme_memberships, knowledge_object_revisions 九张表
- [x] 1.2 在 `internal/database/models.go` 中对应的 GORM 模型
- [x] 1.3 实现 `internal/database/eventrepo.go` (EventRepo: CreateEvent, GetEvent, ListEventsByMonitor)
- [x] 1.4 创建 `internal/database/eventrepo_test.go` (TestEventRepo_CreateEvent)
- [x] 1.5 实现 `internal/database/topic_event_linker.go` (TopicEventLinker)

## 2. Event Domain Service

- [x] 2.1 创建 `internal/event/service.go` (EventService: BuildEventFromPosts)
- [x] 2.2 创建 `internal/event/service_test.go` (TestService_BuildEventFromPosts, TestService_EventIsNotTopicAlias)
- [x] 2.3 实现 `internal/topic/topic_event_linker.go` 接口

## 3. Knowledge Sync Baseline Job

- [x] 3.1 创建 `internal/jobs/publish_knowledge_snapshot.go` (PublishKnowledgeSnapshotJob)
- [x] 3.2 创建 `internal/jobs/publish_knowledge_snapshot_test.go` (TestPublishKnowledgeSnapshotJob_Run)
- [x] 3.3 修改 `internal/jobs/publish_daily_topics.go` 为兼容适配层
- [x] 3.4 修改 `internal/app/worker_jobs.go` 注册新 job

## 4. Contract & Obsidian Layer

- [x] 4.1 创建 `internal/obsidian/contracts.go` (BuildEventContract, BuildRevision, ExportBundleSeed, KnowledgeRevision)
- [x] 4.2 创建 `internal/obsidian/contracts_test.go` (TestKnowledgeContract_MinimumFields, TestBuildRevision)

## 5. Verification

- [x] 5.1 运行 `go test ./internal/event ./internal/topic ./internal/jobs ./internal/database ./internal/obsidian -v` 全量通过
- [x] 5.2 检查 Event != Topic 别名验证
- [x] 5.3 检查未引入 db/migrations/ 目录
