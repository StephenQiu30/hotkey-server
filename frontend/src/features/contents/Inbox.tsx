import { useState } from "react";

import { sourceLabels } from "../monitors/MonitorEditor";
import { ContentDiscussion } from "./ContentDiscussion";

type Content = API.InboxItem;
type ReviewState = API.MonitorMatchReviewInput["review_state"];

export type InboxFilters = {
  monitorId: string;
  source: "" | API.SourceView["id"];
  reviewState: "" | ReviewState;
  discoveredPeriod: "" | "24h" | "7d";
  discoveredSince: string;
};

type InboxProps = {
  items: Content[];
  nextCursor: string | null;
  busy: boolean;
  events: API.EventView[];
  monitors: API.MonitorView[];
  sources: API.SourceView[];
  filters: InboxFilters;
  onFiltersChange: (filters: InboxFilters) => void;
  onReview: (matchId: string, reviewState: ReviewState) => Promise<void>;
  onAssign: (eventId: string, contentId: string) => Promise<void>;
  onWithdraw: (contentId: string) => Promise<void>;
  onMore: () => void;
};

const kindLabels: Record<Content["kind"], string> = {
  post: "内容",
  comment: "评论",
  reply: "回复",
};

const relevanceLabels: Record<
  API.MonitorMatchView["relevance_status"],
  string
> = {
  pending: "待分析",
  accepted: "相关",
  rejected: "低相关",
  needs_review: "需复核",
};

const reviewLabels: Record<ReviewState, string> = {
  new: "待处理",
  ignored: "已忽略",
  following: "跟进中",
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
  monitors,
  sources,
  filters,
  onFiltersChange,
  onReview,
  onAssign,
  onWithdraw,
  onMore,
}: InboxProps) {
  const [selected, setSelected] = useState<Record<string, string>>({});
  const hasFilters = Boolean(
    filters.monitorId ||
    filters.source ||
    filters.reviewState ||
    filters.discoveredSince,
  );
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
      <div className="panel compact-form inbox-filters" aria-label="收件箱筛选">
        <label>
          监控主题
          <select
            value={filters.monitorId}
            onChange={(event) =>
              onFiltersChange({ ...filters, monitorId: event.target.value })
            }
          >
            <option value="">全部主题</option>
            {monitors.map((monitor) => (
              <option key={monitor.id} value={monitor.id}>
                {monitor.title}
              </option>
            ))}
          </select>
        </label>
        <label>
          来源
          <select
            value={filters.source}
            onChange={(event) =>
              onFiltersChange({
                ...filters,
                source: event.target.value as InboxFilters["source"],
              })
            }
          >
            <option value="">全部来源</option>
            {sources.map((source) => (
              <option key={source.id} value={source.id}>
                {sourceLabels[source.id] ?? source.id}
              </option>
            ))}
          </select>
        </label>
        <label>
          审核状态
          <select
            value={filters.reviewState}
            onChange={(event) =>
              onFiltersChange({
                ...filters,
                reviewState: event.target.value as InboxFilters["reviewState"],
              })
            }
          >
            <option value="">全部有效</option>
            <option value="new">待处理</option>
            <option value="following">跟进中</option>
            <option value="ignored">已忽略</option>
          </select>
        </label>
        <label>
          发现时间
          <select
            value={filters.discoveredPeriod}
            onChange={(event) => {
              const period = event.target.value;
              const days = period === "24h" ? 1 : period === "7d" ? 7 : 0;
              onFiltersChange({
                ...filters,
                discoveredPeriod: period as InboxFilters["discoveredPeriod"],
                discoveredSince: days
                  ? new Date(Date.now() - days * 86_400_000).toISOString()
                  : "",
              });
            }}
          >
            <option value="">全部时间</option>
            <option value="24h">最近24小时</option>
            <option value="7d">最近7天</option>
          </select>
        </label>
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
              <div className="match-list" aria-label="命中的监控">
                {item.matches.map((match) => (
                  <div className="match-row" key={match.id}>
                    <div>
                      <strong>
                        {match.monitor_title} · v{match.monitor_version}
                      </strong>
                      <span className="muted small">
                        {relevanceLabels[match.relevance_status]} ·{" "}
                        {reviewLabels[match.review_state]} · 命中{" "}
                        {match.match_reason.join("、")}
                      </span>
                    </div>
                    <div className="actions">
                      {match.review_state === "new" && (
                        <button
                          className="secondary"
                          disabled={busy}
                          aria-label={`跟进 ${match.monitor_title}`}
                          onClick={() => void onReview(match.id, "following")}
                        >
                          跟进
                        </button>
                      )}
                      {match.review_state === "following" && (
                        <button
                          className="secondary"
                          disabled={busy}
                          aria-label={`取消跟进 ${match.monitor_title}`}
                          onClick={() => void onReview(match.id, "new")}
                        >
                          取消跟进
                        </button>
                      )}
                      {match.review_state !== "ignored" ? (
                        <button
                          className="secondary"
                          disabled={busy}
                          aria-label={`忽略 ${match.monitor_title}`}
                          onClick={() => void onReview(match.id, "ignored")}
                        >
                          忽略
                        </button>
                      ) : (
                        <button
                          className="secondary"
                          disabled={busy}
                          aria-label={`恢复 ${match.monitor_title}`}
                          onClick={() => void onReview(match.id, "new")}
                        >
                          恢复待处理
                        </button>
                      )}
                    </div>
                  </div>
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
              <ContentDiscussion
                contentId={item.id}
                externalId={item.external_id}
                kind={item.kind}
              />
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
              <button
                className="secondary"
                disabled={busy}
                aria-label={`撤销 ${item.external_id} 的分析与检索用途`}
                onClick={() => void onWithdraw(item.id)}
              >
                撤销分析与检索用途
              </button>
            </article>
          ))}
        </div>
      ) : (
        <div className="empty">
          <h3>{hasFilters ? "没有符合筛选条件的内容" : "还没有监控内容"}</h3>
          <p>
            {hasFilters
              ? "调整主题、来源、时间或审核状态后重试。"
              : "来源完成用途准入并接入采集任务后，命中的内容会出现在这里。"}
          </p>
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
