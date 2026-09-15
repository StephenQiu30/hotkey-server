type Notification = API.NotificationView;

const kindLabels: Record<Notification["kind"], string> = {
  event_member_added: "内容加入",
  event_member_removed: "内容移出",
  event_merged_in: "事件合并",
  event_merged_out: "事件归档",
  event_split_in: "拆分创建",
  event_split_out: "内容拆分",
};

function time(value: string): string {
  return new Intl.DateTimeFormat("zh-CN", {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(value));
}

export function NotificationInbox({
  items,
  unreadCount,
  nextCursor,
  events,
  busy,
  onRead,
  onMore,
}: {
  items: Notification[];
  unreadCount: number;
  nextCursor: string | null;
  events: API.EventView[];
  busy: boolean;
  onRead: (identity: string) => Promise<void>;
  onMore: () => void;
}) {
  return (
    <section aria-labelledby="notifications-heading">
      <div className="section-title">
        <div>
          <h2 id="notifications-heading">
            事件提醒 <span className="count">{unreadCount} 未读</span>
          </h2>
          <p className="muted small">
            记录事件成员、合并和拆分变化；已读状态会持久保存。
          </p>
        </div>
      </div>
      {items.length ? (
        <div className="inbox-list">
          {items.map((item) => {
            const event = events.find(
              (candidate) => candidate.id === item.event_id,
            );
            return (
              <article
                className={`panel inbox-item${item.read_at ? " notification-read" : ""}`}
                key={item.id}
              >
                <div className="inbox-meta">
                  <span className="badge">
                    {item.read_at ? "已读" : "未读"} · {kindLabels[item.kind]}
                  </span>
                  <span className="muted small">{time(item.created_at)}</span>
                </div>
                <h3>{event?.title ?? `事件 ${item.event_id.slice(0, 8)}`}</h3>
                <p className="muted">{item.message}</p>
                {!item.read_at && (
                  <button
                    className="secondary"
                    disabled={busy}
                    onClick={() => void onRead(item.id)}
                  >
                    标为已读
                  </button>
                )}
              </article>
            );
          })}
        </div>
      ) : (
        <div className="empty">
          <h3>还没有事件提醒</h3>
          <p>事件成员、合并或拆分发生变化后，提醒会显示在这里。</p>
        </div>
      )}
      {nextCursor && (
        <button disabled={busy} onClick={onMore}>
          更多提醒
        </button>
      )}
    </section>
  );
}
