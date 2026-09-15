import { useState } from "react";

type Event = API.EventView;

type EventDossiersProps = {
  items: Event[];
  nextCursor: string | null;
  busy: boolean;
  onCreate: (title: string, summary: string) => Promise<void>;
  onRemove: (eventId: string, contentId: string) => Promise<void>;
  onMore: () => void;
};

export function EventDossiers({
  items,
  nextCursor,
  busy,
  onCreate,
  onRemove,
  onMore,
}: EventDossiersProps) {
  const [title, setTitle] = useState("");
  const [summary, setSummary] = useState("");

  return (
    <section aria-labelledby="events-heading">
      <div className="section-title">
        <div>
          <h2 id="events-heading">
            事件档案 <span className="count">{items.length}</span>
          </h2>
          <p className="muted small">
            人工归并已入库内容，每次成员变化都会保存修订。
          </p>
        </div>
      </div>
      <form
        className="panel compact-form"
        onSubmit={(event) => {
          event.preventDefault();
          void onCreate(title, summary).then(() => {
            setTitle("");
            setSummary("");
          });
        }}
      >
        <label>
          事件名称
          <input
            required
            maxLength={200}
            value={title}
            onChange={(event) => setTitle(event.target.value)}
          />
        </label>
        <label>
          简介
          <input
            maxLength={2000}
            value={summary}
            onChange={(event) => setSummary(event.target.value)}
          />
        </label>
        <button disabled={busy || !title.trim()}>创建事件</button>
      </form>
      {items.length ? (
        <div className="cards">
          {items.map((item) => (
            <article className="panel" key={item.id}>
              <span className="badge">
                修订 v{item.current_revision} · {item.members.length} 条内容
              </span>
              <h3>{item.title}</h3>
              {item.summary && <p className="muted">{item.summary}</p>}
              {item.members.map((member) => (
                <div className="event-member" key={member.content_id}>
                  <span>
                    {member.source} · {member.kind} · {member.external_id}
                  </span>
                  <button
                    className="secondary"
                    disabled={busy}
                    onClick={() => void onRemove(item.id, member.content_id)}
                  >
                    移出
                  </button>
                </div>
              ))}
            </article>
          ))}
        </div>
      ) : (
        <div className="empty">
          <h3>还没有事件档案</h3>
          <p>创建事件后，可从监控收件箱把内容归入同一事件。</p>
        </div>
      )}
      {nextCursor && (
        <button disabled={busy} onClick={onMore}>
          更多事件
        </button>
      )}
    </section>
  );
}
