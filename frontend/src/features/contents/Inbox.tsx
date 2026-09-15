import { useState } from "react";

import { sourceLabels } from "../monitors/MonitorEditor";

type Content = API.InboxItem;

type InboxProps = {
  items: Content[];
  nextCursor: string | null;
  busy: boolean;
  events: API.EventView[];
  onAssign: (eventId: string, contentId: string) => Promise<void>;
  onMore: () => void;
};

const kindLabels: Record<Content["kind"], string> = {
  post: "内容",
  comment: "评论",
  reply: "回复",
};

function time(value: string): string {
  return new Intl.DateTimeFormat("zh-CN", {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(value));
}

export function Inbox({
  items,
  nextCursor,
  busy,
  events,
  onAssign,
  onMore,
}: InboxProps) {
  const [selected, setSelected] = useState<Record<string, string>>({});
  return (
    <section aria-labelledby="inbox-heading">
      <div className="section-title">
        <div>
          <h2 id="inbox-heading">
            监控收件箱 <span className="count">{items.length}</span>
          </h2>
          <p className="muted small">只显示已命中监控配置并完成入库的内容。</p>
        </div>
      </div>
      {items.length ? (
        <div className="inbox-list">
          {items.map((item) => (
            <article className="panel inbox-item" key={item.id}>
              <div className="inbox-meta">
                <span className="badge">
                  {sourceLabels[item.source] ?? item.source} ·{" "}
                  {kindLabels[item.kind]}
                </span>
                <span className="muted small">
                  首次发现 {time(item.first_seen_at)}
                </span>
              </div>
              <p className="inbox-text">{item.text}</p>
              <div className="tags" aria-label="命中的监控">
                {item.monitor_titles.map((title) => (
                  <span key={title}>{title}</span>
                ))}
              </div>
              <p className="muted small">
                发布于 {time(item.published_at)} · 最近观察{" "}
                {time(item.last_seen_at)} · 评论数{" "}
                {item.reply_count ?? "未提供"} · 正文版本 v{item.version}
              </p>
              {item.canonical_url && (
                <a href={item.canonical_url} target="_blank" rel="noreferrer">
                  查看来源
                </a>
              )}
              {(() => {
                const assigned = events.find((event) =>
                  event.members.some((member) => member.content_id === item.id),
                );
                if (assigned)
                  return <span className="badge">已归入 {assigned.title}</span>;
                if (!events.length)
                  return (
                    <span className="muted small">
                      先创建事件，再归入内容。
                    </span>
                  );
                return (
                  <div className="actions">
                    <select
                      aria-label={`选择 ${item.external_id} 的事件`}
                      value={selected[item.id] ?? ""}
                      onChange={(event) =>
                        setSelected((old) => ({
                          ...old,
                          [item.id]: event.target.value,
                        }))
                      }
                    >
                      <option value="">选择事件</option>
                      {events.map((event) => (
                        <option key={event.id} value={event.id}>
                          {event.title}
                        </option>
                      ))}
                    </select>
                    <button
                      className="secondary"
                      disabled={busy || !selected[item.id]}
                      onClick={() => void onAssign(selected[item.id], item.id)}
                    >
                      加入事件
                    </button>
                  </div>
                );
              })()}
            </article>
          ))}
        </div>
      ) : (
        <div className="empty">
          <h3>还没有监控内容</h3>
          <p>来源完成用途准入并接入采集任务后，命中的内容会出现在这里。</p>
        </div>
      )}
      {nextCursor && (
        <button disabled={busy} onClick={onMore}>
          更多内容
        </button>
      )}
    </section>
  );
}
