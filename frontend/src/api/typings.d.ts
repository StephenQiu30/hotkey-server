declare namespace API {
  type activateMonitorParams = {
    identity: string;
  };

  type addEventMemberParams = {
    identity: string;
  };

  type AnalysisComposition = {
    /** Ordering Origins */
    ordering_origins: Record<string, any>;
    /** Platforms */
    platforms: Record<string, any>;
    /** Roots */
    roots: number;
    /** Time Buckets */
    time_buckets: Record<string, any>;
  };

  type AnalysisContextView = {
    /** Available */
    available: boolean;
    /** Canonical Url */
    canonical_url: string | null;
    /** Content Id */
    content_id: string;
    /** Content Version Id */
    content_version_id: string;
    /** Role */
    role: "sample" | "parent" | "root";
    /** Text */
    text: string | null;
    /** Text Sha256 */
    text_sha256: string;
  };

  type AnalysisLabelInput = {
    /** Abstained */
    abstained?: boolean;
    /** Citation Content Version Ids */
    citation_content_version_ids?: string[];
    /** Request */
    request?: string;
    /** Sentiment */
    sentiment: "positive" | "negative" | "neutral" | "mixed" | "unknown";
    /** Stance */
    stance: "support" | "oppose" | "neutral" | "mixed" | "unknown";
    /** Target */
    target?: string;
    /** Topic */
    topic?: string;
  };

  type AnalysisLabelView = {
    /** Abstained */
    abstained: boolean;
    /** Citation Content Version Ids */
    citation_content_version_ids: string[];
    /** Created At */
    created_at: string;
    /** Id */
    id: string;
    /** Label Source */
    label_source: string;
    /** Request */
    request: string;
    /** Schema Version */
    schema_version: string;
    /** Sentiment */
    sentiment: "positive" | "negative" | "neutral" | "mixed" | "unknown";
    /** Stance */
    stance: "support" | "oppose" | "neutral" | "mixed" | "unknown";
    /** Target */
    target: string;
    /** Topic */
    topic: string;
  };

  type AnalysisRunInput = {
    /** Cutoff */
    cutoff: string;
    /** Expected Event Revision */
    expected_event_revision: number;
    /** Max Items */
    max_items?: number;
    /** Since */
    since: string;
    /** Until */
    until: string;
  };

  type AnalysisRunPage = {
    /** Items */
    items: AnalysisRunSummary[];
  };

  type AnalysisRunSummary = {
    /** Abstained Count */
    abstained_count: number;
    /** Created At */
    created_at: string;
    /** Event Id */
    event_id: string;
    /** Event Revision */
    event_revision: number;
    /** Id */
    id: string;
    /** Labeled Count */
    labeled_count: number;
    /** Manifest Sha256 */
    manifest_sha256: string;
    /** Sample Count */
    sample_count: number;
    /** Status */
    status: "pending" | "succeeded" | "stale";
  };

  type AnalysisRunView = {
    /** Abstained Count */
    abstained_count: number;
    /** Analyzer Id */
    analyzer_id: string;
    composition: AnalysisComposition;
    /** Created At */
    created_at: string;
    /** Cutoff */
    cutoff: string;
    /** Event Id */
    event_id: string;
    /** Event Revision */
    event_revision: number;
    /** Id */
    id: string;
    /** Input Tokens */
    input_tokens: number;
    /** Label Schema Version */
    label_schema_version: string;
    /** Labeled Count */
    labeled_count: number;
    /** Manifest Sha256 */
    manifest_sha256: string;
    /** Max Items */
    max_items: number;
    /** Method */
    method: string;
    /** Output Tokens */
    output_tokens: number;
    /** Pending Count */
    pending_count: number;
    /** Prompt Version */
    prompt_version: string;
    /** Recomputed From Manifest */
    recomputed_from_manifest?: boolean;
    /** Sample Count */
    sample_count: number;
    /** Samples */
    samples: AnalysisSampleView[];
    /** Sampling Policy Version */
    sampling_policy_version: string;
    /** Since */
    since: string;
    /** Status */
    status: "pending" | "succeeded" | "stale";
    /** Token Budget */
    token_budget: number;
    /** Until */
    until: string;
    /** Updated At */
    updated_at: string;
    /** Valid Labeled Count */
    valid_labeled_count: number;
    /** Viewpoints */
    viewpoints: AnalysisViewpoint[];
  };

  type AnalysisSampleView = {
    /** Contexts */
    contexts: AnalysisContextView[];
    /** Id */
    id: string;
    /** Kind */
    kind: "comment" | "reply";
    label: AnalysisLabelView | null;
    /** Ordering Origin */
    ordering_origin: string;
    /** Position */
    position: number;
    /** Published At */
    published_at: string;
    /** Root External Id */
    root_external_id: string;
    /** Selection Reason */
    selection_reason: string;
    /** Source */
    source: "x" | "bilibili" | "weibo" | "xiaohongshu" | "douyin" | "bluesky";
    /** Time Bucket Start */
    time_bucket_start: string;
  };

  type AnalysisViewpoint = {
    /** Citations */
    citations: AnalysisContextView[];
    /** Request */
    request: string;
    /** Sample Count */
    sample_count: number;
    /** Sentiment */
    sentiment: "positive" | "negative" | "neutral" | "mixed" | "unknown";
    /** Stance */
    stance: "support" | "oppose" | "neutral" | "mixed" | "unknown";
    /** Target */
    target: string;
    /** Topic */
    topic: string;
  };

  type BudgetSpec = {
    /** Content Purchase Cost */
    content_purchase_cost?: number;
    /** Daily Requests */
    daily_requests?: number;
  };

  type cancelJobParams = {
    identity: string;
  };

  type CollectionRunBatchView = {
    /** Items */
    items: CollectionRunView[];
    /** Replayed */
    replayed: boolean;
  };

  type CollectionRunPage = {
    /** Items */
    items: CollectionRunView[];
    /** Next Cursor */
    next_cursor: string | null;
  };

  type CollectionRunView = {
    /** Budget Day */
    budget_day: string;
    /** Bytes Count */
    bytes_count: number;
    /** Completed At */
    completed_at: string | null;
    /** Created At */
    created_at: string;
    /** Fencing Token */
    fencing_token: number;
    /** Id */
    id: string;
    /** Ingestion Mode */
    ingestion_mode: "live" | "backfill";
    /** Items Count */
    items_count: number;
    /** Job Id */
    job_id: string;
    /** Monitor Version Id */
    monitor_version_id: string;
    /** Operation */
    operation: "search_posts" | "fetch_post" | "list_comments" | "list_replies";
    /** Outcome */
    outcome: "ok" | "empty" | "partial" | "failed" | null;
    /** Pages Count */
    pages_count: number;
    /** Parent Run Id */
    parent_run_id: string | null;
    /** Request Value */
    request_value: string;
    /** Reserved Requests */
    reserved_requests: number;
    /** Retention Days */
    retention_days: number;
    /** Schedule Slot */
    schedule_slot: string | null;
    /** Source */
    source: "x" | "bilibili" | "weibo" | "xiaohongshu" | "douyin" | "bluesky";
    /** State */
    state: "queued" | "running" | "completed" | "failed" | "cancelled";
    /** Stop Reason */
    stop_reason: string | null;
    /** Trigger */
    trigger: "manual" | "scheduled";
    /** Window Since */
    window_since: string;
    /** Window Until */
    window_until: string;
  };

  type CommentCountQuestion = {
    /** Event Id */
    event_id: string;
    /** Kind */
    kind: string;
    /** Question */
    question: string;
    /** Since */
    since: string;
    /** Until */
    until: string;
  };

  type CommentTrackingView = {
    /** Content Id */
    content_id: string;
    /** Match Id */
    match_id: string;
    /** Replayed */
    replayed: boolean;
    /** Review State */
    review_state: string;
    /** Root Content Id */
    root_content_id: string;
    run: CollectionRunView;
  };

  type ContentDetailItem = {
    /** Canonical Url */
    canonical_url: string | null;
    /** External Id */
    external_id: string;
    /** First Seen At */
    first_seen_at: string;
    /** Id */
    id: string;
    /** Kind */
    kind: "post" | "comment" | "reply";
    /** Last Seen At */
    last_seen_at: string;
    /** Parent External Id */
    parent_external_id: string | null;
    /** Provider Namespace */
    provider_namespace: string;
    /** Published At */
    published_at: string;
    /** Relation Status */
    relation_status: "root" | "unresolved" | "resolved";
    /** Reply Count */
    reply_count: number | null;
    /** Root External Id */
    root_external_id: string;
    /** Source */
    source: "x" | "bilibili" | "weibo" | "xiaohongshu" | "douyin" | "bluesky";
    /** Text */
    text: string;
    /** Version */
    version: number;
  };

  type ContentDetailView = {
    /** Discussion */
    discussion: ContentDetailItem[];
    /** Next Cursor */
    next_cursor: string | null;
    parent: ContentDetailItem | null;
    root: ContentDetailItem | null;
    selected: ContentDetailItem;
  };

  type ContentWithdrawalInput = {
    /** Reason */
    reason: "deleted" | "purpose_revoked";
  };

  type ContentWithdrawalView = {
    /** Affected Knowledge Entries */
    affected_knowledge_entries: number;
    /** Id */
    id: string;
    /** Visibility */
    visibility: "unavailable" | "deleted";
  };

  type ControlledCommentStatistics = {
    /** By Kind */
    by_kind: Record<string, any>;
    /** By Source */
    by_source: Record<string, any>;
    /** Event Id */
    event_id: string;
    /** Event Revision */
    event_revision: number;
    /** Rule Version */
    rule_version?: string;
    /** Since */
    since: string;
    /** Total */
    total: number;
    /** Until */
    until: string;
  };

  type createCollectionRunParams = {
    identity: string;
  };

  type createEventAnalysisRunParams = {
    identity: string;
  };

  type createEventTrendAlertRuleParams = {
    identity: string;
  };

  type DiagnosticInput = {
    /** Kind */
    kind: string;
  };

  type ErrorView = {
    /** Code */
    code: string;
    /** Request Id */
    request_id: string;
  };

  type evaluateEventTrendAlertsParams = {
    identity: string;
  };

  type EventInput = {
    /** Summary */
    summary?: string;
    /** Title */
    title: string;
  };

  type EventMemberInput = {
    /** Content Id */
    content_id: string;
  };

  type EventMemberView = {
    /** Added At */
    added_at: string;
    /** Canonical Url */
    canonical_url: string | null;
    /** Content Id */
    content_id: string;
    /** External Id */
    external_id: string;
    /** Kind */
    kind: "post" | "comment" | "reply";
    /** Source */
    source: "x" | "bilibili" | "weibo" | "xiaohongshu" | "douyin" | "bluesky";
  };

  type EventMergeInput = {
    /** Expected Source Revision */
    expected_source_revision: number;
    /** Expected Target Revision */
    expected_target_revision: number;
    /** Source Event Id */
    source_event_id: string;
  };

  type EventPage = {
    /** Items */
    items: EventView[];
    /** Next Cursor */
    next_cursor: string | null;
  };

  type EventRevisionView = {
    /** Change Type */
    change_type:
      | "create"
      | "add_member"
      | "remove_member"
      | "merge_in"
      | "merge_out"
      | "split_in"
      | "split_out";
    /** Created At */
    created_at: string;
    /** Related Event Id */
    related_event_id: string | null;
    /** Revision */
    revision: number;
    snapshot: EventSnapshot;
  };

  type EventSnapshot = {
    /** Member Content Ids */
    member_content_ids: string[];
    /** Status */
    status: "active" | "archived";
    /** Summary */
    summary: string;
    /** Title */
    title: string;
  };

  type EventSourceTrend = {
    /** Buckets */
    buckets: EventTrendBucket[];
    /** Source */
    source: "x" | "bilibili" | "weibo" | "xiaohongshu" | "douyin" | "bluesky";
  };

  type EventSplitInput = {
    /** Content Ids */
    content_ids: string[];
    /** Expected Revision */
    expected_revision: number;
    /** Summary */
    summary?: string;
    /** Title */
    title: string;
  };

  type EventTrendBucket = {
    /** Backfill Run Count */
    backfill_run_count: number;
    /** Coverage Status */
    coverage_status: "comparable" | "interrupted" | "missing";
    /** Ends At */
    ends_at: string;
    /** Excluded Backfill Items */
    excluded_backfill_items: number;
    /** Excluded Backfill Observations */
    excluded_backfill_observations: number;
    /** Interruption Reasons */
    interruption_reasons: (
      | "policy_changed"
      | "run_failed"
      | "run_partial"
      | "run_incomplete"
      | "no_live_coverage"
    )[];
    /** Live Run Count */
    live_run_count: number;
    /** New Discussions */
    new_discussions: number;
    /** New Posts */
    new_posts: number;
    /** Observed Reply Delta */
    observed_reply_delta: number;
    /** Policy Versions */
    policy_versions: string[];
    /** Starts At */
    starts_at: string;
  };

  type EventTrendView = {
    /** Bucket Hours */
    bucket_hours: 1 | 6 | 24;
    /** Event Id */
    event_id: string;
    /** Metric Version */
    metric_version?: string;
    /** Since */
    since: string;
    /** Sources */
    sources: EventSourceTrend[];
    /** Timezone */
    timezone?: string;
    /** Until */
    until: string;
  };

  type EventView = {
    /** Created At */
    created_at: string;
    /** Current Revision */
    current_revision: number;
    /** Id */
    id: string;
    /** Members */
    members: EventMemberView[];
    /** Status */
    status: "active" | "archived";
    /** Summary */
    summary: string;
    /** Title */
    title: string;
    /** Updated At */
    updated_at: string;
  };

  type EvidenceQuestion = {
    /** Event Id */
    event_id?: string | null;
    /** Kind */
    kind: string;
    /** Limit */
    limit?: number;
    /** Mode */
    mode?: "exact_substring" | "semantic";
    /** Question */
    question: string;
  };

  type getAnalysisRunParams = {
    identity: string;
  };

  type getCollectionRunParams = {
    identity: string;
  };

  type getContentDetailParams = {
    identity: string;
    limit?: number;
    cursor?: string | null;
  };

  type getEventParams = {
    identity: string;
  };

  type getEventTrendsParams = {
    identity: string;
    since: string;
    until: string;
    bucket_hours?: TrendBucketHours;
  };

  type getJobParams = {
    identity: string;
  };

  type getKnowledgeEntryParams = {
    identity: string;
  };

  type HealthView = {
    /** Code */
    code?: string | null;
    /** Scope */
    scope?: string | null;
    /** Status */
    status: string;
  };

  type InboxItem = {
    /** Canonical Url */
    canonical_url: string | null;
    /** External Id */
    external_id: string;
    /** First Seen At */
    first_seen_at: string;
    /** Id */
    id: string;
    /** Kind */
    kind: "post" | "comment" | "reply";
    /** Last Seen At */
    last_seen_at: string;
    /** Matches */
    matches: MonitorMatchView[];
    /** Parent External Id */
    parent_external_id: string | null;
    /** Provider Namespace */
    provider_namespace: string;
    /** Published At */
    published_at: string;
    /** Relation Status */
    relation_status: "root" | "unresolved" | "resolved";
    /** Reply Count */
    reply_count: number | null;
    /** Root External Id */
    root_external_id: string;
    /** Source */
    source: "x" | "bilibili" | "weibo" | "xiaohongshu" | "douyin" | "bluesky";
    /** Text */
    text: string;
    /** Version */
    version: number;
  };

  type InboxPage = {
    /** Items */
    items: InboxItem[];
    /** Next Cursor */
    next_cursor: string | null;
  };

  type indexKnowledgeEntryParams = {
    identity: string;
  };

  type JobPage = {
    /** Items */
    items: JobView[];
    /** Next Cursor */
    next_cursor: string | null;
  };

  type JobView = {
    /** Attempts */
    attempts: number;
    /** Deadline */
    deadline: string;
    /** Epoch */
    epoch: number;
    /** Id */
    id: string;
    /** Kind */
    kind: "verify_pipeline" | "collect_page";
    /** Status */
    status: "queued" | "running" | "succeeded" | "failed" | "cancelled";
  };

  type KnowledgeAnswer = {
    /** Answer */
    answer: string;
    /** Citations */
    citations?: KnowledgeCitationView[];
    /** Kind */
    kind: "comment_count" | "evidence";
    /** Knowledge Entry Ids */
    knowledge_entry_ids?: string[];
    /** Method */
    method: "controlled_statistics_v1" | "deterministic_retrieval_v1";
    /** Question */
    question: string;
    statistics?: ControlledCommentStatistics | null;
    /** Status */
    status: "answered" | "unknown";
    /** Unknown Reason */
    unknown_reason?: string | null;
  };

  type KnowledgeCitationView = {
    /** Available */
    available: boolean;
    /** Canonical Url */
    canonical_url: string | null;
    /** Content Version Id */
    content_version_id: string;
    /** Text */
    text: string | null;
    /** Text Sha256 */
    text_sha256: string;
  };

  type KnowledgeEntryView = {
    /** Analysis Manifest Sha256 */
    analysis_manifest_sha256: string;
    /** Body */
    body: string;
    /** Citations */
    citations: KnowledgeCitationView[];
    /** Created At */
    created_at: string;
    /** Entry Type */
    entry_type: string;
    /** Event Id */
    event_id: string;
    /** Id */
    id: string;
    /** Semantic Index State */
    semantic_index_state: "pending" | "ready" | "failed" | "stale" | "deleted";
    /** Similarity */
    similarity?: number | null;
    /** Source Analysis Run Id */
    source_analysis_run_id: string;
    /** Stale */
    stale: boolean;
    /** Title */
    title: string;
    /** Updated At */
    updated_at: string;
    /** Version */
    version: number;
  };

  type KnowledgePage = {
    /** Items */
    items: KnowledgeEntryView[];
    /** Query */
    query: string;
    /** Query Mode */
    query_mode?: "exact_substring" | "semantic";
  };

  type labelAnalysisSampleParams = {
    identity: string;
    sample_id: string;
  };

  type listCollectionRunsParams = {
    limit?: number;
    cursor?: string | null;
  };

  type listEventAnalysisRunsParams = {
    identity: string;
    limit?: number;
  };

  type listEventRevisionsParams = {
    identity: string;
  };

  type listEventsParams = {
    limit?: number;
    cursor?: string | null;
  };

  type listEventTrendAlertRulesParams = {
    identity: string;
  };

  type listInboxContentsParams = {
    limit?: number;
    cursor?: string | null;
    monitor_id?: string | null;
    source?:
      | "x"
      | "bilibili"
      | "weibo"
      | "xiaohongshu"
      | "douyin"
      | "bluesky"
      | null;
    review_state?: "new" | "ignored" | "following" | null;
    discovered_since?: string | null;
  };

  type listJobsParams = {
    limit?: number;
    cursor?: string | null;
  };

  type listMonitorsParams = {
    limit?: number;
    cursor?: string | null;
  };

  type listNotificationsParams = {
    limit?: number;
    cursor?: string | null;
    unread_only?: boolean;
  };

  type LoginInput = {
    /** Password */
    password: string;
    /** Username */
    username: string;
  };

  type markNotificationReadParams = {
    identity: string;
  };

  type mergeEventParams = {
    identity: string;
  };

  type MonitorInput = {
    budget?: BudgetSpec;
    query_spec: QuerySpec;
    schedule?: ScheduleSpec;
    /** Source Ids */
    source_ids: (
      | "x"
      | "bilibili"
      | "weibo"
      | "xiaohongshu"
      | "douyin"
      | "bluesky"
    )[];
    /** Title */
    title: string;
  };

  type MonitorMatchReviewInput = {
    /** Review State */
    review_state: "new" | "ignored";
  };

  type MonitorMatchView = {
    /** First Seen At */
    first_seen_at: string;
    /** Id */
    id: string;
    /** Last Seen At */
    last_seen_at: string;
    /** Match Reason */
    match_reason: string[];
    /** Monitor Id */
    monitor_id: string;
    /** Monitor Title */
    monitor_title: string;
    /** Monitor Version */
    monitor_version: number;
    /** Monitor Version Id */
    monitor_version_id: string;
    /** Relevance Status */
    relevance_status: "pending" | "accepted" | "rejected" | "needs_review";
    /** Review State */
    review_state: "new" | "ignored" | "following";
  };

  type MonitorPage = {
    /** Items */
    items: MonitorView[];
    /** Next Cursor */
    next_cursor: string | null;
  };

  type MonitorRunRequest = {
    /** Expected Version */
    expected_version: number;
    /** Idempotency Key */
    idempotency_key: string;
  };

  type MonitorStateChange = {
    /** Expected Version */
    expected_version: number;
  };

  type MonitorUpdate = {
    budget?: BudgetSpec;
    /** Expected Version */
    expected_version: number;
    query_spec: QuerySpec;
    schedule?: ScheduleSpec;
    /** Source Ids */
    source_ids: (
      | "x"
      | "bilibili"
      | "weibo"
      | "xiaohongshu"
      | "douyin"
      | "bluesky"
    )[];
    /** Title */
    title: string;
  };

  type MonitorView = {
    budget: BudgetSpec;
    /** Created At */
    created_at: string;
    /** Current Version */
    current_version: number;
    /** Id */
    id: string;
    query_spec: QuerySpec;
    schedule: ScheduleSpec;
    /** Source Ids */
    source_ids: (
      | "x"
      | "bilibili"
      | "weibo"
      | "xiaohongshu"
      | "douyin"
      | "bluesky"
    )[];
    /** State */
    state: "draft" | "active" | "paused";
    /** Title */
    title: string;
    /** Updated At */
    updated_at: string;
  };

  type NotificationPage = {
    /** Items */
    items: NotificationView[];
    /** Next Cursor */
    next_cursor: string | null;
    /** Unread Count */
    unread_count: number;
  };

  type NotificationView = {
    /** Change Id */
    change_id: string | null;
    /** Created At */
    created_at: string;
    /** Event Id */
    event_id: string;
    /** Id */
    id: string;
    /** Kind */
    kind:
      | "event_member_added"
      | "event_member_removed"
      | "event_merged_in"
      | "event_merged_out"
      | "event_split_in"
      | "event_split_out"
      | "trend_threshold_reached";
    /** Message */
    message: string;
    /** Read At */
    read_at: string | null;
    /** Rule Version */
    rule_version: number;
    /** Trend Occurrence Id */
    trend_occurrence_id: string | null;
  };

  type pauseMonitorParams = {
    identity: string;
  };

  type Principal = {
    /** Username */
    username: string;
  };

  type publishAnalysisKnowledgeParams = {
    identity: string;
  };

  type QueryPreview = {
    /** Content Purchase Cost */
    content_purchase_cost?: number;
    /** Estimated Requests */
    estimated_requests: number;
    /** Network Accessed */
    network_accessed?: boolean;
    /** Since */
    since: string;
    /** Sources */
    sources: SourceQueryPreview[];
    /** Until */
    until: string;
  };

  type QueryPreviewInput = {
    query_spec: QuerySpec;
    /** Since */
    since: string;
    /** Source Ids */
    source_ids: (
      | "x"
      | "bilibili"
      | "weibo"
      | "xiaohongshu"
      | "douyin"
      | "bluesky"
    )[];
    /** Until */
    until: string;
  };

  type QueryRuleExecution = {
    /** Mode */
    mode: "native" | "local_filter" | "unsupported";
    /** Rule */
    rule: "include_any" | "include_all" | "exclude" | "aliases";
  };

  type QuerySpec = {
    /** Aliases */
    aliases?: string[];
    /** Exclude */
    exclude?: string[];
    /** Include All */
    include_all?: string[];
    /** Include Any */
    include_any: string[];
  };

  type recomputeAnalysisRunParams = {
    identity: string;
  };

  type removeEventMemberParams = {
    identity: string;
    content_id: string;
  };

  type reviewMonitorMatchParams = {
    identity: string;
  };

  type ScheduleSpec = {
    /** Interval Minutes */
    interval_minutes?: number;
    /** Retention Days */
    retention_days?: number;
  };

  type searchKnowledgeParams = {
    query?: string | null;
    event_id?: string | null;
    mode?: "exact_substring" | "semantic";
    limit?: number;
  };

  type SourceOperationCapability = {
    /** Access Mode */
    access_mode:
      | "public_web"
      | "public_api"
      | "official_paid_api"
      | "authorized_session";
    /** Content Purchase Cost */
    content_purchase_cost?: number;
    /** Eligible For Collection */
    eligible_for_collection: boolean;
    /** Evidence Ref */
    evidence_ref: string;
    /** Note */
    note: string;
    /** Operation */
    operation: "search_posts" | "fetch_post" | "list_comments" | "list_replies";
    /** Pipeline */
    pipeline: "not_connected" | "connected" | "degraded" | "paused";
    /** Requires Operations */
    requires_operations: (
      | "search_posts"
      | "fetch_post"
      | "list_comments"
      | "list_replies"
    )[];
    /** Rights */
    rights: "unknown" | "allowed" | "denied";
    /** Support */
    support: "unknown" | "supported" | "unsupported" | "authorization_required";
    /** Verified At */
    verified_at: string;
  };

  type SourceQueryPreview = {
    /** Content Purchase Cost */
    content_purchase_cost?: number;
    /** Estimated Requests */
    estimated_requests: number;
    /** Operation */
    operation?: string;
    /** Pipeline Connected */
    pipeline_connected: boolean;
    /** Queries */
    queries: string[];
    /** Rules */
    rules: QueryRuleExecution[];
    /** Source */
    source: "x" | "bilibili" | "weibo" | "xiaohongshu" | "douyin" | "bluesky";
    /** Support */
    support: "unknown" | "supported" | "unsupported" | "authorization_required";
  };

  type SourceView = {
    /** Id */
    id: "x" | "bilibili" | "weibo" | "xiaohongshu" | "douyin" | "bluesky";
    /** Operations */
    operations: SourceOperationCapability[];
    /** Roles */
    roles: ("discovery" | "comments" | "supplement")[];
  };

  type splitEventParams = {
    identity: string;
  };

  type startCommentTrackingParams = {
    identity: string;
  };

  type TrendAlertEvaluationView = {
    /** Created Notifications */
    created_notifications: number;
    /** Evaluated Rules */
    evaluated_rules: number;
  };

  type TrendAlertRuleInput = {
    /** Bucket Hours */
    bucket_hours: 1 | 6 | 24;
    /** Metric */
    metric: "new_posts" | "new_discussions" | "observed_reply_delta";
    /** Source */
    source: "x" | "bilibili" | "weibo" | "xiaohongshu" | "douyin" | "bluesky";
    /** Threshold Count */
    threshold_count: number;
  };

  type TrendAlertRuleUpdate = {
    /** Enabled */
    enabled: boolean;
    /** Expected Version */
    expected_version: number;
    /** Threshold Count */
    threshold_count: number;
  };

  type TrendAlertRuleView = {
    /** Bucket Hours */
    bucket_hours: 1 | 6 | 24;
    /** Created At */
    created_at: string;
    /** Enabled */
    enabled: boolean;
    /** Event Id */
    event_id: string;
    /** Id */
    id: string;
    /** Metric */
    metric: "new_posts" | "new_discussions" | "observed_reply_delta";
    /** Source */
    source: "x" | "bilibili" | "weibo" | "xiaohongshu" | "douyin" | "bluesky";
    /** Threshold Count */
    threshold_count: number;
    /** Updated At */
    updated_at: string;
    /** Version */
    version: number;
  };

  type TrendBucketHours = 1 | 6 | 24;

  type updateEventTrendAlertRuleParams = {
    identity: string;
    rule_id: string;
  };

  type updateMonitorParams = {
    identity: string;
  };

  type withdrawContentParams = {
    identity: string;
  };
}
