import { FormEvent, useEffect, useState } from "react";
import { indexKnowledgeEntry, searchKnowledge } from "../../api/knowledge";
import { message } from "../../request";

export function KnowledgeSearch() {
  const [query, setQuery] = useState("");
  const [submittedQuery, setSubmittedQuery] = useState("");
  const [mode, setMode] = useState<"exact_substring" | "semantic">(
    "exact_substring",
  );
  const [items, setItems] = useState<API.KnowledgeEntryView[]>([]);
  const [loading, setLoading] = useState(true);
  const [indexingId, setIndexingId] = useState("");
  const [error, setError] = useState("");

  async function load(
    nextQuery: string,
    nextMode: "exact_substring" | "semantic",
  ) {
    setLoading(true);
    setError("");
    try {
      const page = await searchKnowledge({
        query: nextQuery || undefined,
        mode: nextMode,
      });
      setItems(page.items);
      setSubmittedQuery(page.query);
    } catch (reason) {
      setError(message(reason));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load("", "exact_substring");
  }, []);

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    void load(query, mode);
  }

  async function indexEntry(identity: string) {
    setIndexingId(identity);
    setError("");
    try {
      const updated = await indexKnowledgeEntry({ identity });
      setItems((current) =>
        current.map((entry) => (entry.id === identity ? updated : entry)),
      );
    } catch (reason) {
      setError(message(reason));
    } finally {
      setIndexingId("");
    }
  }

  return (
    <section aria-label="知识库">
      <div className="section-title">
        <div>
          <h2>
            知识库 <span className="count">{items.length}</span>
          </h2>
          <p className="muted small">
            精确检索查找连续文字；语义检索使用自建模型理解中英文近义表达，结果仍保留原始证据引用。
          </p>
        </div>
      </div>
      <form className="knowledge-search-form" onSubmit={submit}>
        <label>
          搜索知识条目
          <input
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder={
              mode === "semantic"
                ? "描述你想找的事件，例如：release outage"
                : "输入连续文字，例如：修复稳定性"
            }
            required={mode === "semantic"}
          />
        </label>
        <label>
          检索方式
          <select
            value={mode}
            onChange={(event) =>
              setMode(event.target.value as "exact_substring" | "semantic")
            }
          >
            <option value="exact_substring">精确检索</option>
            <option value="semantic">语义检索</option>
          </select>
        </label>
        <button disabled={loading}>
          {loading ? "检索中…" : mode === "semantic" ? "语义检索" : "精确检索"}
        </button>
        <button
          type="button"
          className="secondary"
          disabled={loading}
          onClick={() => void load(query, mode)}
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
                  <span className="muted small">
                    语义索引：
                    {entry.semantic_index_state === "ready"
                      ? "已就绪"
                      : entry.semantic_index_state === "failed"
                        ? "建立失败"
                        : "待建立"}
                    {entry.similarity != null &&
                      ` · 相似度 ${(entry.similarity * 100).toFixed(1)}%`}
                  </span>
                </div>
                <time dateTime={entry.updated_at}>
                  {new Date(entry.updated_at).toLocaleString()}
                </time>
              </div>
              <p className="knowledge-body">{entry.body}</p>
              {entry.semantic_index_state !== "ready" && (
                <button
                  className="secondary knowledge-index-button"
                  disabled={indexingId === entry.id}
                  onClick={() => void indexEntry(entry.id)}
                >
                  {indexingId === entry.id
                    ? "建立中…"
                    : entry.semantic_index_state === "failed"
                      ? "重试语义索引"
                      : "建立语义索引"}
                </button>
              )}
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
