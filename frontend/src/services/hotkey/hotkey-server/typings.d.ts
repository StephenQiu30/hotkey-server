declare namespace HotKeyAPI {
  type AICandidateRequest = {
    /** Gin must not apply required directly to this nullable wrapper: both an
explicit JSON null and a positive integer are valid. The application
helper below enforces presence/value at runtime; validate keeps Swagger's
required property without making explicit null impossible to bind. */
    expected_draft_version: number;
    expected_monitor_version: number;
    operator: string;
    priority?: number;
    rule_type: string;
    value: string;
    weight?: number;
  };

  type AlertActionRequest = {
    expected_version: number;
    reason_code: string;
  };

  type AlertDetailResponse = {
    audits?: AlertStateAuditResponse[];
    occurrences?: AlertOccurrenceResponse[];
    thread?: AlertThreadResponse;
  };

  type AlertOccurrenceResponse = {
    event_update_id?: number;
    final_score?: number;
    id?: number;
    reason_codes?: string[];
    severity?: string;
    threshold?: number;
    triggered_at?: string;
  };

  type AlertPageResponse = {
    items?: AlertThreadResponse[];
    next_cursor?: string;
  };

  type AlertResultHttpAlertDetailResponse = {
    code?: number;
    data?: AlertDetailResponse;
    message?: string;
  };

  type AlertResultHttpAlertPageResponse = {
    code?: number;
    data?: AlertPageResponse;
    message?: string;
  };

  type AlertResultHttpAlertThreadResponse = {
    code?: number;
    data?: AlertThreadResponse;
    message?: string;
  };

  type AlertResultInternalModulesAlertTransportHttpEmptyResponse = {
    code?: number;
    data?: EmptyResponse;
    message?: string;
  };

  type AlertStateAuditResponse = {
    actor_type?: string;
    actor_user_id?: number;
    created_at?: string;
    expected_version?: number;
    from_state?: string;
    id?: number;
    reason_code?: string;
    to_state?: string;
  };

  type AlertThreadResponse = {
    acknowledged_at?: string;
    acknowledged_by_user_id?: number;
    cooldown_until?: string;
    event_id?: number;
    first_triggered_at?: string;
    id?: number;
    last_triggered_at?: string;
    monitor_id?: number;
    monitor_revision?: number;
    occurrence_count?: number;
    policy_version?: string;
    reason?: string;
    resolved_at?: string;
    resolved_by_user_id?: number;
    severity?: string;
    state?: string;
    suppressed_at?: string;
    suppressed_by_user_id?: number;
    threshold?: number;
    title?: string;
    trigger_type?: string;
    version?: number;
  };

  type ApprovalRequest = {
    approval: "approved" | "rejected";
    /** Gin must not apply required directly to this nullable wrapper: both an
explicit JSON null and a positive integer are valid. The application
helper below enforces presence/value at runtime; validate keeps Swagger's
required property without making explicit null impossible to bind. */
    expected_draft_version: number;
    expected_monitor_version: number;
  };

  type AuthenticationResponse = {
    access_token?: string;
    user?: UserResponse;
  };

  type Capabilities = {
    api_version?: string;
  };

  type ChangePasswordRequest = {
    current_password: string;
    new_password: string;
  };

  type ClaimEvidenceRequest = {
    confidence?: number;
    content_id: number;
    excerpt?: string;
    locator: string;
    stance: string;
  };

  type ClaimRequest = {
    claim_hash: string;
    confidence?: number;
    evidence: ClaimEvidenceRequest[];
    id: number;
    manual_locked?: boolean;
    normalized_claim: string;
    status: string;
    version: number;
  };

  type ClaimResponse = {
    claim_hash?: string;
    confidence?: number;
    event_id?: number;
    id?: number;
    normalized_claim?: string;
    status?: string;
    version?: number;
  };

  type CollectionResultHttpCollectionRunPageResponse = {
    code?: number;
    data?: CollectionRunPageResponse;
    message?: string;
  };

  type CollectionResultHttpCollectionRunResponse = {
    code?: number;
    data?: CollectionRunResponse;
    message?: string;
  };

  type CollectionResultHttpSourceHealthResponse = {
    code?: number;
    data?: SourceHealthResponse;
    message?: string;
  };

  type CollectionResultInternalModulesSourceTransportHttpEmptyResponse = {
    code?: number;
    data?: EmptyResponse;
    message?: string;
  };

  type CollectionRunPageResponse = {
    items?: CollectionRunResponse[];
    next_cursor?: string;
  };

  type CollectionRunResponse = {
    accepted_count?: number;
    candidate_count?: number;
    error_code?: string;
    finished_at?: string;
    id?: number;
    rejected_count?: number;
    started_at?: string;
    status?: string;
    targets?: CollectionRunTargetResponse[];
  };

  type CollectionRunTargetResponse = {
    accepted_count?: number;
    candidate_count?: number;
    error_code?: string;
    id?: number;
    rejected_count?: number;
    status?: string;
  };

  type ConfirmPasswordResetRequest = {
    password: string;
    verification_ticket: string;
  };

  type ConfirmVerificationRequest = {
    code: string;
    email: string;
    purpose: "registration" | "password_reset";
  };

  type ConfirmVerificationResponse = {
    verification_ticket?: string;
  };

  type ContentDocumentResponse = {
    availability?: "ready" | "not_captured";
    canonical_url?: string;
    captured_at?: string;
    content_id?: number;
    language?: string;
    markdown?: string;
    published_at?: string;
    sha256?: string;
    source_name?: string;
    title?: string;
  };

  type ContentMetricsResponse = {
    comment_count?: number;
    like_count?: number;
    share_count?: number;
    view_count?: number;
  };

  type ContentPageResponse = {
    items?: ContentResponse[];
    next_cursor?: string;
  };

  type ContentResponse = {
    canonical_url?: string;
    content_type?: string;
    dedupe_reason?: string;
    dedupe_status?: "active" | "duplicate";
    dedupe_version?: string;
    external_id?: string;
    fetched_at?: string;
    id?: number;
    language?: string;
    metrics?: ContentMetricsResponse;
    published_at?: string;
    source_name?: string;
    source_type?: string;
    title?: string;
  };

  type ContentResultArrayHttpRelevanceEvaluationResponse = {
    code?: number;
    data?: RelevanceEvaluationResponse[];
    message?: string;
  };

  type ContentResultArrayHttpRelevancePreviewItemResponse = {
    code?: number;
    data?: RelevancePreviewItemResponse[];
    message?: string;
  };

  type ContentResultHttpContentDocumentResponse = {
    code?: number;
    data?: ContentDocumentResponse;
    message?: string;
  };

  type ContentResultHttpContentPageResponse = {
    code?: number;
    data?: ContentPageResponse;
    message?: string;
  };

  type ContentResultHttpContentResponse = {
    code?: number;
    data?: ContentResponse;
    message?: string;
  };

  type ContentResultHttpRelevanceFeedbackResponse = {
    code?: number;
    data?: RelevanceFeedbackResponse;
    message?: string;
  };

  type ContentResultHttpRelevanceMatchDetailResponse = {
    code?: number;
    data?: RelevanceMatchDetailResponse;
    message?: string;
  };

  type ContentResultHttpRelevanceMatchPageResponse = {
    code?: number;
    data?: RelevanceMatchPageResponse;
    message?: string;
  };

  type ContentResultHttpRelevanceRefreshResponse = {
    code?: number;
    data?: RelevanceRefreshResponse;
    message?: string;
  };

  type ContentResultHttpRelevanceSuggestionPageResponse = {
    code?: number;
    data?: RelevanceSuggestionPageResponse;
    message?: string;
  };

  type ContentResultHttpRelevanceSuggestionResponse = {
    code?: number;
    data?: RelevanceSuggestionResponse;
    message?: string;
  };

  type ContentResultInternalModulesIngestionTransportHttpEmptyResponse = {
    code?: number;
    data?: EmptyResponse;
    message?: string;
  };

  type CreateMetricCapabilityProfileRequest = {
    credibility_weight?: number;
    independence_strategy: "source_connection" | "author";
    max_single_item_contribution: number;
    normalization_window_hours: number;
    profile_version: string;
    source_type: "rss" | "hacker_news";
    supports_comments?: boolean;
    supports_likes?: boolean;
    supports_shares?: boolean;
    supports_views?: boolean;
  };

  type CreateModelProfileRequest = {
    credential_ref?: string;
    daily_budget?: string;
    embedding_dimensions?: number;
    enabled?: boolean;
    fallback_priority?: number;
    max_attempts?: number;
    max_cost?: string;
    model_name?: string;
    model_version?: string;
    name?: string;
    provider?: "openai" | "deepseek" | "ollama" | "onnx";
    task_type?:
      | "embedding"
      | "term_expansion"
      | "relevance_review"
      | "event_summary"
      | "entity_claim_extraction";
    timeout_seconds?: number;
  };

  type CreateMonitorRequest = {
    config: MonitorConfigRequest;
    description?: string;
    name: string;
    rules: MonitorRuleRequest[];
    sources: MonitorSourceRequest[];
  };

  type CreateSourceRequest = {
    auth_type: "none" | "api_key" | "oauth2" | "bearer";
    config?: SourceConfigRequest;
    credential_ref?: string;
    enabled?: boolean;
    endpoint: string;
    name: string;
    source_type: "rss" | "hacker_news";
    terms_policy_url?: string;
  };

  type CreateSubscriptionRequest = {
    channel?: "email" | "rss";
    enabled?: boolean;
    monitor_id?: number;
    recipient?: string;
    report_type?: "daily" | "weekly";
    schedule?: string;
    timezone?: string;
  };

  type deleteAiModelProfilesIdParams = {
    /** model profile ID */
    id: number;
  };

  type deleteContentsIdParams = {
    /** content ID */
    id: number;
  };

  type deleteMonitorsIdParams = {
    /** monitor ID */
    id: number;
  };

  type deleteReportSubscriptionsIdParams = {
    /** subscription ID */
    id: number;
  };

  type DeleteSubscriptionRequest = {
    expected_version: number;
  };

  type deleteUsersIdParams = {
    /** user ID */
    id: number;
  };

  type DeliveryEmptyResponse = true;

  type DeliveryResultHttpDeliveryEmptyResponse = {
    code?: number;
    data?: DeliveryEmptyResponse;
    message?: string;
  };

  type DeliveryResultHttpSubscriptionPageResponse = {
    code?: number;
    data?: SubscriptionPageResponse;
    message?: string;
  };

  type DeliveryResultHttpSubscriptionResponse = {
    code?: number;
    data?: SubscriptionResponse;
    message?: string;
  };

  type DeliveryResultHttpSubscriptionSecretResponse = {
    code?: number;
    data?: SubscriptionSecretResponse;
    message?: string;
  };

  type Document = {
    contentHash?: string;
    eventID?: number;
    generatedHash?: string;
    id?: number;
    reportID?: number;
    revisionNo?: number;
    status?: DocumentStatus;
    topicID?: number;
    type?: DocumentType;
    vaultPath?: string;
    version?: number;
  };

  type DocumentStatus =
    | "planned"
    | "active"
    | "conflict"
    | "archived"
    | "missing";

  type DocumentType = "event" | "topic" | "report";

  type EmptyResponse = true;

  type EmptyResponse = true;

  type EmptyResponse = true;

  type EmptyResponse = true;

  type EmptyResponse = true;

  type EmptyResponse = true;

  type EmptyResponse = true;

  type EmptyResponse = true;

  type EmptyResponse = true;

  type EmptyResponse = true;

  type EventIntelligenceResponse = {
    claims?: IntelligenceClaimResponse[];
    entities?: IntelligenceEntityResponse[];
    event_id?: number;
  };

  type EventMemberPageResponse = {
    items?: EventMemberResponse[];
  };

  type EventMemberResponse = {
    content_id?: number;
    event_id?: number;
    evidence_role?: string;
    id?: number;
    manual_locked?: boolean;
    membership_score?: number;
    origin?: string;
    representative?: boolean;
    version?: number;
  };

  type EventPageResponse = {
    items?: EventResponse[];
    next_cursor?: number;
  };

  type EventResponse = {
    calculated_at?: string;
    capability_profile_set_hash?: string;
    event_key?: string;
    first_seen_at?: string;
    heat_score?: number;
    heat_version?: string;
    id?: number;
    last_seen_at?: string;
    lifecycle_status?: string;
    manual_locked?: boolean;
    merged_into_id?: number;
    reason_codes?: string[];
    representative_content_id?: number;
    summary?: string;
    title_en?: string;
    title_zh?: string;
    trend_score?: number;
    trend_status?: string;
    version?: number;
    window_hours?: number;
  };

  type EventResultHttpClaimResponse = {
    code?: number;
    data?: ClaimResponse;
    message?: string;
  };

  type EventResultHttpEventIntelligenceResponse = {
    code?: number;
    data?: EventIntelligenceResponse;
    message?: string;
  };

  type EventResultHttpEventMemberPageResponse = {
    code?: number;
    data?: EventMemberPageResponse;
    message?: string;
  };

  type EventResultHttpEventMemberResponse = {
    code?: number;
    data?: EventMemberResponse;
    message?: string;
  };

  type EventResultHttpEventPageResponse = {
    code?: number;
    data?: EventPageResponse;
    message?: string;
  };

  type EventResultHttpEventResponse = {
    code?: number;
    data?: EventResponse;
    message?: string;
  };

  type EventResultHttpEventUpdatePageResponse = {
    code?: number;
    data?: EventUpdatePageResponse;
    message?: string;
  };

  type EventResultHttpExtractionRegenerationResponse = {
    code?: number;
    data?: ExtractionRegenerationResponse;
    message?: string;
  };

  type EventResultHttpHeatResponse = {
    code?: number;
    data?: HeatResponse;
    message?: string;
  };

  type EventResultHttpRadarPageResponse = {
    code?: number;
    data?: RadarPageResponse;
    message?: string;
  };

  type EventResultHttpSummaryRegenerationResponse = {
    code?: number;
    data?: SummaryRegenerationResponse;
    message?: string;
  };

  type EventResultInternalModulesEventTransportHttpEmptyResponse = {
    code?: number;
    data?: EmptyResponse;
    message?: string;
  };

  type EventSummaryResponse = {
    degraded?: boolean;
    sentences?: SummarySentenceResponse[];
    title_en?: string;
    title_zh?: string;
    version?: string;
  };

  type EventUpdatePageResponse = {
    items?: EventUpdateResponse[];
    next_cursor?: number;
  };

  type EventUpdateResponse = {
    after_state?: EventUpdateStateResponse;
    before_state?: EventUpdateStateResponse;
    event_id?: number;
    evidence_set_hash?: string;
    id?: number;
    kind?: string;
    observed_at?: string;
    reason_codes?: string[];
    sequence_no?: number;
    summary?: string;
    version?: number;
  };

  type EventUpdateStateResponse = {
    capability_profile_set_hash?: string;
    content_count?: number;
    evidence_set_hash?: string;
    heat_score?: number;
    heat_version?: string;
    source_count?: number;
    trend_score?: number;
    trend_status?: string;
    window_end?: string;
    window_hours?: number;
  };

  type ExtractionRegenerationResponse = {
    claim_count?: number;
    entity_count?: number;
    event_id?: number;
    reason_code?: string;
    reused?: boolean;
    run_id?: number;
    status?: string;
  };

  type getAiModelProfilesIdParams = {
    /** model profile ID */
    id: number;
  };

  type getAlertsIdParams = {
    /** alert thread ID */
    id: number;
  };

  type getAlertsParams = {
    /** thread state */
    state?: "open" | "acknowledged" | "resolved" | "suppressed";
    /** severity */
    severity?: "info" | "warning" | "critical";
    /** monitor ID */
    monitor_id?: number;
    /** page size */
    limit?: number;
    /** opaque alert cursor */
    cursor?: string;
  };

  type getCollectionRunsParams = {
    /** cursor */
    cursor?: string;
    /** page size */
    limit?: number;
  };

  type getContentsIdDocumentParams = {
    /** content ID */
    id: number;
  };

  type getContentsIdParams = {
    /** content ID */
    id: number;
  };

  type getContentsParams = {
    /** cursor */
    cursor?: string;
    /** page size */
    limit?: number;
  };

  type getEventsIdContentsParams = {
    /** event ID */
    id: number;
  };

  type getEventsIdHeatParams = {
    /** event ID */
    id: number;
  };

  type getEventsIdIntelligenceParams = {
    /** event ID */
    id: number;
  };

  type getEventsIdParams = {
    /** event ID */
    id: number;
  };

  type getEventsIdUpdatesParams = {
    /** event ID */
    id: number;
    /** sequence cursor */
    cursor?: number;
    /** page size */
    limit?: number;
  };

  type getEventsParams = {
    /** event id cursor */
    cursor?: number;
    /** page size */
    limit?: number;
  };

  type getMonitorsIdFeedbackEvaluationParams = {
    /** monitor ID */
    id: number;
  };

  type getMonitorsIdFeedbackSuggestionsParams = {
    /** monitor ID */
    id: number;
    /** pending, approved, or rejected */
    status?: string;
    /** opaque cursor */
    cursor?: string;
    /** page size, 1-100 */
    limit?: number;
  };

  type getMonitorsIdMatchesMatchIdParams = {
    /** monitor ID */
    id: number;
    /** match ID */
    match_id: number;
  };

  type getMonitorsIdMatchesParams = {
    /** monitor ID */
    id: number;
    /** accepted, rejected, or review */
    decision?: string;
    /** opaque cursor */
    cursor?: string;
    /** page size, 1-100 */
    limit?: number;
  };

  type getMonitorsIdParams = {
    /** monitor ID */
    id: number;
  };

  type getMonitorsParams = {
    /** cursor */
    cursor?: string;
    /** page size */
    limit?: number;
  };

  type getOperationsJobsParams = {
    /** last job id */
    cursor?: number;
    /** job kind */
    kind?: string;
    /** job state */
    state?: string;
    /** page size */
    limit?: number;
  };

  type getRadarEventsParams = {
    /** Radar window */
    window?: "1h" | "6h" | "24h" | "7d";
    /** monitor ID */
    monitor_id?: number;
    /** lifecycle filters */
    lifecycle?: (
      | "detected"
      | "active"
      | "cooling"
      | "closed"
      | "merged"
      | "archived"
      | "rejected"
    )[];
    /** trend filters */
    trend?: ("emerging" | "rising" | "stable" | "falling" | "dormant")[];
    /** verification filters */
    verification?: (
      | "disputed"
      | "corroborated"
      | "single_source"
      | "unverified"
      | "insufficient"
    )[];
    /** minimum heat */
    min_heat?: number;
    /** ranking dimension */
    sort?: "momentum" | "attention" | "breadth" | "latest" | "relevance";
    /** page size */
    limit?: number;
    /** opaque Radar cursor */
    cursor?: string;
  };

  type getReportsIdParams = {
    /** report ID */
    id: number;
  };

  type getReportsParams = {
    /** report id cursor */
    cursor?: number;
    /** page size */
    limit?: number;
    /** daily or weekly */
    type?: string;
    /** draft, published, failed or archived */
    status?: string;
  };

  type getReportSubscriptionsIdParams = {
    /** subscription ID */
    id: number;
  };

  type getReportSubscriptionsParams = {
    /** opaque subscription cursor */
    cursor?: string;
    /** page size */
    limit?: number;
  };

  type getSourceConnectionsIdParams = {
    /** source connection ID */
    id: number;
  };

  type getSourceConnectionsParams = {
    /** cursor */
    cursor?: string;
    /** page size */
    limit?: number;
  };

  type HeatResponse = {
    capability_profile_set_hash?: string;
    captured_at?: string;
    content_count?: number;
    event_id?: number;
    evidence_set_hash?: string;
    heat_score?: number;
    heat_version?: string;
    reason_codes?: string[];
    source_count?: number;
    trend_score?: number;
    trend_status?: string;
    window_hours?: number;
  };

  type IdentityResultArrayHttpUserResponse = {
    code?: number;
    data?: UserResponse[];
    message?: string;
  };

  type IdentityResultHttpAuthenticationResponse = {
    code?: number;
    data?: AuthenticationResponse;
    message?: string;
  };

  type IdentityResultHttpConfirmVerificationResponse = {
    code?: number;
    data?: ConfirmVerificationResponse;
    message?: string;
  };

  type IdentityResultHttpUserResponse = {
    code?: number;
    data?: UserResponse;
    message?: string;
  };

  type IdentityResultInternalModulesIdentityTransportHttpEmptyResponse = {
    code?: number;
    data?: EmptyResponse;
    message?: string;
  };

  type IntelligenceClaimResponse = {
    claim_hash?: string;
    confidence?: number;
    evidence?: IntelligenceEvidenceResponse[];
    id?: number;
    manual_locked?: boolean;
    normalized_claim?: string;
    status?: string;
    version?: number;
  };

  type IntelligenceEntityResponse = {
    canonical_name?: string;
    confidence?: number;
    confirmed?: boolean;
    entity_id?: number;
    entity_key?: string;
    entity_locked?: boolean;
    entity_type?: string;
    entity_version?: number;
    origin?: string;
    relation_id?: number;
    relation_version?: number;
    role?: string;
  };

  type IntelligenceEvidenceResponse = {
    confidence?: number;
    content_id?: number;
    excerpt?: string;
    locator?: string;
    stance?: string;
  };

  type JobPageResponse = {
    items?: JobResponse[];
    next_cursor?: number;
  };

  type JobResponse = {
    attempt?: number;
    attempted_at?: string;
    created_at?: string;
    finalized_at?: string;
    id?: number;
    kind?: string;
    max_attempts?: number;
    priority?: number;
    scheduled_at?: string;
    state?: string;
  };

  type JobResultHttpJobPageResponse = {
    code?: number;
    data?: JobPageResponse;
    message?: string;
  };

  type JobResultHttpJobResponse = {
    code?: number;
    data?: JobResponse;
    message?: string;
  };

  type JobResultInternalModulesOperationsTransportHttpEmptyResponse = {
    code?: number;
    data?: EmptyResponse;
    message?: string;
  };

  type LifecycleRequest = {
    expected_version: number;
    reason: string;
    to: string;
  };

  type LifecycleRequest = {
    expected_monitor_version: number;
  };

  type LoginRequest = {
    email: string;
    password: string;
  };

  type ManagementSourceResponse = {
    config?: SourceConfigDTO;
    credential_configured?: boolean;
    deleted?: boolean;
    enabled?: boolean;
    endpoint?: string;
    health_status?: string;
    id?: number;
    name?: string;
    source_type?: string;
    terms_policy_url?: string;
    version?: number;
  };

  type MemberLockRequest = {
    expected_version: number;
    locked?: boolean;
    reason: string;
  };

  type MergeRequest = {
    reason: string;
    source_expected_version: number;
    target_event_id: number;
    target_expected_version: number;
  };

  type MetricCapabilityLifecycleRequest = {
    expected_version: number;
    reason_code: string;
  };

  type MetricCapabilityProfileResponse = {
    archived_at?: string;
    credibility_weight?: number;
    id?: number;
    independence_strategy?: string;
    max_single_item_contribution?: number;
    normalization_window_hours?: number;
    profile_version?: string;
    published_at?: string;
    source_type?: string;
    status?: string;
    supports_comments?: boolean;
    supports_likes?: boolean;
    supports_shares?: boolean;
    supports_views?: boolean;
    version?: number;
  };

  type ModelProfileListResponse = {
    items?: ModelProfileResponse[];
  };

  type ModelProfileResponse = {
    created_at?: string;
    daily_budget?: string;
    deleted?: boolean;
    embedding_dimensions?: number;
    enabled?: boolean;
    fallback_priority?: number;
    id?: number;
    max_attempts?: number;
    max_cost?: string;
    model_name?: string;
    model_version?: string;
    name?: string;
    provider?: string;
    task_type?: string;
    timeout_seconds?: number;
    updated_at?: string;
    version?: number;
  };

  type ModelProfileResultHttpModelProfileListResponse = {
    code?: number;
    data?: ModelProfileListResponse;
    message?: string;
  };

  type ModelProfileResultHttpModelProfileResponse = {
    code?: number;
    data?: ModelProfileResponse;
    message?: string;
  };

  type ModelProfileResultInternalModulesIntelligenceTransportHttpEmptyResponse =
    {
      code?: number;
      data?: EmptyResponse;
      message?: string;
    };

  type ModelProfileVersionRequest = {
    version?: number;
  };

  type MonitorConfigRequest = {
    collection_interval_seconds: number;
    event_threshold: number;
    languages: string[];
    regions?: string[];
    relevance_threshold: number;
    retention_days: number;
    timezone: string;
  };

  type MonitorConfigResponse = {
    collection_interval_seconds?: number;
    event_threshold?: number;
    id?: number;
    languages?: string[];
    regions?: string[];
    relevance_threshold?: number;
    retention_days?: number;
    revision?: number;
    rules?: MonitorRuleResponse[];
    sources?: MonitorSourceResponse[];
    timezone?: string;
    version?: number;
  };

  type MonitorPageResponse = {
    items?: MonitorResponse[];
    next_cursor?: string;
  };

  type MonitorResponse = {
    description?: string;
    draft?: MonitorConfigResponse;
    id?: number;
    name?: string;
    published?: MonitorConfigResponse;
    published_revision?: number;
    status?: string;
    version?: number;
  };

  type MonitorResultHttpMonitorPageResponse = {
    code?: number;
    data?: MonitorPageResponse;
    message?: string;
  };

  type MonitorResultHttpMonitorResponse = {
    code?: number;
    data?: MonitorResponse;
    message?: string;
  };

  type MonitorResultHttpMonitorRuleResponse = {
    code?: number;
    data?: MonitorRuleResponse;
    message?: string;
  };

  type MonitorResultHttpPreviewResponse = {
    code?: number;
    data?: PreviewResponse;
    message?: string;
  };

  type MonitorResultInternalModulesMonitorTransportHttpEmptyResponse = {
    code?: number;
    data?: EmptyResponse;
    message?: string;
  };

  type MonitorRuleRequest = {
    enabled?: boolean;
    operator: string;
    priority?: number;
    rule_type: string;
    value: string;
    weight?: number;
  };

  type MonitorRuleResponse = {
    approval_status?: string;
    enabled?: boolean;
    id?: number;
    operator?: string;
    origin?: string;
    priority?: number;
    rule_type?: string;
    value?: string;
    weight?: number;
  };

  type MonitorSourceRequest = {
    enabled?: boolean;
    priority?: number;
    query_override?: string;
    source_connection_id: number;
  };

  type MonitorSourceResponse = {
    enabled?: boolean;
    id?: number;
    name?: string;
    priority?: number;
    query_override?: string;
    source_connection_id?: number;
    source_type?: string;
  };

  type OverviewResultDomainRuntimeOverview = {
    code?: number;
    data?: RuntimeOverview;
    message?: string;
  };

  type OverviewResultInternalModulesOperationsTransportHttpEmptyResponse = {
    code?: number;
    data?: EmptyResponse;
    message?: string;
  };

  type patchAiModelProfilesIdParams = {
    /** model profile ID */
    id: number;
  };

  type patchReportSubscriptionsIdParams = {
    /** subscription ID */
    id: number;
  };

  type patchSourceConnectionsIdParams = {
    /** source connection ID */
    id: number;
  };

  type patchUsersIdParams = {
    /** user ID */
    id: number;
  };

  type postAiModelProfilesIdRestoreParams = {
    /** model profile ID */
    id: number;
  };

  type postAlertsIdAcknowledgeParams = {
    /** alert thread ID */
    id: number;
  };

  type postAlertsIdResolveParams = {
    /** alert thread ID */
    id: number;
  };

  type postAlertsIdSuppressParams = {
    /** alert thread ID */
    id: number;
  };

  type postCollectionRunsIdRetryParams = {
    /** collection run ID */
    id: number;
  };

  type postEventsIdClaimsParams = {
    /** event ID */
    id: number;
  };

  type postEventsIdContentsContentIdLockParams = {
    /** event ID */
    id: number;
    /** content ID */
    content_id: number;
  };

  type postEventsIdIntelligenceExtractParams = {
    /** event ID */
    id: number;
  };

  type postEventsIdIntelligenceSummaryRegenerateParams = {
    /** event ID */
    id: number;
  };

  type postEventsIdLifecycleParams = {
    /** event ID */
    id: number;
  };

  type postEventsIdMergeParams = {
    /** source event ID */
    id: number;
  };

  type postEventsIdSplitParams = {
    /** source event ID */
    id: number;
  };

  type postKnowledgeProposalsIdApplyParams = {
    /** proposal ID */
    id: number;
    /** proposal version */
    version: number;
  };

  type postKnowledgeProposalsIdApproveParams = {
    /** proposal ID */
    id: number;
    /** proposal version */
    version: number;
  };

  type postKnowledgeProposalsIdRejectParams = {
    /** proposal ID */
    id: number;
    /** proposal version */
    version: number;
  };

  type postMetricCapabilityProfilesIdArchiveParams = {
    /** metric capability profile ID */
    id: number;
  };

  type postMetricCapabilityProfilesIdPublishParams = {
    /** metric capability profile ID */
    id: number;
  };

  type postMonitorsIdArchiveParams = {
    /** monitor ID */
    id: number;
  };

  type postMonitorsIdDraftAiCandidatesParams = {
    /** monitor ID */
    id: number;
  };

  type postMonitorsIdDraftRulesRuleIdApprovalParams = {
    /** monitor ID */
    id: number;
    /** rule ID */
    rule_id: number;
  };

  type postMonitorsIdFeedbackSuggestionsRefreshParams = {
    /** monitor ID */
    id: number;
  };

  type postMonitorsIdFeedbackSuggestionsSuggestionIdReviewParams = {
    /** monitor ID */
    id: number;
    /** suggestion ID */
    suggestion_id: number;
  };

  type postMonitorsIdPauseParams = {
    /** monitor ID */
    id: number;
  };

  type postMonitorsIdPreviewParams = {
    /** monitor ID */
    id: number;
  };

  type postMonitorsIdPublishParams = {
    /** monitor ID */
    id: number;
  };

  type postMonitorsIdRelevancePreviewParams = {
    /** monitor ID */
    id: number;
  };

  type postMonitorsIdRestoreParams = {
    /** monitor ID */
    id: number;
  };

  type postMonitorsIdResumeParams = {
    /** monitor ID */
    id: number;
  };

  type postOperationsJobsIdCancelParams = {
    /** job id */
    id: number;
  };

  type postOperationsJobsIdRetryParams = {
    /** job id */
    id: number;
  };

  type postReportsIdBuildParams = {
    /** report ID */
    id: number;
  };

  type postReportsIdPreviewParams = {
    /** report ID */
    id: number;
  };

  type postReportsIdPublishParams = {
    /** report ID */
    id: number;
  };

  type postReportSubscriptionsIdRssTokenRotateParams = {
    /** subscription ID */
    id: number;
  };

  type postSourceConnectionsIdArchiveParams = {
    /** source connection ID */
    id: number;
  };

  type postSourceConnectionsIdDisableParams = {
    /** source connection ID */
    id: number;
  };

  type postSourceConnectionsIdEnableParams = {
    /** source connection ID */
    id: number;
  };

  type postSourceConnectionsIdHealthParams = {
    /** source connection ID */
    id: number;
  };

  type postSourceConnectionsIdRestoreParams = {
    /** source connection ID */
    id: number;
  };

  type postUsersIdRestoreParams = {
    /** user ID */
    id: number;
  };

  type PreviewResponse = {
    config_hash?: string;
    eligible?: boolean;
    estimated_requests?: number;
    sources?: PreviewSourceResponse[];
    warnings?: string[];
  };

  type PreviewSourceResponse = {
    estimated_requests?: number;
    excluded_rule_ids?: number[];
    included_rule_ids?: number[];
    query_signature?: string;
    source_connection_id?: number;
    unapproved_rule_ids?: number[];
  };

  type Proposal = {
    baseHash?: string;
    baseRevisionNo?: number;
    diffSummary?: string;
    documentID?: number;
    id?: number;
    proposedBody?: string;
    proposedFrontmatter?: string;
    reason?: string;
    status?: ProposalStatus;
    version?: number;
  };

  type ProposalRequest = {
    base_hash?: string;
    base_revision?: number;
    body?: string;
    document_id?: number;
    frontmatter?: string;
    reason?: string;
  };

  type ProposalResultDomainDocument = {
    code?: number;
    data?: Document;
    message?: string;
  };

  type ProposalResultDomainProposal = {
    code?: number;
    data?: Proposal;
    message?: string;
  };

  type ProposalResultDomainReconciliationReport = {
    code?: number;
    data?: ReconciliationReport;
    message?: string;
  };

  type ProposalResultInternalModulesKnowledgeTransportHttpEmptyResponse = {
    code?: number;
    data?: EmptyResponse;
    message?: string;
  };

  type ProposalStatus =
    | "pending"
    | "approved"
    | "rejected"
    | "conflict"
    | "applied"
    | "failed";

  type PublishRequest = {
    /** Gin must not apply required directly to this nullable wrapper: both an
explicit JSON null and a positive integer are valid. The application
helper below enforces presence/value at runtime; validate keeps Swagger's
required property without making explicit null impossible to bind. */
    expected_draft_version: number;
    expected_monitor_version: number;
  };

  type putMonitorsIdContentsContentIdFeedbackParams = {
    /** monitor ID */
    id: number;
    /** content ID */
    content_id: number;
  };

  type putMonitorsIdDraftParams = {
    /** monitor ID */
    id: number;
  };

  type putMonitorsIdMatchesMatchIdFeedbackParams = {
    /** monitor ID */
    id: number;
    /** match ID */
    match_id: number;
  };

  type RadarEventResponse = {
    attention?: number;
    breadth?: number;
    confirmation?: string;
    confirmation_score?: number;
    data_confidence?: number;
    event_id?: number;
    event_key?: string;
    first_seen_at?: string;
    independent_source_count?: number;
    last_seen_at?: string;
    latest_update?: EventUpdateResponse;
    lifecycle_status?: string;
    momentum?: number;
    ranking_score?: number;
    reason_codes?: string[];
    summary?: string;
    title?: string;
    title_en?: string;
    title_zh?: string;
    trend_score?: number;
    trend_status?: string;
    version?: number;
    watch_final_score?: number;
    watch_relevance?: number;
  };

  type RadarPageResponse = {
    as_of?: string;
    items?: RadarEventResponse[];
    next_cursor?: string;
  };

  type ReconciliationIssue = {
    actualHash?: string;
    expectedHash?: string;
    kind?: string;
    path?: string;
  };

  type ReconciliationReport = {
    changed?: number;
    conflict?: number;
    issues?: ReconciliationIssue[];
    scanned?: number;
  };

  type RegistrationRequest = {
    display_name: string;
    password: string;
    verification_ticket: string;
  };

  type RelevanceContentResponse = {
    canonical_url?: string;
    id?: number;
    language?: string;
    published_at?: string;
    title?: string;
  };

  type RelevanceEvaluationResponse = {
    evaluated_count?: number;
    exclusion_false_positive_rate?: number;
    precision_at_20?: number;
    scoring_version?: string;
  };

  type RelevanceExplanationResponse = {
    excluded_terms?: string[];
    matched_entities?: string[];
    matched_terms?: string[];
    reason_codes?: string[];
    recall_paths?: string[];
    scores?: Record<string, any>;
  };

  type RelevanceFactorsResponse = {
    entity?: number;
    lexical?: number;
    preference?: number;
    semantic?: number;
    title?: number;
  };

  type RelevanceFalseNegativeFeedbackRequest = {
    expected_feedback_version?: number;
  };

  type RelevanceFeedbackRequest = {
    expected_feedback_version?: number;
    feedback_type?: string;
  };

  type RelevanceFeedbackResponse = {
    content_id?: number;
    feedback_type?: string;
    id?: number;
    match_id?: number;
    updated_at?: string;
    version?: number;
  };

  type RelevanceMatchDetailResponse = {
    content?: RelevanceContentResponse;
    match?: RelevanceMatchResponse;
  };

  type RelevanceMatchPageResponse = {
    items?: RelevanceMatchResponse[];
    next_cursor?: string;
  };

  type RelevanceMatchResponse = {
    content_id?: number;
    created_at?: string;
    decision?: "accepted" | "rejected" | "review";
    decision_origin?: "rule" | "ai";
    degraded?: boolean;
    explanation?: RelevanceExplanationResponse;
    final_score?: number;
    id?: number;
    llm_score?: number;
    manual_locked?: boolean;
    monitor_config_version_id?: number;
    reason_codes?: string[];
    recall_paths?: string[];
    rule_score?: number;
    scoring_version?: string;
    semantic_score?: number;
    version?: number;
  };

  type RelevancePreviewCandidateResponse = {
    decision?: string;
    degraded?: boolean;
    excluded_terms?: string[];
    factors?: RelevanceFactorsResponse;
    hard_veto?: boolean;
    matched_entities?: string[];
    matched_terms?: string[];
    monitor_config_version_id?: number;
    reason_codes?: string[];
    recall_paths?: string[];
    rule_score?: number;
    scoring_version?: string;
  };

  type RelevancePreviewItemResponse = {
    candidates?: RelevancePreviewCandidateResponse[];
    content_id?: number;
  };

  type RelevanceRefreshResponse = {
    suggestion_count?: number;
  };

  type RelevanceSuggestionPageResponse = {
    items?: RelevanceSuggestionResponse[];
    next_cursor?: string;
  };

  type RelevanceSuggestionResponse = {
    created_at?: string;
    id?: number;
    status?: string;
    suggestion_type?: string;
    support_count?: number;
    updated_at?: string;
    value?: string;
    version?: number;
  };

  type RelevanceSuggestionReviewRequest = {
    expected_version?: number;
    status?: string;
  };

  type ReplaceDraftRequest = {
    config: MonitorConfigRequest;
    description?: string;
    /** Gin must not apply required directly to this nullable wrapper: both an
explicit JSON null and a positive integer are valid. The application
helper below enforces presence/value at runtime; validate keeps Swagger's
required property without making explicit null impossible to bind. */
    expected_draft_version: number;
    expected_monitor_version: number;
    name: string;
    rules: MonitorRuleRequest[];
    sources: MonitorSourceRequest[];
  };

  type ReportItemResponse = {
    event_id?: number;
    heat_score?: number;
    inclusion_reason?: string;
    rank?: number;
    summary?: string;
    title?: string;
  };

  type ReportPageResponse = {
    items?: ReportResponse[];
    next_cursor?: number;
  };

  type ReportPreviewResponse = {
    publishable?: boolean;
    report?: ReportResponse;
  };

  type ReportResponse = {
    body?: string;
    frozen?: boolean;
    generated_at?: string;
    id?: number;
    items?: ReportItemResponse[];
    monitor_id?: number;
    period_end?: string;
    period_start?: string;
    published_at?: string;
    status?: string;
    summary?: string;
    timezone?: string;
    title?: string;
    type?: string;
    version?: number;
    version_no?: number;
  };

  type ReportResultHttpReportPageResponse = {
    code?: number;
    data?: ReportPageResponse;
    message?: string;
  };

  type ReportResultHttpReportPreviewResponse = {
    code?: number;
    data?: ReportPreviewResponse;
    message?: string;
  };

  type ReportResultHttpReportResponse = {
    code?: number;
    data?: ReportResponse;
    message?: string;
  };

  type ReportResultInternalModulesReportTransportHttpEmptyResponse = {
    code?: number;
    data?: EmptyResponse;
    message?: string;
  };

  type RequestVerificationRequest = {
    email: string;
    purpose: "registration" | "password_reset";
  };

  type ResultHttpCapabilities = {
    code?: number;
    data?: Capabilities;
    message?: string;
  };

  type RotateRSSTokenRequest = {
    expected_version: number;
  };

  type RuntimeOverview = {
    available_jobs?: number;
    cancelled_jobs?: number;
    completed_jobs?: number;
    discarded_jobs?: number;
    generated_at?: string;
    oldest_available_at?: string;
    running_jobs?: number;
  };

  type SourceConfigDTO = {
    allow_body_storage?: boolean;
    allowed_languages?: string[];
    allowed_regions?: string[];
    content_retention_days?: number;
    max_pages_per_run?: number;
    metrics_retention_days?: number;
    rate_limit_per_minute?: number;
    request_timeout_seconds?: number;
    requires_attribution?: boolean;
    requires_deletion_sync?: boolean;
  };

  type SourceConfigRequest = {
    allow_body_storage?: boolean;
    allowed_languages?: string[];
    allowed_regions?: string[];
    content_retention_days?: number;
    max_pages_per_run?: number;
    metrics_retention_days?: number;
    rate_limit_per_minute?: number;
    request_timeout_seconds?: number;
    requires_attribution?: boolean;
    requires_deletion_sync?: boolean;
  };

  type SourceHealthResponse = {
    checked_at?: string;
    error_code?: string;
    healthy?: boolean;
  };

  type SourceLifecycleRequest = {
    expected_source_version: number;
  };

  type SourceReadPageResponse = {
    items?: SourceReadResponse[];
    next_cursor?: string;
  };

  type SourceReadResponse = {
    config?: SourceConfigDTO;
    credential_configured?: boolean;
    deleted?: boolean;
    enabled?: boolean;
    endpoint?: string;
    health_status?: string;
    id?: number;
    name?: string;
    source_type?: string;
    terms_policy_url?: string;
    version?: number;
  };

  type SourceResultHttpManagementSourceResponse = {
    code?: number;
    data?: ManagementSourceResponse;
    message?: string;
  };

  type SourceResultHttpMetricCapabilityProfileResponse = {
    code?: number;
    data?: MetricCapabilityProfileResponse;
    message?: string;
  };

  type SourceResultHttpSourceReadPageResponse = {
    code?: number;
    data?: SourceReadPageResponse;
    message?: string;
  };

  type SourceResultHttpSourceReadResponse = {
    code?: number;
    data?: SourceReadResponse;
    message?: string;
  };

  type SourceResultInternalModulesSourceTransportHttpEmptyResponse = {
    code?: number;
    data?: EmptyResponse;
    message?: string;
  };

  type SplitMemberRequest = {
    content_id: number;
    expected_version: number;
  };

  type SplitRequest = {
    members: SplitMemberRequest[];
    reason: string;
    source_expected_version: number;
  };

  type SubscriptionPageResponse = {
    items?: SubscriptionResponse[];
    next_cursor?: string;
  };

  type SubscriptionResponse = {
    channel?: string;
    enabled?: boolean;
    id?: number;
    monitor_id?: number;
    recipient?: string;
    report_type?: string;
    schedule?: string;
    timezone?: string;
    version?: number;
  };

  type SubscriptionSecretResponse = {
    rss_token?: string;
    subscription?: SubscriptionResponse;
  };

  type SummaryRegenerationResponse = {
    event_id?: number;
    reason_code?: string;
    reused?: boolean;
    run_id?: number;
    status?: string;
    summary?: EventSummaryResponse;
  };

  type SummarySentenceResponse = {
    evidence?: IntelligenceEvidenceResponse[];
    text?: string;
  };

  type UpdateModelProfileRequest = {
    daily_budget?: string;
    enabled?: boolean;
    fallback_priority?: number;
    max_attempts?: number;
    max_cost?: string;
    timeout_seconds?: number;
    version?: number;
  };

  type UpdateSourceRequest = {
    auth_type?: "none" | "api_key" | "oauth2" | "bearer";
    config?: SourceConfigRequest;
    credential_ref?: string;
    endpoint?: string;
    expected_source_version: number;
    name?: string;
    source_type?: "rss" | "hacker_news";
    terms_policy_url?: string;
  };

  type UpdateSubscriptionRequest = {
    enabled?: boolean;
    expected_version: number;
    recipient?: string;
    schedule?: string;
    timezone?: string;
  };

  type UpdateUserRequest = {
    role?: "admin" | "editor" | "viewer";
    status?: "active" | "disabled";
  };

  type UserResponse = {
    created_at?: string;
    deleted_at?: string;
    display_name?: string;
    email?: string;
    id?: number;
    last_login_at?: string;
    role?: "admin" | "editor" | "viewer";
    status?: "active" | "disabled";
    updated_at?: string;
  };
}
