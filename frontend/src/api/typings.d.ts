declare namespace API {
  type activateMonitorParams = {
    identity: string;
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

  type CollectionRunPage = {
    /** Items */
    items: CollectionRunView[];
    /** Next Cursor */
    next_cursor: string | null;
  };

  type CollectionRunRequest = {
    /** Expected Version */
    expected_version: number;
    /** Operation */
    operation?: string;
    /** Policy Version */
    policy_version: string;
    /** Request Value */
    request_value: string;
    /** Retention Days */
    retention_days: number;
    /** Since */
    since: string;
    /** Source */
    source: "x" | "bilibili" | "weibo" | "xiaohongshu" | "douyin" | "bluesky";
    /** Until */
    until: string;
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

  type createCollectionRunParams = {
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

  type getCollectionRunParams = {
    identity: string;
  };

  type getJobParams = {
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
    /** Monitor Titles */
    monitor_titles: string[];
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

  type listCollectionRunsParams = {
    limit?: number;
    cursor?: string | null;
  };

  type listInboxContentsParams = {
    limit?: number;
    cursor?: string | null;
  };

  type listJobsParams = {
    limit?: number;
    cursor?: string | null;
  };

  type listMonitorsParams = {
    limit?: number;
    cursor?: string | null;
  };

  type LoginInput = {
    /** Password */
    password: string;
    /** Username */
    username: string;
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

  type MonitorPage = {
    /** Items */
    items: MonitorView[];
    /** Next Cursor */
    next_cursor: string | null;
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

  type pauseMonitorParams = {
    identity: string;
  };

  type Principal = {
    /** Username */
    username: string;
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

  type ScheduleSpec = {
    /** Interval Minutes */
    interval_minutes?: number;
    /** Retention Days */
    retention_days?: number;
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

  type updateMonitorParams = {
    identity: string;
  };
}
