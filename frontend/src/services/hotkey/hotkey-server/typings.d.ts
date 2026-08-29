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

  type AIRunRecomputeResponse = {
    created?: boolean;
    job_id?: number;
    run_id?: number;
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

  type AuditPage = {
    items?: AuditRecord[];
    next_cursor?: string;
  };

  type AuditRecord = {
    action?: string;
    actor_id?: number;
    actor_type?: string;
    created_at?: string;
    id?: number;
    request_id?: string;
    resource_id?: number;
    resource_type?: string;
    result?: string;
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

  type CitationAnchorMapResponseDTO = {
    anchor_map_version?: string;
    markdown_anchor?: string;
    normalization_version?: string;
  };

  type CitationArtifactAnchorBlockResponseDTO = {
    markdown_anchor?: string;
    ordinal?: number;
  };

  type CitationArtifactAnchorMapResponseDTO = {
    anchor_map_profile_version?: string;
    anchor_map_sha256?: string;
    blocks?: CitationArtifactAnchorBlockResponseDTO[];
    normalization_version?: string;
  };

  type CitationArtifactResponseDTO = {
    anchor_map?: CitationArtifactAnchorMapResponseDTO;
    artifact_type?: "markdown";
    etag?: string;
    mime_type?: string;
    sha256?: string;
    size_bytes?: number;
    transformer_profile_sha256?: string;
  };

  type CitationPartyResponseDTO = {
    display_name?: string;
    external_id?: string;
    homepage_url?: string;
    identity_namespace?: string;
    kind?: "organization" | "person" | "account";
    role?: "publisher" | "author" | "distributor" | "content_origin";
  };

  type CitationRawEvidenceResponseDTO = {
    availability?:
      | "available"
      | "expired"
      | "exception_retained"
      | "unavailable";
    deletion_audited?: boolean;
    exception_approved?: boolean;
    payload_sha256s?: string[];
    retention_until?: string;
  };

  type CitationResponseDTO = {
    anchor_map?: CitationAnchorMapResponseDTO;
    artifact?: CitationArtifactResponseDTO;
    author?: string;
    availability?:
      | "full_archive"
      | "partial_archive"
      | "summary_only"
      | "metadata_only"
      | "policy_blocked"
      | "temporarily_unavailable"
      | "quarantined"
      | "tombstoned";
    body_origin?: string;
    canonical_url?: string;
    captured_at?: string;
    completeness?: string;
    content_origin?: CitationPartyResponseDTO;
    content_origin_availability?: "available" | "unavailable";
    content_origin_unavailable_reason?: string;
    content_sha256?: string;
    discussion_url?: string;
    distributors?: CitationPartyResponseDTO[];
    document_id?: number;
    document_version_id?: number;
    exact_quote?: string;
    language?: string;
    locator_availability?: "available" | "unavailable";
    locator_unavailable_reason?: string;
    published_at?: string;
    published_utc_offset_minutes?: number;
    publisher?: string;
    publisher_availability?: "available" | "unavailable";
    publisher_party?: CitationPartyResponseDTO;
    publisher_unavailable_reason?: string;
    raw_evidence?: CitationRawEvidenceResponseDTO;
    source_name?: string;
    source_record_url?: string;
    source_type?: string;
    title?: string;
    unavailable_reason?: string;
    utf8_byte_end?: number;
    utf8_byte_start?: number;
  };

  type ClaimEvidenceCorrectionResponseDTO = {
    evidence_id?: number;
    evidence_state?: EvidenceStateResponseDTO;
    evidence_version?: number;
    feedback_id?: number;
  };

  type ClaimEvidenceMutationResponseDTO = {
    claim_id?: number;
    claim_version?: number;
    evidence_id?: number;
    evidence_state?: EvidenceStateResponseDTO;
    evidence_version?: number;
  };

  type ClaimEvidenceResponseDTO = {
    availability?: string;
    canonical_url?: string;
    captured_at?: string;
    claim_id?: number;
    claim_version?: number;
    content_family_id?: number;
    content_family_member_version?: number;
    content_origin?: string;
    created_at?: string;
    decision_origin?: string;
    document_version_id?: number;
    exact_quote?: string;
    extraction_schema_version?: string;
    id?: number;
    lineage_decision_id?: number;
    lineage_root_document_version_id?: number;
    markdown_anchor?: string;
    object?: string;
    plaintext_sha256?: string;
    predicate?: string;
    prefix?: string;
    published_at?: string;
    publisher?: string;
    quote_sha256?: string;
    relation?: string;
    selector_version?: string;
    source_record_url?: string;
    subject?: string;
    suffix?: string;
    text_quote_selector_id?: number;
    utf8_byte_end?: number;
    utf8_byte_start?: number;
    version?: number;
  };

  type ClaimQualifierRequestDTO = {
    key: string;
    value: string;
  };

  type CleanupResult = {
    affected?: number;
    batch_size?: number;
    candidate_hash?: string;
    cutoff?: string;
    data_class?: string;
    dry_run?: boolean;
    failure_code?: string;
    has_more?: boolean;
    policy_version?: number;
    run_id?: number;
    status?: string;
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

  type CollectionResultHttpManualCollectionResponse = {
    code?: number;
    data?: ManualCollectionResponse;
    message?: string;
  };

  type CollectionResultHttpMonitorScanPageResponse = {
    code?: number;
    data?: MonitorScanPageResponse;
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
    availability?: "ready" | "not_captured" | "unavailable";
    canonical_url?: string;
    captured_at?: string;
    content_id?: number;
    language?: string;
    markdown?: string;
    published_at?: string;
    sha256?: string;
    source_name?: string;
    title?: string;
    unavailable_reason?:
      | "pending"
      | "missing"
      | "deleting"
      | "read_failed"
      | "integrity_failed";
  };

  type ContentLineageFeedbackResponseDTO = {
    document_version_id?: number;
    feedback_id?: number;
    feedback_type?: string;
    lineage_decision_id?: number;
    original_content_family_id?: number;
    original_parent_document_version_id?: number;
    original_relation?: string;
    result_content_family_id?: number;
    result_content_family_version?: number;
    result_lineage_decision_id?: number;
    result_parent_document_version_id?: number;
    result_relation?: string;
    reused?: boolean;
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
    document_version_id?: number;
    external_id?: string;
    fetched_at?: string;
    id?: number;
    language?: string;
    match_decision?: "accepted" | "review" | "rejected";
    metrics?: ContentMetricsResponse;
    published_at?: string;
    relevance_score?: number;
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

  type ContentResultHttpCitationResponseDTO = {
    code?: number;
    data?: CitationResponseDTO;
    message?: string;
  };

  type ContentResultHttpContentDocumentResponse = {
    code?: number;
    data?: ContentDocumentResponse;
    message?: string;
  };

  type ContentResultHttpContentLineageFeedbackResponseDTO = {
    code?: number;
    data?: ContentLineageFeedbackResponseDTO;
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

  type ContentResultHttpDocumentMatchPageResponseDTO = {
    code?: number;
    data?: DocumentMatchPageResponseDTO;
    message?: string;
  };

  type ContentResultHttpHotspotPageResponse = {
    code?: number;
    data?: HotspotPageResponse;
    message?: string;
  };

  type ContentResultHttpOverrideDocumentMatchResponseDTO = {
    code?: number;
    data?: OverrideDocumentMatchResponseDTO;
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

  type ContentResultHttpTextQuoteSelectorResponseDTO = {
    code?: number;
    data?: TextQuoteSelectorResponseDTO;
    message?: string;
  };

  type ContentResultHttpVersionedDocumentResponseDTO = {
    code?: number;
    data?: VersionedDocumentResponseDTO;
    message?: string;
  };

  type ContentResultInternalModulesIngestionTransportHttpEmptyResponse = {
    code?: number;
    data?: EmptyResponse;
    message?: string;
  };

  type CorrectClaimEvidenceRequestDTO = {
    expected_claim_version: number;
    note?: string;
    reason_code: string;
    result_relation: string;
    result_text_quote_selector_id: number;
  };

  type CreateMetricCapabilityProfileRequest = {
    independence_strategy: "source_connection" | "author";
    max_single_item_contribution: number;
    normalization_window_hours: number;
    profile_version: string;
    source_type: "rss" | "hacker_news" | "x" | "bilibili" | "weibo";
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
      | "event_cluster"
      | "event_summary"
      | "entity_claim_extraction";
    timeout_seconds?: number;
  };

  type CreateMonitorRequest = {
    alert_email_enabled?: boolean;
    collection_interval_seconds?: number;
    name: string;
    query: string;
    source_connection_ids: number[];
  };

  type CreateReportRequest = {
    at?: string;
    monitor_id?: number;
    timezone: string;
    type: "daily" | "weekly";
  };

  type CreateRightsPolicyRequestDTO = {
    approved_by_user_id?: number;
    basis_summary?: string;
    effective_from?: string;
    expires_at?: string;
    license_uri?: string;
    parent_policy_id?: number;
    priority?: number;
    revision?: number;
    scope_subject?: string;
    scope_type?: string;
    terms_url?: string;
  };

  type CreateRightsPolicyResponseDTO = {
    idempotent_replay?: boolean;
    policy?: RightsPolicyResponseDTO;
  };

  type CreateSourceRequest = {
    auth_type?: "none" | "api_key" | "oauth2" | "bearer";
    config?: SourceConfigRequest;
    credential?: string;
    credential_ref?: string;
    enabled?: boolean;
    endpoint?: string;
    name: string;
    preset_id?: string;
    preset_values?: SourcePresetValueRequest[];
    source_type?:
      | "rss"
      | "hacker_news"
      | "x"
      | "bing_grounding"
      | "bilibili"
      | "weibo"
      | "google_agent_search";
    terms_policy_url?: string;
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

  type deleteUsersIdParams = {
    /** user ID */
    id: number;
  };

  type DocumentMatchPageResponseDTO = {
    items?: DocumentMatchResponseDTO[];
    next_cursor?: string;
  };

  type DocumentMatchResponseDTO = {
    automatic_decision?: "accepted" | "review" | "rejected";
    calibration_version?: string;
    compiled_profile_id?: number;
    decided_at?: string;
    degraded?: boolean;
    document_version_id?: number;
    effective_decision?: "accepted" | "review" | "rejected";
    match_decision_id?: number;
    matching_algorithm_version?: string;
    monitor_id?: number;
    monitor_version_id?: number;
    reason_codes?: string[];
    relevance_probability?: number;
    relevance_profile_id?: number;
    reranker_version?: string;
    resource_version?: number;
    rrf_score?: number;
    signals?: DocumentMatchSignalResponseDTO[];
  };

  type DocumentMatchSignalResponseDTO = {
    algorithm_version?: string;
    channel?: "lexical" | "semantic" | "structured";
    rank?: number;
    raw_score?: number;
  };

  type DocumentPageResponse = {
    items?: DocumentResponse[];
    next_cursor?: string;
  };

  type DocumentResponse = {
    contentHash?: string;
    eventID?: number;
    generatedHash?: string;
    id?: number;
    reportID?: number;
    revisionNo?: number;
    status?: string;
    topicID?: number;
    type?: string;
    vaultPath?: string;
    version?: number;
  };

  type EmptyResponse = true;

  type EmptyResponse = true;

  type EmptyResponse = true;

  type EmptyResponse = true;

  type EmptyResponse = true;

  type EmptyResponse = true;

  type EmptyResponse = true;

  type EmptyResponse = true;

  type EmptyResponse = true;

  type EmptyResponseDTO = true;

  type EvaluateRightsActionsRequestDTO = {
    at?: string;
    input_digest?: string;
    subject_key?: string;
    subject_type?: string;
  };

  type EventHeatV2ResponseDTO = {
    acceleration?: number;
    available_weight?: number;
    coverage?: number;
    heat_profile_version?: string;
    heat_score?: number;
    id?: number;
    independent_lineage_root_count?: number;
    micro_event_version?: number;
    normalized_engagement?: number;
    reason_codes?: string[];
    recency?: number;
    velocity?: number;
    warming_up?: boolean;
    window_ended_at?: string;
    window_started_at?: string;
  };

  type EvidenceStateResponseDTO = {
    algorithm_version?: string;
    calculated_at?: string;
    event_version?: number;
    id?: number;
    independent_origin_count?: number;
    reason_codes?: string[];
    state?: string;
  };

  type EvidenceSummaryResponseDTO = {
    created_at?: string;
    event_version?: number;
    id?: number;
    sentences?: EvidenceSummarySentenceResponseDTO[];
    summary_profile_version?: string;
  };

  type EvidenceSummarySentenceResponseDTO = {
    claim_evidence_version_ids?: number[];
    decision_origin?: string;
    editorial_note?: boolean;
    id?: number;
    ordinal?: number;
    text?: string;
  };

  type getAiModelProfilesIdParams = {
    /** model profile ID */
    id: number;
  };

  type getAiModelProfilesParams = {
    /** opaque signed model profile cursor */
    cursor?: string;
    /** page size */
    limit?: number;
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
    /** title or summary keyword */
    q?: string;
    /** source connection ID */
    source_connection_id?: number;
    /** published at or after (RFC3339) */
    published_from?: string;
    /** published at or before (RFC3339) */
    published_to?: string;
    /** monitor ID */
    monitor_id?: number;
    /** latest monitor match decision */
    decision?: "accepted" | "review" | "rejected";
    /** sort order */
    sort?:
      | "latest"
      | "discovered"
      | "published"
      | "importance"
      | "relevance"
      | "heat";
  };

  type getDocumentVersionsIdCitationParams = {
    /** document version ID */
    id: number;
  };

  type getDocumentVersionsIdDocumentParams = {
    /** document version ID */
    id: number;
  };

  type getHotspotsParams = {
    /** cursor */
    cursor?: string;
    /** page size */
    limit?: number;
    /** title or summary keyword */
    q?: string;
    /** source connection ID */
    source_connection_id?: number;
    /** published at or after (RFC3339) */
    published_from?: string;
    /** published at or before (RFC3339) */
    published_to?: string;
    /** monitor ID */
    monitor_id?: number;
    /** latest monitor match decision */
    decision?: "accepted" | "review" | "rejected";
    /** sort order */
    sort?: "discovered" | "published" | "importance" | "relevance" | "heat";
  };

  type getKnowledgeDocumentsIdParams = {
    /** document ID */
    id: number;
  };

  type getKnowledgeDocumentsParams = {
    /** opaque signed document cursor */
    cursor?: string;
    /** page size */
    limit?: number;
  };

  type getKnowledgeProposalsIdParams = {
    /** proposal ID */
    id: number;
  };

  type getKnowledgeProposalsParams = {
    /** opaque signed proposal cursor */
    cursor?: string;
    /** page size */
    limit?: number;
    /** proposal status */
    status?:
      | "pending"
      | "approved"
      | "rejected"
      | "conflict"
      | "applied"
      | "failed";
  };

  type getMicroEventsIdEvidenceParams = {
    /** micro-event ID */
    id: number;
    /** opaque evidence snapshot cursor */
    cursor?: string;
    /** page size */
    limit?: number;
  };

  type getMicroEventsIdParams = {
    /** micro-event ID */
    id: number;
  };

  type getMicroEventsParams = {
    /** opaque frozen-ranking cursor */
    cursor?: string;
    /** server ranking */
    sort?: "heat" | "relevance" | "latest";
    /** page size */
    limit?: number;
    /** comma-separated lifecycle states */
    status?: string;
    /** monitor relevance filter */
    monitor_id?: number;
    /** comma-separated source types */
    source_type?: string;
    /** comma-separated evidence states */
    evidence_state?: string;
    /** event start lower bound in RFC3339 */
    started_from?: string;
    /** event start upper bound in RFC3339 */
    started_to?: string;
  };

  type getMonitorsIdDocumentMatchesParams = {
    /** monitor ID */
    id: number;
    /** effective accepted, review, or rejected decision */
    decision?: string;
    /** opaque cursor */
    cursor?: string;
    /** page size, 1-100 */
    limit?: number;
  };

  type getMonitorsIdDraftExpansionRunsRunIdParams = {
    /** monitor ID */
    id: number;
    /** expansion run ID */
    run_id: number;
  };

  type getMonitorsIdDraftParams = {
    /** monitor ID */
    id: number;
  };

  type getMonitorsIdDraftPreviewRunsRunIdParams = {
    /** monitor ID */
    id: number;
    /** preview run ID */
    run_id: number;
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

  type getMonitorsIdScansParams = {
    /** monitor ID */
    id: number;
    /** cursor */
    cursor?: string;
    /** scan count */
    limit?: number;
  };

  type getMonitorsIdVersionsParams = {
    /** monitor ID */
    id: number;
    /** cursor */
    cursor?: string;
    /** page size */
    limit?: number;
  };

  type getMonitorsParams = {
    /** cursor */
    cursor?: string;
    /** page size */
    limit?: number;
  };

  type getNotificationsParams = {
    /** last processed user notification ID */
    after_id?: number;
    /** authorized monitor filter */
    monitor_id?: number;
    /** page size */
    limit?: number;
  };

  type getOperationsAuditLogsParams = {
    /** opaque signed audit cursor */
    cursor?: string;
    /** page size */
    limit?: number;
    /** exact action */
    action?: string;
    /** exact resource type */
    resource_type?: string;
    /** success, failure or denied */
    result?: string;
  };

  type getOperationsJobsParams = {
    /** opaque signed job snapshot cursor */
    cursor?: string;
    /** job kind */
    kind?: string;
    /** job state */
    state?: string;
    /** page size */
    limit?: number;
  };

  type getReportsIdParams = {
    /** report ID */
    id: number;
  };

  type getReportsParams = {
    /** opaque signed report cursor */
    cursor?: string;
    /** page size */
    limit?: number;
    /** daily or weekly */
    type?: string;
    /** draft, pending_approval, published, rejected, failed or archived */
    status?: string;
  };

  type getSearchParams = {
    /** lexical keyword */
    q: string;
    /** comma-separated content,event,knowledge */
    types?: string;
    /** source connection filter */
    source_connection_id?: number;
    /** monitor filter */
    monitor_id?: number;
    /** exact normalized entity */
    entity?: string;
    /** resource status */
    status?: string;
    /** relevance or latest */
    sort?: "relevance" | "latest";
    /** inclusive RFC3339 start */
    from?: string;
    /** inclusive RFC3339 end */
    to?: string;
    /** result limit */
    limit?: number;
    /** opaque signed search snapshot cursor */
    cursor?: string;
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

  type getSourceEndpointsIdCapabilitiesParams = {
    /** source endpoint ID */
    id: number;
  };

  type getSourceEndpointsIdRightsDecisionBatchesParams = {
    /** source endpoint ID */
    id: number;
    /** opaque cursor */
    cursor?: string;
    /** page size */
    limit?: number;
  };

  type getSourceEndpointsIdRightsDecisionsDecisionIdParams = {
    /** source endpoint ID */
    id: number;
    /** rights decision ID */
    decision_id: number;
  };

  type getSourceEndpointsIdRightsPoliciesParams = {
    /** source endpoint ID */
    id: number;
    /** opaque cursor */
    cursor?: string;
    /** page size */
    limit?: number;
  };

  type getUsersParams = {
    /** opaque signed user cursor */
    cursor?: string;
    /** page size */
    limit?: number;
    /** email or display name search */
    search?: string;
    /** role */
    role?: "admin" | "analyst" | "editor" | "viewer";
    /** lifecycle status */
    status?: "active" | "disabled" | "deleted";
  };

  type GovernanceResultArrayHttpRetentionPolicyResponse = {
    code?: number;
    data?: RetentionPolicyResponse[];
    message?: string;
  };

  type GovernanceResultDomainAuditPage = {
    code?: number;
    data?: AuditPage;
    message?: string;
  };

  type GovernanceResultDomainCleanupResult = {
    code?: number;
    data?: CleanupResult;
    message?: string;
  };

  type GovernanceResultDomainUsageOverview = {
    code?: number;
    data?: UsageOverview;
    message?: string;
  };

  type GovernanceResultInternalModulesOperationsTransportHttpEmptyResponse = {
    code?: number;
    data?: EmptyResponse;
    message?: string;
  };

  type HotspotCardResponse = {
    author?: string;
    canonical_url?: string;
    content_type?: string;
    discovered_at?: string;
    external_id?: string;
    heat_score?: number;
    id?: number;
    importance?: "low" | "medium" | "high" | "urgent";
    keyword_mentioned?: boolean;
    language?: string;
    metrics?: MetricsResponse;
    published_at?: string;
    quality_state?: "credible" | "suspicious" | "unavailable";
    relevance?: number;
    relevance_reason?: string;
    source_name?: string;
    source_type?: string;
    summary?: string;
    title?: string;
  };

  type HotspotPageResponse = {
    items?: HotspotCardResponse[];
    next_cursor?: string;
    summary?: HotspotSummaryResponse;
  };

  type HotspotSummaryResponse = {
    today?: number;
    total?: number;
    urgent?: number;
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

  type IdentityResultHttpUserPageResponse = {
    code?: number;
    data?: UserPageResponse;
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

  type InstantSearchRequest = {
    limit?: number;
    query: string;
    source_types?: string[];
  };

  type InstantSearchResponse = {
    query?: string;
    results?: HotspotCardResponse[];
    searched_at?: string;
    source_statuses?: InstantSearchSourceStatusResponse[];
  };

  type InstantSearchSourceStatusResponse = {
    error_code?: string;
    result_count?: number;
    source_name?: string;
    source_type?: string;
    state?: "success" | "empty" | "partial" | "failed" | "unavailable";
  };

  type IntentClauseRequestDTO = {
    field:
      | "term"
      | "phrase"
      | "action"
      | "location"
      | "language"
      | "region"
      | "source"
      | "time_window";
    operator: "must" | "should" | "must_not";
    value: string;
  };

  type IntentClauseResponseDTO = {
    field?: string;
    operator?: string;
    value?: string;
  };

  type IntentDraftResponseDTO = {
    candidates?: IntentExpansionCandidateResponseDTO[];
    clauses?: IntentClauseResponseDTO[];
    draft_id?: number;
    entities?: IntentEntityResponseDTO[];
    examples?: IntentExampleResponseDTO[];
    monitor_id?: number;
    objective?: string;
    resource_version?: number;
  };

  type IntentEntityRequestDTO = {
    aliases?: string[];
    ambiguity_note?: string;
    canonical_id: string;
    display_name: string;
  };

  type IntentEntityResponseDTO = {
    aliases?: string[];
    ambiguity_note?: string;
    canonical_id?: string;
    display_name?: string;
  };

  type IntentExampleRequestDTO = {
    label: "positive" | "negative";
    text: string;
  };

  type IntentExampleResponseDTO = {
    label?: string;
    text?: string;
  };

  type IntentExpansionCandidateResponseDTO = {
    approval_status?: string;
    id?: string;
    input_hash?: string;
    model_version?: string;
    prompt_version?: string;
    reason?: string;
    review_note?: string;
    reviewed_at?: string;
    reviewer_user_id?: number;
    risk?: string;
    similarity?: number;
    source?: string;
    value?: string;
  };

  type IntentExpansionRunStatusResponseDTO = {
    candidates?: IntentExpansionCandidateResponseDTO[];
    completed_at?: string;
    draft_id?: number;
    failure_code?: string;
    input_hash?: string;
    invalidated_at?: string;
    kind?: "expansion" | "preview";
    monitor_id?: number;
    queued_at?: string;
    resource_version?: number;
    run_id?: number;
    started_at?: string;
    status?: "queued" | "running" | "succeeded" | "failed" | "invalidated";
    status_url?: string;
  };

  type IntentPreviewRecallSignalResponseDTO = {
    channel?: string;
    rank?: number;
    /** RawScore is comparable only within the same recall channel. It is not a
probability or a cross-channel relevance percentage. */
    raw_score?: number;
  };

  type IntentPreviewResponseDTO = {
    estimated_alert_count?: number;
    samples?: IntentPreviewSampleResponseDTO[];
    warnings?: string[];
  };

  type IntentPreviewRunStatusResponseDTO = {
    completed_at?: string;
    draft_id?: number;
    failure_code?: string;
    input_hash?: string;
    invalidated_at?: string;
    kind?: "expansion" | "preview";
    monitor_id?: number;
    preview?: IntentPreviewResponseDTO;
    queued_at?: string;
    resource_version?: number;
    run_id?: number;
    started_at?: string;
    status?: "queued" | "running" | "succeeded" | "failed" | "invalidated";
    status_url?: string;
  };

  type IntentPreviewSampleResponseDTO = {
    decision?: string;
    document_version_id?: number;
    exclusion_reasons?: string[];
    reasons?: string[];
    recall_signals?: IntentPreviewRecallSignalResponseDTO[];
    title?: string;
  };

  type IntentRunAcceptedResponseDTO = {
    draft_id?: number;
    input_hash?: string;
    kind?: "expansion" | "preview";
    monitor_id?: number;
    resource_version?: number;
    reused?: boolean;
    run_id?: number;
    status?: string;
    status_url?: string;
  };

  type JobPageResponse = {
    items?: JobResponse[];
    next_cursor?: string;
  };

  type JobResponse = {
    attempt?: number;
    attempted_at?: string;
    created_at?: string;
    failure_code?: string;
    finalized_at?: string;
    id?: number;
    kind?: string;
    max_attempts?: number;
    priority?: number;
    resource_id?: number;
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
    expected_monitor_version: number;
  };

  type LocateTextQuoteSelectorRequestDTO = {
    exact_quote: string;
    normalization_version: string;
    plaintext_sha256: string;
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

  type ManualCollectionResponse = {
    cooldown_until?: string;
    created?: number;
    requested?: number;
    reused?: number;
  };

  type MetricCapabilityLifecycleRequest = {
    expected_version: number;
    reason_code: string;
  };

  type MetricCapabilityProfileResponse = {
    archived_at?: string;
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

  type MetricsResponse = {
    comment_count?: number;
    like_count?: number;
    share_count?: number;
    view_count?: number;
  };

  type MicroEventEvidencePageResponseDTO = {
    items?: ClaimEvidenceResponseDTO[];
    next_cursor?: string;
  };

  type MicroEventGovernanceRequestDTO = {
    action: string;
    content_family_id?: number;
    expected_event_version: number;
    expected_member_version?: number;
    expected_target_event_version?: number;
    membership_decision_id?: number;
    note?: string;
    reason_code: string;
    target_micro_event_id?: number;
  };

  type MicroEventGovernanceResponseDTO = {
    feedback_id?: number;
    source_event?: MicroEventGovernanceResultDTO;
    target_event?: MicroEventGovernanceResultDTO;
  };

  type MicroEventGovernanceResultDTO = {
    id?: number;
    status?: string;
    version?: number;
  };

  type MicroEventMemberResponseDTO = {
    clustering_profile_version?: string;
    content_family_id?: number;
    id?: number;
    membership_decision_id?: number;
    version?: number;
  };

  type MicroEventPageResponseDTO = {
    items?: MicroEventResponseDTO[];
    next_cursor?: string;
  };

  type MicroEventResponseDTO = {
    clustering_profile_version?: string;
    content_family_count?: number;
    document_count?: number;
    event_ended_at?: string;
    event_key?: string;
    event_started_at?: string;
    evidence_state?: EvidenceStateResponseDTO;
    evidence_summary?: EvidenceSummaryResponseDTO;
    id?: number;
    identifier_keys?: string[];
    latest_heat?: EventHeatV2ResponseDTO;
    location_keys?: string[];
    members?: MicroEventMemberResponseDTO[];
    primary_action_key?: string;
    primary_subject_key?: string;
    relevance_score?: number;
    status?: string;
    storyline?: StorylineResponseDTO;
    version?: number;
  };

  type MicroEventV2ResultHttpClaimEvidenceCorrectionResponseDTO = {
    code?: number;
    data?: ClaimEvidenceCorrectionResponseDTO;
    message?: string;
  };

  type MicroEventV2ResultHttpClaimEvidenceMutationResponseDTO = {
    code?: number;
    data?: ClaimEvidenceMutationResponseDTO;
    message?: string;
  };

  type MicroEventV2ResultHttpMicroEventEvidencePageResponseDTO = {
    code?: number;
    data?: MicroEventEvidencePageResponseDTO;
    message?: string;
  };

  type MicroEventV2ResultHttpMicroEventGovernanceResponseDTO = {
    code?: number;
    data?: MicroEventGovernanceResponseDTO;
    message?: string;
  };

  type MicroEventV2ResultHttpMicroEventPageResponseDTO = {
    code?: number;
    data?: MicroEventPageResponseDTO;
    message?: string;
  };

  type MicroEventV2ResultHttpMicroEventResponseDTO = {
    code?: number;
    data?: MicroEventResponseDTO;
    message?: string;
  };

  type MicroEventV2ResultInternalModulesEventTransportHttpEmptyResponse = {
    code?: number;
    data?: EmptyResponse;
    message?: string;
  };

  type ModelProfileListResponse = {
    items?: ModelProfileResponse[];
    next_cursor?: string;
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

  type ModelProfileResultHttpAIRunRecomputeResponse = {
    code?: number;
    data?: AIRunRecomputeResponse;
    message?: string;
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
    alert_cooldown_minutes?: number;
    alert_critical_threshold?: number;
    alert_email_enabled?: boolean;
    alert_email_min_severity?: "warning" | "critical";
    alert_min_breadth?: number;
    alert_min_heat?: number;
    alert_min_momentum?: number;
    alert_warning_threshold?: number;
    collection_interval_seconds: number;
    event_threshold: number;
    languages: string[];
    regions?: string[];
    relevance_threshold: number;
    retention_days: number;
    timezone: string;
  };

  type MonitorConfigResponse = {
    alert_cooldown_minutes?: number;
    alert_critical_threshold?: number;
    alert_email_enabled?: boolean;
    alert_email_min_severity?: string;
    alert_min_breadth?: number;
    alert_min_heat?: number;
    alert_min_momentum?: number;
    alert_warning_threshold?: number;
    collection_interval_seconds?: number;
    config_hash?: string;
    event_threshold?: number;
    id?: number;
    languages?: string[];
    published_at?: string;
    regions?: string[];
    relevance_threshold?: number;
    retention_days?: number;
    revision?: number;
    rules?: MonitorRuleResponse[];
    sources?: MonitorSourceResponse[];
    state?: string;
    timezone?: string;
    version?: number;
  };

  type MonitorPageResponse = {
    items?: MonitorResponse[];
    next_cursor?: string;
  };

  type MonitorQuotaErrorResponse = {
    code?: number;
    data?: {
      dimension?: string;
      limit?: number;
      remaining?: number;
      reset_at?: string;
    };
    message?: string;
  };

  type MonitorResponse = {
    alert_email_enabled?: boolean;
    collection_interval_seconds?: number;
    created_by_user_id?: number;
    description?: string;
    id?: number;
    name?: string;
    query?: string;
    sources?: MonitorSourceResponse[];
    status?: string;
    version?: number;
  };

  type MonitorResultHttpIntentDraftResponseDTO = {
    code?: number;
    data?: IntentDraftResponseDTO;
    message?: string;
  };

  type MonitorResultHttpIntentExpansionRunStatusResponseDTO = {
    code?: number;
    data?: IntentExpansionRunStatusResponseDTO;
    message?: string;
  };

  type MonitorResultHttpIntentPreviewRunStatusResponseDTO = {
    code?: number;
    data?: IntentPreviewRunStatusResponseDTO;
    message?: string;
  };

  type MonitorResultHttpIntentRunAcceptedResponseDTO = {
    code?: number;
    data?: IntentRunAcceptedResponseDTO;
    message?: string;
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

  type MonitorResultHttpMonitorVersionHistoryResponse = {
    code?: number;
    data?: MonitorVersionHistoryResponse;
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

  type MonitorScanPageResponse = {
    items?: MonitorScanResponse[];
    next_cursor?: string;
  };

  type MonitorScanResponse = {
    accepted_count?: number;
    candidate_count?: number;
    finished_at?: string;
    id?: string;
    monitor_id?: number;
    rejected_count?: number;
    run_outcome?: "success" | "partial_success" | "failed";
    scheduled_at?: string;
    sources?: MonitorScanSourceResponse[];
    started_at?: string;
    status?: "queued" | "running" | "succeeded" | "partial" | "failed";
    trigger_type?: "schedule" | "manual" | "retry" | "reconcile";
  };

  type MonitorScanSourceResponse = {
    accepted_count?: number;
    candidate_count?: number;
    error_code?: string;
    finished_at?: string;
    rejected_count?: number;
    run_id?: number;
    scheduled_at?: string;
    source_connection_id?: number;
    source_name?: string;
    source_type?: string;
    started_at?: string;
    status?: "queued" | "running" | "succeeded" | "failed" | "cancelled";
    trigger_type?: "schedule" | "manual" | "retry" | "reconcile";
  };

  type MonitorSourceRequest = {
    enabled?: boolean;
    priority?: number;
    query_override?: string;
    source_connection_id: number;
  };

  type MonitorSourceResponse = {
    enabled?: boolean;
    name?: string;
    source_connection_id?: number;
    source_type?: string;
  };

  type MonitorVersionHistoryResponse = {
    items?: MonitorConfigResponse[];
    next_cursor?: string;
  };

  type NotificationReadReceiptResponseDTO = {
    advanced?: boolean;
    read_through_id?: number;
    receipt_id?: number;
    recorded_at?: string;
  };

  type NotificationResultHttpEmptyResponseDTO = {
    code?: number;
    data?: EmptyResponseDTO;
    message?: string;
  };

  type NotificationResultHttpNotificationReadReceiptResponseDTO = {
    code?: number;
    data?: NotificationReadReceiptResponseDTO;
    message?: string;
  };

  type NotificationResultHttpUserNotificationPageResponseDTO = {
    code?: number;
    data?: UserNotificationPageResponseDTO;
    message?: string;
  };

  type OverrideDocumentMatchRequestDTO = {
    decision: "accepted" | "rejected";
    note?: string;
    reason_code: string;
  };

  type OverrideDocumentMatchResponseDTO = {
    actor_user_id?: number;
    created_at?: string;
    decision?: "accepted" | "rejected";
    document_version_id?: number;
    match_decision_id?: number;
    monitor_id?: number;
    monitor_version_id?: number;
    note?: string;
    override_id?: number;
    previous_effective_decision?: "accepted" | "review" | "rejected";
    reason_code?: string;
    resource_version?: number;
    reused?: boolean;
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

  type postAiRunsIdRecomputeParams = {
    /** AI run ID */
    id: number;
  };

  type postCollectionRunsIdRetryParams = {
    /** collection run ID */
    id: number;
  };

  type postContentLineageDecisionsIdFeedbackParams = {
    /** lineage decision ID */
    id: number;
  };

  type postDocumentVersionsIdTextQuoteSelectorsParams = {
    /** document version ID */
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

  type postMicroEventsIdEvidenceEvidenceIdFeedbackParams = {
    /** micro-event ID */
    id: number;
    /** original claim evidence version ID */
    evidence_id: number;
  };

  type postMicroEventsIdEvidenceParams = {
    /** micro-event ID */
    id: number;
  };

  type postMicroEventsIdFeedbackParams = {
    /** micro-event ID */
    id: number;
  };

  type postMonitorsIdArchiveParams = {
    /** monitor ID */
    id: number;
  };

  type postMonitorsIdCollectParams = {
    /** monitor ID */
    id: number;
  };

  type postMonitorsIdDocumentMatchesMatchDecisionIdOverridesParams = {
    /** monitor ID */
    id: number;
    /** automatic match decision ID */
    match_decision_id: number;
  };

  type postMonitorsIdDraftAiCandidatesParams = {
    /** monitor ID */
    id: number;
  };

  type postMonitorsIdDraftExpansionCandidatesCandidateIdDecisionParams = {
    /** monitor ID */
    id: number;
    /** expansion candidate ID */
    candidate_id: string;
  };

  type postMonitorsIdDraftExpansionRunsParams = {
    /** monitor ID */
    id: number;
  };

  type postMonitorsIdDraftPreviewRunsParams = {
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

  type postOperationsRetentionPoliciesIdPreviewParams = {
    /** retention policy ID */
    id: number;
  };

  type postOperationsRetentionRunsIdApproveParams = {
    /** retention run ID */
    id: number;
  };

  type postOperationsRetentionRunsIdExecuteParams = {
    /** retention run ID */
    id: number;
  };

  type postReportsIdApproveParams = {
    /** report ID */
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

  type postReportsIdRejectParams = {
    /** report ID */
    id: number;
  };

  type postReportsIdSubmitParams = {
    /** report ID */
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

  type postSourceEndpointsIdRightsDecisionBatchesParams = {
    /** source endpoint ID */
    id: number;
  };

  type postSourceEndpointsIdRightsEvaluationsParams = {
    /** source endpoint ID */
    id: number;
  };

  type postSourceEndpointsIdRightsPoliciesParams = {
    /** source endpoint ID */
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
    compiled_query?: string;
    estimated_requests?: number;
    excluded_rule_ids?: number[];
    excluded_term_count?: number;
    included_rule_ids?: number[];
    included_term_count?: number;
    languages?: string[];
    max_query_bytes?: number;
    query_mode?: string;
    query_signature?: string;
    regions?: string[];
    source_connection_id?: number;
    unapproved_rule_ids?: number[];
  };

  type ProposalPageResponse = {
    items?: ProposalResponse[];
    next_cursor?: string;
  };

  type ProposalRequest = {
    base_hash?: string;
    base_revision?: number;
    body?: string;
    document_id?: number;
    frontmatter?: string;
    reason?: string;
  };

  type ProposalResponse = {
    baseHash?: string;
    baseRevisionNo?: number;
    diffSummary?: string;
    documentID?: number;
    id?: number;
    proposedBody?: string;
    proposedFrontmatter?: string;
    reason?: string;
    status?: string;
    version?: number;
  };

  type ProposalResultHttpDocumentPageResponse = {
    code?: number;
    data?: DocumentPageResponse;
    message?: string;
  };

  type ProposalResultHttpDocumentResponse = {
    code?: number;
    data?: DocumentResponse;
    message?: string;
  };

  type ProposalResultHttpProposalPageResponse = {
    code?: number;
    data?: ProposalPageResponse;
    message?: string;
  };

  type ProposalResultHttpProposalResponse = {
    code?: number;
    data?: ProposalResponse;
    message?: string;
  };

  type ProposalResultHttpReconciliationResponse = {
    code?: number;
    data?: ReconciliationResponse;
    message?: string;
  };

  type ProposalResultInternalModulesKnowledgeTransportHttpEmptyResponse = {
    code?: number;
    data?: EmptyResponse;
    message?: string;
  };

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

  type putMonitorsIdDraftIntentParams = {
    /** monitor ID */
    id: number;
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

  type putMonitorsIdParams = {
    /** monitor ID */
    id: number;
  };

  type ReconciliationIssueResponse = {
    actualHash?: string;
    expectedHash?: string;
    kind?: string;
    path?: string;
  };

  type ReconciliationResponse = {
    changed?: number;
    conflict?: number;
    issues?: ReconciliationIssueResponse[];
    scanned?: number;
  };

  type RecordClaimEvidenceRequestDTO = {
    document_version_id: number;
    expected_event_version: number;
    object: string;
    predicate: string;
    qualifiers?: ClaimQualifierRequestDTO[];
    relation: string;
    subject: string;
    text_quote_selector_id: number;
  };

  type RecordNotificationReadReceiptRequest = {
    read_through_id: number;
  };

  type RecordRightsDecisionBatchRequestDTO = {
    decisions?: RightsActionDecisionRequestDTO[];
    expected_policy_version?: number;
    input_digest?: string;
    policy_id?: number;
    subject_key?: string;
    subject_type?: string;
  };

  type RecordRightsDecisionBatchResponseDTO = {
    decision_batch_id?: number;
    decisions?: RightsDecisionResponseDTO[];
    idempotent_replay?: boolean;
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

  type ReplaceIntentDraftRequestDTO = {
    clauses?: IntentClauseRequestDTO[];
    entities?: IntentEntityRequestDTO[];
    examples?: IntentExampleRequestDTO[];
    expected_resource_version?: number;
    objective: string;
  };

  type ReportItemResponse = {
    event_id?: number;
    event_update_id?: number;
    evidence_set_hash?: string;
    heat_score?: number;
    inclusion_reason?: string;
    micro_event_id?: number;
    micro_event_summary_id?: number;
    micro_event_update_id?: number;
    micro_event_version?: number;
    rank?: number;
    reason_codes?: string[];
    sentences?: ReportSentenceResponse[];
    summary?: string;
    title?: string;
  };

  type ReportPageResponse = {
    items?: ReportResponse[];
    next_cursor?: string;
  };

  type ReportPreviewResponse = {
    approvable?: boolean;
    report?: ReportResponse;
    submittable?: boolean;
  };

  type ReportResponse = {
    body?: string;
    created_by?: number;
    frozen?: boolean;
    generated_at?: string;
    id?: number;
    input_snapshot_hash?: string;
    items?: ReportItemResponse[];
    monitor_id?: number;
    period_end?: string;
    period_start?: string;
    published_at?: string;
    review_reason?: string;
    reviewed_at?: string;
    reviewed_by?: number;
    status?: string;
    submitted_at?: string;
    submitted_by?: number;
    summary?: string;
    timezone?: string;
    title?: string;
    type?: string;
    updated_by?: number;
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

  type ReportRevisionLifecycleRequest = {
    expected_resource_version: number;
    reason_code?: string;
  };

  type ReportSentenceResponse = {
    actor_user_id?: number;
    claim_evidence_version_ids?: number[];
    decision_origin?: string;
    editorial_note?: boolean;
    model_run_id?: number;
    ordinal?: number;
    source_summary_sentence_id?: number;
    text?: string;
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

  type RetentionConfirmationRequest = {
    candidate_hash: string;
  };

  type RetentionPolicyResponse = {
    action?: string;
    data_class?: string;
    description?: string;
    enabled?: boolean;
    id?: number;
    protected?: boolean;
    retention_days?: number;
    version?: number;
  };

  type RetentionPreviewRequest = {
    batch_size: number;
    expected_version: number;
  };

  type ReviewContentLineageRequestDTO = {
    expected_member_version: number;
    expected_target_member_version?: number;
    feedback_type: string;
    note?: string;
    reason_code: string;
    relation_override?: string;
    target_parent_document_version_id?: number;
  };

  type ReviewIntentExpansionCandidateRequestDTO = {
    decision: "approved" | "rejected";
    expected_resource_version: number;
    note?: string;
  };

  type RightsActionCapabilityResponseDTO = {
    action?: string;
    decision?: string;
    decision_ids?: number[];
    policy_ids?: number[];
    priority?: number;
    retention_days?: number;
  };

  type RightsActionDecisionRequestDTO = {
    action?: string;
    decision?: string;
    effective_from?: string;
    evaluated_at?: string;
    evaluator?: string;
    expires_at?: string;
    reason_codes?: string[];
    retention_days?: number;
    supersedes_decision_id?: number;
  };

  type RightsActionMatrixResponseDTO = {
    actions?: RightsActionCapabilityResponseDTO[];
    evaluated_at?: string;
    source_endpoint_id?: number;
  };

  type RightsDecisionBatchPageResponseDTO = {
    items?: RightsDecisionBatchResponseDTO[];
    next_cursor?: string;
  };

  type RightsDecisionBatchResponseDTO = {
    created_at?: string;
    decision_count?: number;
    decisions?: RightsDecisionResponseDTO[];
    expected_policy_version?: number;
    id?: number;
    input_digest?: string;
    policy_id?: number;
    recorded_by_user_id?: number;
    source_endpoint_id?: number;
    subject_key?: string;
    subject_type?: string;
    version?: number;
  };

  type RightsDecisionResponseDTO = {
    action?: string;
    basis_summary?: string;
    created_at?: string;
    decision?: string;
    decision_batch_id?: number;
    effective_from?: string;
    evaluated_at?: string;
    evaluator?: string;
    expires_at?: string;
    id?: number;
    input_digest?: string;
    license_uri?: string;
    policy_id?: number;
    policy_revision?: number;
    policy_scope_subject?: string;
    policy_scope_type?: string;
    priority?: number;
    reason_codes?: string[];
    recorded_by_user_id?: number;
    retention_days?: number;
    source_endpoint_id?: number;
    subject_key?: string;
    subject_type?: string;
    supersedes_decision_id?: number;
    terms_url?: string;
  };

  type RightsPolicyPageResponseDTO = {
    items?: RightsPolicyResponseDTO[];
    next_cursor?: string;
  };

  type RightsPolicyResponseDTO = {
    approved_by_user_id?: number;
    basis_summary?: string;
    created_at?: string;
    effective_from?: string;
    expires_at?: string;
    id?: number;
    license_uri?: string;
    parent_policy_id?: number;
    policy_hash?: string;
    priority?: number;
    recorded_by_user_id?: number;
    revision?: number;
    scope_subject?: string;
    scope_type?: string;
    source_endpoint_id?: number;
    terms_url?: string;
    version?: number;
  };

  type RuntimeAlert = {
    affected_count?: number;
    alert_id?: string;
    attempt_id?: number;
    event_id?: number;
    job_id?: number;
    notification_id?: number;
    owner?: string;
    policy_version?: string;
    reason_code?: string;
    resource_id?: number;
    resource_type?: string;
    runbook_url?: string;
    severity?: string;
    silence_key?: string;
    threshold_count?: number;
    threshold_seconds?: number;
    trace_id?: string;
    triggered_at?: string;
  };

  type RuntimeOverview = {
    alert_policy_version?: string;
    alerts?: RuntimeAlert[];
    available_jobs?: number;
    cancelled_jobs?: number;
    completed_jobs?: number;
    discarded_jobs?: number;
    generated_at?: string;
    oldest_available_at?: string;
    queue_lag_seconds?: number;
    running_jobs?: number;
  };

  type SearchEmptyResponseDTO = true;

  type SearchItemResponseDTO = {
    id?: number;
    occurred_at?: string;
    score?: number;
    snippet?: string;
    snippet_highlight?: string;
    status?: string;
    title?: string;
    title_highlight?: string;
    type?: string;
  };

  type SearchPageResponseDTO = {
    items?: SearchItemResponseDTO[];
    next_cursor?: string;
  };

  type SearchResultHttpSearchEmptyResponseDTO = {
    code?: number;
    data?: SearchEmptyResponseDTO;
    message?: string;
  };

  type SearchResultHttpSearchPageResponseDTO = {
    code?: number;
    data?: SearchPageResponseDTO;
    message?: string;
  };

  type SourceConfigDTO = {
    /** Deprecated: informational legacy configuration only; not a rights grant. */
    allow_body_storage?: boolean;
    allowed_languages?: string[];
    allowed_regions?: string[];
    bilibili_open_id?: string;
    content_retention_days?: number;
    google_location?: string;
    google_serving_config?: string;
    grounding_data_boundary_approved?: boolean;
    hacker_news_mode?: string;
    max_pages_per_run?: number;
    metrics_retention_days?: number;
    rate_limit_per_minute?: number;
    request_timeout_seconds?: number;
    requires_attribution?: boolean;
    requires_deletion_sync?: boolean;
    x_metric_refresh_daily_request_budget?: number;
    x_metric_refresh_enabled?: boolean;
    x_metric_refresh_interval_minutes?: number;
    x_metric_refresh_max_posts_per_run?: number;
    x_metric_refresh_observation_hours?: number;
  };

  type SourceConfigRequest = {
    /** Deprecated: retained for legacy source configuration compatibility. This
value never authorizes v2 raw evidence or document body persistence. */
    allow_body_storage?: boolean;
    allowed_languages?: string[];
    allowed_regions?: string[];
    bilibili_open_id?: string;
    content_retention_days?: number;
    google_location?: string;
    google_serving_config?: string;
    grounding_data_boundary_approved?: boolean;
    hacker_news_mode?: "new" | "top" | "best";
    max_pages_per_run?: number;
    metrics_retention_days?: number;
    rate_limit_per_minute?: number;
    request_timeout_seconds?: number;
    requires_attribution?: boolean;
    requires_deletion_sync?: boolean;
    x_metric_refresh_daily_request_budget?: number;
    x_metric_refresh_enabled?: boolean;
    x_metric_refresh_interval_minutes?: number;
    x_metric_refresh_max_posts_per_run?: number;
    x_metric_refresh_observation_hours?: number;
  };

  type SourceEndpointCapabilityResponseDTO = {
    availability?: string;
    collection_interface?: string;
    content_scope?: string;
    default_access_mode?: string;
    document_capture_mode?: string;
    follows_canonical_url?: boolean;
    required_actions?: string[];
    rights_status?: string;
    source_endpoint_id?: number;
    source_type?: string;
  };

  type SourceHealthResponse = {
    checked_at?: string;
    error_code?: string;
    healthy?: boolean;
  };

  type SourceLifecycleRequest = {
    expected_source_version: number;
  };

  type SourcePresetInputResponse = {
    key?: string;
    label?: string;
    max_length?: number;
    placeholder?: string;
    required?: boolean;
  };

  type SourcePresetPageResponse = {
    items?: SourcePresetResponse[];
  };

  type SourcePresetResponse = {
    auth_label?: string;
    cost?: "free" | "paid" | "credentialed";
    credential_required?: boolean;
    description?: string;
    id?: string;
    inputs?: SourcePresetInputResponse[];
    label?: string;
    source_type?: string;
  };

  type SourcePresetValueRequest = {
    key: string;
    value: string;
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

  type SourceResultHttpCreateRightsPolicyResponseDTO = {
    code?: number;
    data?: CreateRightsPolicyResponseDTO;
    message?: string;
  };

  type SourceResultHttpInstantSearchResponse = {
    code?: number;
    data?: InstantSearchResponse;
    message?: string;
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

  type SourceResultHttpRecordRightsDecisionBatchResponseDTO = {
    code?: number;
    data?: RecordRightsDecisionBatchResponseDTO;
    message?: string;
  };

  type SourceResultHttpRightsActionMatrixResponseDTO = {
    code?: number;
    data?: RightsActionMatrixResponseDTO;
    message?: string;
  };

  type SourceResultHttpRightsDecisionBatchPageResponseDTO = {
    code?: number;
    data?: RightsDecisionBatchPageResponseDTO;
    message?: string;
  };

  type SourceResultHttpRightsDecisionResponseDTO = {
    code?: number;
    data?: RightsDecisionResponseDTO;
    message?: string;
  };

  type SourceResultHttpRightsPolicyPageResponseDTO = {
    code?: number;
    data?: RightsPolicyPageResponseDTO;
    message?: string;
  };

  type SourceResultHttpSourceEndpointCapabilityResponseDTO = {
    code?: number;
    data?: SourceEndpointCapabilityResponseDTO;
    message?: string;
  };

  type SourceResultHttpSourcePresetPageResponse = {
    code?: number;
    data?: SourcePresetPageResponse;
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

  type StorylineResponseDTO = {
    id?: number;
    relation_profile_version?: string;
    status?: string;
    storyline_key?: string;
    summary?: string;
    title?: string;
    version?: number;
  };

  type SubmitIntentExpansionRunRequestDTO = {
    expansion_profile: "monitor-intent-expansion-v1";
    expected_resource_version: number;
  };

  type SubmitIntentPreviewRunRequestDTO = {
    evaluator_profile: string;
    expected_resource_version: number;
    sample_limit: number;
  };

  type TextQuoteSelectorResponseDTO = {
    document_version_id?: number;
    exact_quote?: string;
    id?: number;
    markdown_anchor?: string;
    normalization_version?: string;
    plaintext_sha256?: string;
    prefix?: string;
    quote_sha256?: string;
    retention_until?: string;
    selector_version?: string;
    suffix?: string;
    utf8_byte_end?: number;
    utf8_byte_start?: number;
    version?: number;
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

  type UpdateMonitorRequest = {
    alert_email_enabled: boolean;
    collection_interval_seconds: number;
    expected_monitor_version: number;
    name: string;
    query: string;
    source_connection_ids: number[];
  };

  type UpdateSourceRequest = {
    auth_type?: "none" | "api_key" | "oauth2" | "bearer";
    config?: SourceConfigRequest;
    credential?: string;
    credential_ref?: string;
    endpoint?: string;
    expected_source_version: number;
    name?: string;
    source_type?:
      | "rss"
      | "hacker_news"
      | "x"
      | "bing_grounding"
      | "bilibili"
      | "weibo"
      | "google_agent_search";
    terms_policy_url?: string;
  };

  type UpdateUserRequest = {
    role?: "admin" | "analyst" | "editor" | "viewer";
    status?: "active" | "disabled";
  };

  type UsageItem = {
    dimension?: string;
    label?: string;
    limit?: string;
    mode?: string;
    remaining?: string;
    reserved?: string;
    reset_at?: string;
    scope?: string;
    settled?: string;
    unit?: string;
    used?: string;
  };

  type UsageOverview = {
    generated_at?: string;
    items?: UsageItem[];
  };

  type UserNotificationPageResponseDTO = {
    items?: UserNotificationResponseDTO[];
    next_after_id?: number;
    read_through_id?: number;
  };

  type UserNotificationResponseDTO = {
    created_at?: string;
    deep_link?: string;
    event_type?: string;
    id?: number;
    monitor_id?: number;
    occurred_at?: string;
    resource_id?: number;
    resource_status?: string;
    resource_type?: string;
    resource_version?: number;
    summary?: string;
    title?: string;
    version?: number;
  };

  type UserPageResponse = {
    items?: UserResponse[];
    next_cursor?: string;
  };

  type UserResponse = {
    created_at?: string;
    deleted_at?: string;
    display_name?: string;
    email?: string;
    id?: number;
    last_login_at?: string;
    role?: "admin" | "analyst" | "editor" | "viewer";
    status?: "active" | "disabled";
    updated_at?: string;
  };

  type VersionedDocumentResponseDTO = {
    citation?: CitationResponseDTO;
    etag?: string;
    markdown?: string;
  };
}
