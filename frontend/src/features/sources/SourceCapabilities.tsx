type Source = API.SourceView;
type Capability = API.SourceOperationCapability;

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
                  <strong data-support={capability.support}>
                    {supportLabels[capability.support]} ·{" "}
                    {rightsLabels[capability.rights]} ·{" "}
                    {pipelineLabels[capability.pipeline]}
                    {capability.eligible_for_collection ? " · 可采集" : ""}
                  </strong>
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
