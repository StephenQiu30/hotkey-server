import { useState } from "react";

import { getContentDetail } from "../../api/contents";
import { message } from "../../request";

type ContentKind = API.ContentDetailItem["kind"];

type ContentDiscussionProps = {
  contentId: string;
  externalId: string;
  kind: ContentKind;
};

const kindLabels: Record<ContentKind, string> = {
  post: "根帖",
  comment: "评论",
  reply: "回复",
};

export function ContentDiscussion({
  contentId,
  externalId,
  kind,
}: ContentDiscussionProps) {
  const [opened, setOpened] = useState(false);
  const [detail, setDetail] = useState<API.ContentDetailView | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  const load = async (cursor?: string) => {
    setLoading(true);
    setError("");
    try {
      const next = await getContentDetail({
        identity: contentId,
        limit: 20,
        cursor,
      });
      setDetail((current) =>
        cursor && current
          ? {
              ...next,
              discussion: [...current.discussion, ...next.discussion],
            }
          : next,
      );
    } catch (cause) {
      setError(message(cause));
    } finally {
      setLoading(false);
    }
  };

  const toggle = () => {
    const nextOpened = !opened;
    setOpened(nextOpened);
    if (nextOpened && detail === null && !loading) void load();
  };

  return (
    <div>
      <button
        className="secondary"
        type="button"
        aria-expanded={opened}
        aria-controls={`discussion-${contentId}`}
        onClick={toggle}
      >
        {opened ? "收起评论上下文" : "查看评论上下文"}
      </button>
      {opened && (
        <section
          id={`discussion-${contentId}`}
          className="panel"
          aria-label={`${externalId} 的评论上下文`}
        >
          <h4>评论上下文</h4>
          {loading && detail === null && <p className="muted">正在读取…</p>}
          {error && <p role="alert">{error}</p>}
          {detail && (
            <>
              {detail.root && detail.root.id !== detail.selected.id && (
                <div className="match-row">
                  <strong>根帖</strong>
                  <span>{detail.root.text}</span>
                </div>
              )}
              {detail.parent && detail.parent.id !== detail.selected.id && (
                <div className="match-row">
                  <strong>父级评论</strong>
                  <span>{detail.parent.text}</span>
                </div>
              )}
              {!detail.root && kind !== "post" && (
                <p className="muted small">根帖关系尚未唯一解析。</p>
              )}
              {detail.discussion.length ? (
                <ol aria-label="已保存的评论列表">
                  {detail.discussion.map((item) => (
                    <li key={item.id}>
                      <span className="badge">{kindLabels[item.kind]}</span>{" "}
                      {item.text}
                      {item.parent_external_id && (
                        <span className="muted small">
                          {" "}
                          · 回复 {item.parent_external_id}
                        </span>
                      )}
                    </li>
                  ))}
                </ol>
              ) : (
                <p className="muted small">当前没有已保存的评论。</p>
              )}
              {detail.next_cursor && (
                <button
                  className="secondary"
                  type="button"
                  disabled={loading}
                  onClick={() => void load(detail.next_cursor ?? undefined)}
                >
                  {loading ? "正在读取…" : "更多评论"}
                </button>
              )}
            </>
          )}
        </section>
      )}
    </div>
  );
}
