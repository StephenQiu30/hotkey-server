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
    /** Kind */
    kind: "monitor.collect";
    observation: CollectionJobObservationInput;
    /** Scheduled For At */
    scheduled_for_at?: string | null;
    /** Scope */
    scope: Record<string, any>;
  };

  type CollectionJobObservationInput = {
    /** Configuration Ref */
    configuration_ref: string;
    /** Configuration Version */
    configuration_version: number;
    /** Source Key */
    source_key?: string | null;
    source_capability?: SocialSourceCapability | null;
  };

  type ContentDiscoveryView = {
    /** Job Id */
    job_id: string;
    /** Configuration Ref */
    configuration_ref: string;
    /** Configuration Version */
    configuration_version: number;
    /** First Observed At */
    first_observed_at: string;
  };

  type ContentMetricView = {
    /** Like Count */
    like_count: number | null;
    /** Comment Count */
    comment_count: number | null;
    /** Repost Count */
    repost_count: number | null;
    /** View Count */
    view_count: number | null;
    /** Play Count */
    play_count: number | null;
    /** Danmaku Count */
    danmaku_count: number | null;
  };

  type ContentObservationView = {
    /** Id */
    id: string;
    /** Observed At */
    observed_at: string;
    /** Received At */
    received_at: string;
    /** Published At */
    published_at: string | null;
    /** Published At Fractional Digits */
    published_at_fractional_digits: number | null;
    /** Canonical Url */
    canonical_url: string | null;
    /** Final Url */
    final_url: string | null;
    /** Author External Id */
    author_external_id: string | null;
    metrics: ContentMetricView;
    content_version: ContentVersionView | null;
  };

  type ContentRecordDetailView = {
    /** Id */
    id: string;
    /** Source Key */
    source_key: string;
    /** Object Type */
    object_type: "post" | "comment" | "webpage";
    /** Native Scope */
    native_scope: string | null;
    /** External Id */
    external_id: string;
    latest_observation: ContentObservationView;
    current_visibility: ContentVisibilityView | null;
    /** Discovery Count */
    discovery_count: number;
    /** Discoveries */
    discoveries: ContentDiscoveryView[];
    /** Version History */
    version_history: ContentVersionHistoryView[];
    /** Visibility History */
    visibility_history: ContentVisibilityView[];
  };

  type ContentRecordSummaryView = {
    /** Id */
    id: string;
    /** Source Key */
    source_key: string;
    /** Object Type */
    object_type: "post" | "comment" | "webpage";
    /** Native Scope */
    native_scope: string | null;
    /** External Id */
    external_id: string;
    latest_observation: ContentObservationView;
    current_visibility: ContentVisibilityView | null;
    /** Discovery Count */
    discovery_count: number;
  };

  type ContentRelationType = "quote" | "repost";

  type ContentTextOrigin = "source" | "machine_extracted";

  type ContentTextScope = "full" | "summary" | "truncated" | "media_only";

  type ContentTruncationReason = "source_limit" | "collector_limit";

  type ContentVersionHistoryView = {
    content_version: ContentVersionView;
    /** First Observed At */
    first_observed_at: string;
    /** Last Observed At */
    last_observed_at: string;
    /** Observation Count */
    observation_count: number;
  };

  type ContentVersionRelationView = {
    relation_type: ContentRelationType;
    /** Target Native Scope */
    target_native_scope: string | null;
    /** Target External Id */
    target_external_id: string;
    /** Target Author External Id */
    target_author_external_id: string | null;
    /** Target Content Id */
    target_content_id: string | null;
  };

  type ContentVersionView = {
    /** Id */
    id: string;
    text_scope: ContentTextScope;
    text_origin: ContentTextOrigin;
    /** Text Origin Ref */
    text_origin_ref: string | null;
    /** Title */
    title: string | null;
    /** Body */
    body: string | null;
    truncation_reason: ContentTruncationReason | null;
    /** Relations */
    relations: ContentVersionRelationView[];
  };

  type ContentVisibilityBasis =
    | "content_returned"
    | "source_tombstone"
    | "http_gone"
    | "access_denied"
    | "authentication_required"
    | "not_found"
    | "timeout"
    | "rate_limited"
    | "upstream_error"
    | "protocol_error";

  type ContentVisibilityStatus =
    "visible" | "deleted" | "restricted" | "transient_failure" | "unknown";

  type ContentVisibilityView = {
    /** Id */
    id: string;
    /** Observed At */
    observed_at: string;
    /** Received At */
    received_at: string;
    status: ContentVisibilityStatus;
    basis: ContentVisibilityBasis;
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

  type getContentRecordParams = {
    content_id: string;
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
    /** Result Content Id */
    result_content_id: string | null;
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

  type listContentRecordsParams = {
    cursor?: string | null;
    limit?: number;
  };

  type listMonitorTopicsParams = {
    include_archived?: boolean;
    cursor?: string | null;
    limit?: number;
  };

  type MonitorExpansionPreviewView = {
    /** Local Alias External Queries */
    local_alias_external_queries: number;
    /** Local Alias Budget Units */
    local_alias_budget_units: number;
    /** Upstream Status */
    upstream_status: string;
    /** Upstream External Queries */
    upstream_external_queries: null;
    /** Upstream Budget Units */
    upstream_budget_units: null;
  };

  type MonitorRulePreviewSampleView = {
    /** Sample Index */
    sample_index: number;
    /** Matched */
    matched: boolean;
    /** Excluded By */
    excluded_by: string[];
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

  type MonitorTopicPreviewInput = {
    /** Match Any */
    match_any: KeywordInput[];
    /** Match All */
    match_all: KeywordInput[];
    /** Exclude */
    exclude: KeywordInput[];
    /** Sample Titles */
    sample_titles: PreviewSampleInput[];
  };

  type MonitorTopicPreviewView = {
    rules: MonitorRuleSetView;
    /** Samples */
    samples: MonitorRulePreviewSampleView[];
    expansion: MonitorExpansionPreviewView;
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

  type PageViewContentRecordSummaryView_ = {
    /** Items */
    items: ContentRecordSummaryView[];
    /** Next Cursor */
    next_cursor: string | null;
  };

  type PageViewMonitorTopicView_ = {
    /** Items */
    items: MonitorTopicView[];
    /** Next Cursor */
    next_cursor: string | null;
  };

  type PageViewSourcePlatformView_ = {
    /** Items */
    items: SourcePlatformView[];
    /** Next Cursor */
    next_cursor: string | null;
  };

  type pauseMonitorTopicParams = {
    topic_id: string;
  };

  type PreviewSampleInput = string;

  type resumeMonitorTopicParams = {
    topic_id: string;
  };

  type retryCollectionJobParams = {
    job_id: string;
  };

  type SocialSourceCapability =
    "search" | "author_posts" | "comments" | "replies";

  type SourceCapability =
    "search" | "author_posts" | "comments" | "replies" | "page_content";

  type SourceCapabilityStatus =
    | "unconfigured"
    | "pending_verification"
    | "available"
    | "authentication_required"
    | "restricted"
    | "disabled";

  type SourceCapabilityView = {
    capability: SourceCapability;
    /** Display Name */
    display_name: string;
    manual: SourceEntryPointView;
    scheduled: SourceEntryPointView;
  };

  type SourceConnectionStatus = "active" | "disabled";

  type SourceConnectionUpdateInput = {
    /** Expected Version */
    expected_version: number;
    status: SourceConnectionStatus;
    /** Allowed Hosts */
    allowed_hosts?: string[];
  };

  type SourceConnectionView = {
    /** Id */
    id: string;
    /** Source Key */
    source_key: string;
    status: SourceConnectionStatus;
    /** Version */
    version: number;
    /** Allowed Hosts */
    allowed_hosts: string[];
    /** Updated At */
    updated_at: string;
  };

  type SourceEntryPointView = {
    status: SourceCapabilityStatus;
    /** Last Checked At */
    last_checked_at: string | null;
    /** Last Persisted Success At */
    last_persisted_success_at: string | null;
    stop_reason: SourceStopReason | null;
    /** Next Action */
    next_action: string;
  };

  type SourcePlatformStatus =
    | "unconfigured"
    | "pending_verification"
    | "available"
    | "authentication_required"
    | "restricted"
    | "disabled"
    | "partial";

  type SourcePlatformView = {
    /** Source Key */
    source_key: string;
    /** Display Name */
    display_name: string;
    rollout_role: SourceRolloutRole;
    status: SourcePlatformStatus;
    /** Connection Version */
    connection_version: number | null;
    /** Has Credentials */
    has_credentials: boolean;
    /** Connection Id */
    connection_id: string | null;
    connection_status: SourceConnectionStatus | null;
    /** Credential Configured */
    credential_configured: boolean;
    /** Credential Update Available */
    credential_update_available: boolean;
    /** Allowed Hosts */
    allowed_hosts: string[];
    /** Capabilities */
    capabilities: SourceCapabilityView[];
  };

  type SourceRolloutRole = "required" | "candidate";

  type SourceStopReason =
    | "end_of_results"
    | "source_empty"
    | "rate_limited"
    | "authentication_required"
    | "access_denied"
    | "not_found"
    | "unsupported"
    | "cancelled"
    | "budget_exhausted"
    | "upstream_error"
    | "protocol_error";

  type updateMonitorTopicParams = {
    topic_id: string;
  };

  type updateSourceConnectionParams = {
    source_key: string;
  };

  type ValidationErrorItem = {
    /** Location */
    location: (string | number)[];
    /** Message */
    message: string;
    /** Type */
    type: string;
  };

  type WebPageCollectionJobInput = {
    /** Operation Id */
    operation_id: string;
    /** Kind */
    kind: "webpage.collect";
    /** Url */
    url: string;
  };
}
