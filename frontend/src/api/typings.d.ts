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
      | "weibo"
      | "bilibili"
      | "xiaohongshu"
      | "douyin"
      | "zhihu"
      | "kuaishou"
      | "wechat"
      | "youtube"
      | "bluesky"
      | "x"
      | "reddit"
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
      | "weibo"
      | "bilibili"
      | "xiaohongshu"
      | "douyin"
      | "zhihu"
      | "kuaishou"
      | "wechat"
      | "youtube"
      | "bluesky"
      | "x"
      | "reddit"
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

  type SourceView = {
    /** Comments */
    comments?: string;
    /** Id */
    id:
      | "weibo"
      | "bilibili"
      | "xiaohongshu"
      | "douyin"
      | "zhihu"
      | "kuaishou"
      | "wechat"
      | "youtube"
      | "bluesky"
      | "x"
      | "reddit";
    /** Search */
    search?: string;
  };

  type updateMonitorParams = {
    identity: string;
  };
}
