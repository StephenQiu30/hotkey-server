import { FormEvent, useEffect, useState } from "react";
import { searchKnowledge } from "../../api/knowledge";
import { message } from "../../request";

export function KnowledgeSearch() {
  const [query, setQuery] = useState("");
  const [submittedQuery, setSubmittedQuery] = useState("");
  const [items, setItems] = useState<API.KnowledgeEntryView[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  async function load(nextQuery: string) {
    setLoading(true);
    setError("");
    try {
      const page = await searchKnowledge({ query: nextQuery || undefined });
      setItems(page.items);
      setSubmittedQuery(page.query);
    } catch (reason) {
      setError(message(reason));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load("");
  }, []);

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    void load(query);
  }

  return (
    <section aria-label="知识库">
      <div className="section-title">
        <div>
          <h2>
            知识库 <span className="count">{items.length}</span>
          </h2>
          <p className="muted small">
            搜索已保存分析的标题和正文；当前仅支持规范化后的字面精确检索。
          </p>
        </div>
      </div>
      <form className="knowledge-search-form" onSubmit={submit}>
        <label>
          搜索知识条目
          <input
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder="输入连续文字，例如：修复稳定性"
          />
        </label>
        <button disabled={loading}>{loading ? "检索中…" : "精确检索"}</button>
        <button
          type="button"
          className="secondary"
          disabled={loading}
          onClick={() => void load(query)}
        >
          刷新知识库
        </button>
      </form>
      {error && <p role="alert">{error}</p>}
      {!loading && !error && items.length === 0 && (
        <div className="empty">
          <h3>{submittedQuery ? "没有匹配条目" : "还没有知识条目"}</h3>
          <p>
            {submittedQuery
              ? `没有找到包含“${submittedQuery}”的已保存分析。`
              : "完成评论分析后，可从事件档案保存为可追溯知识条目。"}
          </p>
        </div>
      )}
      {items.length > 0 && (
        <div className="knowledge-list">
          {items.map((entry) => (
            <article className="panel knowledge-entry" key={entry.id}>
              <div className="knowledge-entry-heading">
                <div>
                  <span
                    className={`badge ${entry.stale ? "analysis-stale" : ""}`}
                  >
                    {entry.stale ? "引用已变化" : `快照 v${entry.version}`}
                  </span>
                  <h3>{entry.title}</h3>
                </div>
                <time dateTime={entry.updated_at}>
                  {new Date(entry.updated_at).toLocaleString()}
                </time>
              </div>
              <p className="knowledge-body">{entry.body}</p>
              <div className="knowledge-citations">
                <strong>证据引用</strong>
                {entry.citations.map((citation) => (
                  <div key={citation.content_version_id}>
                    {citation.available ? citation.text : "该引用当前不可用"}
                    {citation.available && citation.canonical_url && (
                      <a
                        href={citation.canonical_url}
                        target="_blank"
                        rel="noreferrer"
                      >
                        查看原链接
                      </a>
                    )}
                  </div>
                ))}
              </div>
            </article>
          ))}
        </div>
      )}
    </section>
  );
}
