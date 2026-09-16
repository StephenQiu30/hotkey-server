import { useState } from "react";
import { ContentDiscussion } from "../contents/ContentDiscussion";
import { EventAnalysis } from "./EventAnalysis";
import { EventTrends } from "./EventTrends";

type Event = API.EventView;

type Props = {
  items: Event[];
  nextCursor: string | null;
  busy: boolean;
  onCreate: (title: string, summary: string) => Promise<void>;
  onRemove: (eventId: string, contentId: string) => Promise<void>;
  onMerge: (
    sourceId: string,
    targetId: string,
    sourceRevision: number,
    targetRevision: number,
  ) => Promise<void>;
  onSplit: (
    sourceId: string,
    title: string,
    contentIds: string[],
    revision: number,
  ) => Promise<void>;
  onNotificationsChanged: () => Promise<void>;
  onMore: () => void;
};

export function EventDossiers({
  items,
  nextCursor,
  busy,
  onCreate,
  onRemove,
  onMerge,
  onSplit,
  onNotificationsChanged,
  onMore,
}: Props) {
  const [title, setTitle] = useState("");
  const [summary, setSummary] = useState("");
  const [mergeTargets, setMergeTargets] = useState<Record<string, string>>({});
  const [splitTitles, setSplitTitles] = useState<Record<string, string>>({});
  const [splitMembers, setSplitMembers] = useState<Record<string, string[]>>(
    {},
  );
  return (
    <section aria-labelledby="events-heading">
      <div className="section-title">
        <div>
          <h2 id="events-heading">
            事件档案 <span className="count">{items.length}</span>
          </h2>
          <p className="muted small">
            人工归并已入库内容，每次变化都会保存修订。
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
        <div className="cards event-cards">
          {items.map((item) => {
            const targets = items.filter(
              (candidate) =>
                candidate.status === "active" && candidate.id !== item.id,
            );
            const selected = splitMembers[item.id] ?? [];
            return (
              <article className="panel" key={item.id}>
                <span className="badge">
                  {item.status === "archived" ? "已归档 · " : ""}修订 v
                  {item.current_revision} · {item.members.length} 条内容
                </span>
                <h3>{item.title}</h3>
                {item.summary && <p className="muted">{item.summary}</p>}
                {item.members.map((member) => (
                  <div key={member.content_id}>
                    <div className="event-member">
                      <span>
                        {member.source} · {member.kind} · {member.external_id}
                      </span>
                      <button
                        className="secondary"
                        disabled={busy}
                        onClick={() =>
                          void onRemove(item.id, member.content_id)
                        }
                      >
                        移出
                      </button>
                    </div>
                    <ContentDiscussion
                      contentId={member.content_id}
                      externalId={member.external_id}
                      kind={member.kind}
                    />
                  </div>
                ))}
                <EventTrends
                  eventId={item.id}
                  active={item.status === "active"}
                  onNotificationsChanged={onNotificationsChanged}
                />
                <EventAnalysis
                  eventId={item.id}
                  eventRevision={item.current_revision}
                />
                {item.status === "active" && targets.length > 0 && (
                  <div className="actions">
                    <select
                      aria-label={`选择 ${item.title} 的合并目标`}
                      value={mergeTargets[item.id] ?? ""}
                      onChange={(event) =>
                        setMergeTargets((old) => ({
                          ...old,
                          [item.id]: event.target.value,
                        }))
                      }
                    >
                      <option value="">选择合并目标</option>
                      {targets.map((candidate) => (
                        <option key={candidate.id} value={candidate.id}>
                          {candidate.title}
                        </option>
                      ))}
                    </select>
                    <button
                      className="secondary"
                      disabled={busy || !mergeTargets[item.id]}
                      onClick={() => {
                        const target = targets.find(
                          (candidate) => candidate.id === mergeTargets[item.id],
                        );
                        if (target)
                          void onMerge(
                            item.id,
                            target.id,
                            item.current_revision,
                            target.current_revision,
                          );
                      }}
                    >
                      合并到所选事件
                    </button>
                  </div>
                )}
                {item.status === "active" && item.members.length > 1 && (
                  <fieldset className="split-box">
                    <legend>拆分成员</legend>
                    {item.members.map((member) => (
                      <label key={member.content_id}>
                        <input
                          type="checkbox"
                          checked={selected.includes(member.content_id)}
                          onChange={(event) =>
                            setSplitMembers((old) => ({
                              ...old,
                              [item.id]: event.target.checked
                                ? [...selected, member.content_id]
                                : selected.filter(
                                    (id) => id !== member.content_id,
                                  ),
                            }))
                          }
                        />
                        {member.external_id}
                      </label>
                    ))}
                    <label>
                      新事件名称
                      <input
                        aria-label={`${item.title} 的新事件名称`}
                        maxLength={200}
                        value={splitTitles[item.id] ?? ""}
                        onChange={(event) =>
                          setSplitTitles((old) => ({
                            ...old,
                            [item.id]: event.target.value,
                          }))
                        }
                      />
                    </label>
                    <button
                      className="secondary"
                      disabled={
                        busy ||
                        !splitTitles[item.id]?.trim() ||
                        selected.length === 0 ||
                        selected.length === item.members.length
                      }
                      onClick={() =>
                        void onSplit(
                          item.id,
                          splitTitles[item.id],
                          selected,
                          item.current_revision,
                        )
                      }
                    >
                      拆分为新事件
                    </button>
                  </fieldset>
                )}
              </article>
            );
          })}
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
