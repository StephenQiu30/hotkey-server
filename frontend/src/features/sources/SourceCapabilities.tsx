type Source = API.SourceView;
type Capability = API.SourceOperationCapability;
type Runtime = API.SourceOperationRuntime;

const sourceLabels: Record<Source["id"], string> = {
  x: "X",
  bilibili: "B站",
  weibo: "微博",
  xiaohongshu: "小红书",
  douyin: "抖音",
  bluesky: "Bluesky",
};

const operationLabels: Record<Capability["operation"], string> = {
  search_posts: "关键词发现",
  fetch_post: "帖子详情",
  list_comments: "根评论",
  list_replies: "评论回复",
};

const supportLabels: Record<Capability["support"], string> = {
  supported: "技术可读",
  authorization_required: "需要授权",
  unknown: "尚未验证",
  unsupported: "不支持",
};

const rightsLabels: Record<Capability["rights"], string> = {
  unknown: "权限待核对",
  allowed: "用途已授权",
  denied: "用途禁止",
};

const pipelineLabels: Record<Capability["pipeline"], string> = {
  not_connected: "未连接",
  connected: "已连接",
  degraded: "连接异常",
  paused: "已暂停",
};

const recoveryLabels: Record<
  Exclude<Runtime["recovery_action"], null | undefined>,
  string
> = {
  refresh_authorization: "更新授权后执行有界重试",
  wait_for_rate_limit: "等待限流窗口后由调度恢复",
  check_source_availability: "检查平台可用性后重试",
  update_adapter: "暂停来源并更新适配器",
  review_run: "查看运行详情后再重试",
};

const failureLabels: Record<string, string> = {
  access_denied: "授权被拒绝",
  authorization_required: "需要授权",
  rate_limited: "平台限流",
  upstream_unavailable: "平台暂不可用",
  schema_drift: "来源结构变化",
  timeout: "请求超时",
};

const observedAt = new Intl.DateTimeFormat("zh-CN", {
  dateStyle: "short",
  timeStyle: "short",
  hour12: false,
});

function formatObservedAt(value?: string | null): string | null {
  if (!value) return null;
  return observedAt.format(new Date(value));
}

function RuntimeStatus({ runtime }: { runtime: Runtime }) {
  const successAt = formatObservedAt(runtime.last_success_at);
  const failureAt = formatObservedAt(runtime.last_failure_at);
  if (runtime.status === "unobserved") {
    return <small className="runtime-status">尚无运行记录</small>;
  }
  if (runtime.status === "healthy") {
    return (
      <small className="runtime-status" data-runtime="healthy">
        运行正常{successAt ? ` · 最近成功 ${successAt}` : ""}
      </small>
    );
  }
  const failure = runtime.last_failure_code
    ? (failureLabels[runtime.last_failure_code] ?? runtime.last_failure_code)
    : "未知失败";
  const recovery = runtime.recovery_action
    ? recoveryLabels[runtime.recovery_action]
    : null;
  return (
    <small className="runtime-status" data-runtime="degraded">
      <span>
        运行异常 · {failure}
        {failureAt ? ` · 最近失败 ${failureAt}` : ""}
      </span>
      {successAt ? <span>最近成功 {successAt}</span> : null}
      {recovery ? <span>恢复：{recovery}</span> : null}
    </small>
  );
}

export function canCollect(
  source: Source,
  operation: Capability["operation"] = "search_posts",
): boolean {
  return (
    source.operations.find((item) => item.operation === operation)
      ?.eligible_for_collection === true
  );
}

export function SourceCapabilities({ sources }: { sources: Source[] }) {
  return (
    <section aria-labelledby="source-capabilities-title">
      <div className="section-title">
        <div>
          <h2 id="source-capabilities-title">来源能力</h2>
          <p className="muted small">
            技术可读、用途权限与采集连接分别判断；当前能力卡不会触发采集。
          </p>
        </div>
      </div>
      <div className="cards source-cards">
        {sources.map((source) => (
          <article className="panel" key={source.id}>
            <div className="source-heading">
              <h3>{sourceLabels[source.id]}</h3>
              <span className="badge">
                {source.operations.filter(
                  (operation) => operation.eligible_for_collection,
                ).length || 0}
                项可采集
              </span>
            </div>
            <ul className="capability-list">
              {source.operations.map((capability) => (
                <li key={capability.operation}>
                  <span>{operationLabels[capability.operation]}</span>
                  <div className="capability-detail">
                    <strong data-support={capability.support}>
                      {supportLabels[capability.support]} ·{" "}
                      {rightsLabels[capability.rights]} ·{" "}
                      {pipelineLabels[capability.pipeline]}
                      {capability.eligible_for_collection ? " · 可采集" : ""}
                    </strong>
                    <RuntimeStatus runtime={capability.runtime} />
                  </div>
                </li>
              ))}
            </ul>
            <p className="muted small source-note">
              {source.operations[0]?.note}
            </p>
          </article>
        ))}
      </div>
    </section>
  );
}
