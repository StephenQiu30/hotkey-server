declare namespace HotKeyAPI {
  type archiveMonitorTopicParams = {
    topic_id: string;
  };

  type cancelCollectionJobParams = {
    job_id: string;
  };

  type cloneMonitorTopicParams = {
    topic_id: string;
  };

  type CollectionJobInput = {
    /** Operation Id */
    operation_id: string;
    kind: CollectionJobKind;
    observation: CollectionJobObservationInput;
    /** Scheduled For At */
    scheduled_for_at?: string | null;
    /** Scope */
    scope: Record<string, any>;
  };

  type CollectionJobKind = "monitor.collect";

  type CollectionJobObservationInput = {
    /** Configuration Ref */
    configuration_ref: string;
    /** Configuration Version */
    configuration_version: number;
    /** Source Key */
    source_key?: string | null;
    source_capability?: SourceCapability | null;
  };

  type ErrorView = {
    /** Code */
    code: string;
    /** Message */
    message: string;
    /** Request Id */
    request_id: string;
    /** Details */
    details?: ValidationErrorItem[] | null;
  };

  type getCollectionJobParams = {
    job_id: string;
  };

  type getMonitorTopicParams = {
    topic_id: string;
  };

  type HealthView = {
    /** Status */
    status: "ok" | "ready";
  };

  type IdentityCredentialsInput = {
    /** Username */
    username: string;
    /** Password */
    password: string;
  };

  type IdentitySessionView = {
    user: IdentityUserView;
    /** Expires At */
    expires_at: string;
  };

  type IdentityUserView = {
    /** Id */
    id: string;
    /** Username */
    username: string;
  };

  type IdentityWorkspaceView = {
    owner: IdentityUserView;
  };

  type JobAcceptanceStatus = "queued";

  type JobAcceptedView = {
    /** Job Id */
    job_id: string;
    status: JobAcceptanceStatus;
  };

  type JobCancellationView = {
    /** Requested At */
    requested_at: string;
    /** Deadline At */
    deadline_at: string | null;
    /** Timed Out */
    timed_out: boolean;
  };

  type JobControlStatus =
    | "queued"
    | "running"
    | "cancelling"
    | "succeeded"
    | "partially_succeeded"
    | "failed"
    | "cancelled";

  type JobFailureCategory =
    | "transient"
    | "rate_limited"
    | "authentication_required"
    | "permission_denied"
    | "invalid_response"
    | "parse_error"
    | "invalid_input"
    | "configuration_unavailable";

  type JobFailureView = {
    /** Error Code */
    error_code: string;
    category: JobFailureCategory;
    /** Occurred At */
    occurred_at: string;
    /** Next Action */
    next_action: string;
    /** Manual Retry Allowed */
    manual_retry_allowed: boolean;
  };

  type JobObservationContext = {
    /** Configuration Ref */
    configuration_ref: string;
    /** Configuration Version */
    configuration_version: number;
    /** Source Key */
    source_key?: string | null;
    source_capability?: SourceCapability | null;
  };

  type JobProgressView = {
    stage: JobStage | null;
    /** Requests Sent */
    requests_sent: number;
    /** Items Saved */
    items_saved: number;
    /** Updated At */
    updated_at: string | null;
  };

  type JobScopeValue = Record<string, any>;

  type JobStage = "request" | "parse" | "save" | "analysis";

  type JobStatusView = {
    /** Id */
    id: string;
    /** Operation Id */
    operation_id: string;
    /** Kind */
    kind: string;
    observation: JobObservationContext;
    status: JobControlStatus;
    progress: JobProgressView;
    cancellation: JobCancellationView | null;
    failure: JobFailureView | null;
    /** Retry Count */
    retry_count: number;
    /** Next Run At */
    next_run_at: string | null;
    /** Scheduled For At */
    scheduled_for_at: string | null;
    /** Started At */
    started_at: string | null;
    /** Completed At */
    completed_at: string | null;
    /** Created At */
    created_at: string;
  };

  type KeywordInput = string;

  type listMonitorTopicsParams = {
    include_archived?: boolean;
    cursor?: string | null;
    limit?: number;
  };

  type MonitorRuleSetView = {
    /** Match Any */
    match_any: string[];
    /** Match All */
    match_all: string[];
    /** Exclude */
    exclude: string[];
  };

  type MonitorTopicCreateInput = {
    /** Match Any */
    match_any: KeywordInput[];
    /** Match All */
    match_all: KeywordInput[];
    /** Exclude */
    exclude: KeywordInput[];
    /** Name */
    name: string;
  };

  type MonitorTopicReadinessStatus =
    "pending_source_selection" | "pending_source_readiness" | "ready";

  type MonitorTopicStatus = "paused" | "active" | "archived";

  type MonitorTopicUpdateInput = {
    /** Match Any */
    match_any: KeywordInput[];
    /** Match All */
    match_all: KeywordInput[];
    /** Exclude */
    exclude: KeywordInput[];
    /** Name */
    name: string;
    /** Expected Version */
    expected_version: number;
  };

  type MonitorTopicView = {
    /** Id */
    id: string;
    /** Name */
    name: string;
    status: MonitorTopicStatus;
    readiness_status: MonitorTopicReadinessStatus;
    /** Current Version */
    current_version: number;
    rules: MonitorRuleSetView;
    /** Created At */
    created_at: string;
    /** Updated At */
    updated_at: string;
  };

  type PageViewMonitorTopicView_ = {
    /** Items */
    items: MonitorTopicView[];
    /** Next Cursor */
    next_cursor: string | null;
  };

  type pauseMonitorTopicParams = {
    topic_id: string;
  };

  type resumeMonitorTopicParams = {
    topic_id: string;
  };

  type retryCollectionJobParams = {
    job_id: string;
  };

  type SourceCapability = "search" | "author_posts" | "comments" | "replies";

  type updateMonitorTopicParams = {
    topic_id: string;
  };

  type ValidationErrorItem = {
    /** Location */
    location: (string | number)[];
    /** Message */
    message: string;
    /** Type */
    type: string;
  };
}
