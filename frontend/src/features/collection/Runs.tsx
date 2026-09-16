import { sourceLabels } from "../monitors/MonitorEditor";

type Run = API.CollectionRunView;

const stateLabels: Record<Run["state"], string> = {
  queued: "排队中",
  running: "采集中",
  completed: "已完成",
  failed: "失败",
  cancelled: "已取消",
};

const operationLabels: Record<Run["operation"], string> = {
  search_posts: "搜索",
  fetch_post: "正文",
  list_comments: "根评论",
  list_replies: "回复",
};

const ingestionModes: Record<Run["ingestion_mode"], string> = {
  live: "实时",
  backfill: "历史回填",
};

function time(value: string): string {
  return new Intl.DateTimeFormat("zh-CN", {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(value));
}

export function Runs({
  items,
  nextCursor,
  busy,
  onMore,
  onCancel,
}: {
  items: Run[];
  nextCursor: string | null;
  busy: boolean;
  onMore: () => void;
  onCancel: (jobId: string) => void;
}) {
  return (
    <section aria-labelledby="runs-heading">
      <div className="section-title">
        <div>
          <h2 id="runs-heading">
            采集运行 <span className="count">{items.length}</span>
          </h2>
          <p className="muted small">
            记录周期槽、预算占用和实际停止原因；页面读取不会触发采集。
          </p>
        </div>
      </div>
      {items.length ? (
        <div className="inbox-list">
          {items.map((run) => (
            <article className="panel inbox-item" key={run.id}>
              <div className="inbox-meta">
                <span className="badge">
                  {stateLabels[run.state]} ·{" "}
                  {sourceLabels[run.source] ?? run.source}
                </span>
                <span className="muted small">
                  {run.trigger === "scheduled" ? "周期任务" : "手动任务"} ·{" "}
                  {ingestionModes[run.ingestion_mode]} · 预算 {run.budget_day} /{" "}
                  {run.reserved_requests} 次
                </span>
              </div>
              <h3>
                {operationLabels[run.operation]}：{run.request_value}
              </h3>
              {run.parent_run_id && (
                <p className="muted small">父运行 {run.parent_run_id}</p>
              )}
              <p className="muted small">
                窗口 {time(run.window_since)} 至 {time(run.window_until)} · 页数{" "}
                {run.pages_count}· 内容 {run.items_count} · 证据{" "}
                {run.bytes_count} 字节
              </p>
              <p className="muted small">
                结果 {run.outcome ?? "等待执行"} · 停止原因{" "}
                {run.stop_reason ?? "无"}
              </p>
              {(run.state === "queued" || run.state === "running") && (
                <div className="actions">
                  <button
                    className="secondary"
                    disabled={busy}
                    onClick={() => onCancel(run.job_id)}
                    aria-label={`取消采集 ${run.request_value}`}
                  >
                    取消采集
                  </button>
                </div>
              )}
            </article>
          ))}
        </div>
      ) : (
        <div className="empty">
          <h3>还没有采集运行</h3>
          <p>来源和现有 MinIO 通过准入后，周期任务会在这里留下真实状态。</p>
        </div>
      )}
      {nextCursor && (
        <button disabled={busy} onClick={onMore}>
          更多运行
        </button>
      )}
    </section>
  );
}
