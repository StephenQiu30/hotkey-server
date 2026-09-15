declare namespace API {
  type cancelJobParams = {
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
    kind: string;
    /** Status */
    status: "queued" | "running" | "succeeded" | "failed" | "cancelled";
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
    /** Keywords */
    keywords: string[];
    /** Sources */
    sources: (
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

  type MonitorUpdate = {
    /** Keywords */
    keywords: string[];
    /** Sources */
    sources: (
      | "x"
      | "bilibili"
      | "weibo"
      | "xiaohongshu"
      | "douyin"
      | "bluesky"
    )[];
    /** Title */
    title: string;
    /** Version */
    version: number;
  };

  type MonitorView = {
    /** Created At */
    created_at: string;
    /** Id */
    id: string;
    /** Keywords */
    keywords: string[];
    /** Sources */
    sources: string[];
    /** Status */
    status?: string;
    /** Title */
    title: string;
    /** Version */
    version: number;
  };

  type Principal = {
    /** Username */
    username: string;
  };

  type QueryPreview = {
    /** Adapter Version */
    adapter_version?: string;
    /** Coverage */
    coverage?: string;
    /** Limit */
    limit: number;
    /** Operation */
    operation?: string;
    /** Pipeline Connected */
    pipeline_connected?: boolean;
    /** Query */
    query: string;
    /** Semantics */
    semantics?: string;
    /** Since */
    since: string;
    /** Sort */
    sort?: string;
    /** Source */
    source?: string;
    /** Until */
    until: string;
  };

  type SearchInput = {
    /** Cursor */
    cursor?: string | null;
    /** Keyword */
    keyword: string;
    /** Limit */
    limit?: number;
    /** Since */
    since: string;
    /** Until */
    until: string;
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
    /** Evidence Ref */
    evidence_ref: string;
    /** Note */
    note: string;
    /** Operation */
    operation: "search_posts" | "fetch_post" | "list_comments" | "list_replies";
    /** Rights */
    rights: "unknown" | "allowed" | "denied";
    /** Support */
    support: "unknown" | "supported" | "unsupported" | "authorization_required";
    /** Verified At */
    verified_at: string;
  };

  type SourceView = {
    /** Eligible For Collection */
    eligible_for_collection: boolean;
    /** Id */
    id: "x" | "bilibili" | "weibo" | "xiaohongshu" | "douyin" | "bluesky";
    /** Operations */
    operations: SourceOperationCapability[];
    /** Pipeline */
    pipeline: "not_connected" | "connected" | "degraded" | "paused";
    /** Roles */
    roles: ("discovery" | "comments" | "supplement")[];
  };

  type updateMonitorParams = {
    identity: string;
  };
}
