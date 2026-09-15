import { FormEvent, useEffect, useState } from "react";
import {
  indexKnowledgeEntry,
  queryKnowledge,
  searchKnowledge,
} from "../../api/knowledge";
import { message } from "../../request";

type KnowledgeSearchProps = {
  events: API.EventView[];
};

function localDateTime(value: Date): string {
  const local = new Date(value.getTime() - value.getTimezoneOffset() * 60_000);
  return local.toISOString().slice(0, 16);
}

export function KnowledgeSearch({ events }: KnowledgeSearchProps) {
  const [query, setQuery] = useState("");
  const [submittedQuery, setSubmittedQuery] = useState("");
  const [mode, setMode] = useState<"exact_substring" | "semantic">(
    "exact_substring",
  );
  const [items, setItems] = useState<API.KnowledgeEntryView[]>([]);
  const [loading, setLoading] = useState(true);
  const [indexingId, setIndexingId] = useState("");
  const [error, setError] = useState("");
  const [questionKind, setQuestionKind] = useState<
    "comment_count" | "evidence"
  >("comment_count");
  const [question, setQuestion] = useState("这段时间采到了多少条评论？");
  const [questionEventId, setQuestionEventId] = useState("");
  const [questionMode, setQuestionMode] = useState<
    "exact_substring" | "semantic"
  >("exact_substring");
  const [since, setSince] = useState(() =>
    localDateTime(new Date(Date.now() - 7 * 24 * 60 * 60 * 1000)),
  );
  const [until, setUntil] = useState(() => localDateTime(new Date()));
  const [answer, setAnswer] = useState<API.KnowledgeAnswer | null>(null);
  const [questionLoading, setQuestionLoading] = useState(false);
  const [questionError, setQuestionError] = useState("");

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

  useEffect(() => {
    if (!events.some((item) => item.id === questionEventId))
      setQuestionEventId(events[0]?.id ?? "");
  }, [events, questionEventId]);

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

  async function submitQuestion(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setQuestionLoading(true);
    setQuestionError("");
    setAnswer(null);
    try {
      const result =
        questionKind === "comment_count"
          ? await queryKnowledge({
              kind: "comment_count",
              question,
              event_id: questionEventId,
              since: new Date(since).toISOString(),
              until: new Date(until).toISOString(),
            })
          : await queryKnowledge({
              kind: "evidence",
              question,
              event_id: questionEventId || null,
              mode: questionMode,
              limit: 3,
            });
      setAnswer(result);
    } catch (reason) {
      setQuestionError(message(reason));
    } finally {
      setQuestionLoading(false);
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
      <form
        className="knowledge-question-form panel"
        aria-label="受控问答"
        onSubmit={(event) => void submitQuestion(event)}
      >
        <div>
          <h3>受控问答</h3>
          <p className="muted small">
            统计使用固定事件和时间范围；证据回答只引用当前有效知识，找不到依据时明确返回未知。
          </p>
        </div>
        <label>
          问题类型
          <select
            value={questionKind}
            onChange={(event) => {
              const value = event.target.value as "comment_count" | "evidence";
              setQuestionKind(value);
              setQuestion(
                value === "comment_count"
                  ? "这段时间采到了多少条评论？"
                  : "修复稳定性",
              );
              setAnswer(null);
            }}
          >
            <option value="comment_count">评论数量</option>
            <option value="evidence">证据问题</option>
          </select>
        </label>
        <label>
          事件范围
          <select
            value={questionEventId}
            required={questionKind === "comment_count"}
            onChange={(event) => setQuestionEventId(event.target.value)}
          >
            {questionKind === "evidence" && <option value="">全部事件</option>}
            {!events.length && <option value="">还没有事件</option>}
            {events.map((item) => (
              <option key={item.id} value={item.id}>
                {item.title}
              </option>
            ))}
          </select>
        </label>
        {questionKind === "comment_count" && (
          <div className="knowledge-question-window">
            <label>
              开始时间
              <input
                type="datetime-local"
                value={since}
                required
                onChange={(event) => setSince(event.target.value)}
              />
            </label>
            <label>
              结束时间
              <input
                type="datetime-local"
                value={until}
                required
                onChange={(event) => setUntil(event.target.value)}
              />
            </label>
          </div>
        )}
        {questionKind === "evidence" && (
          <label>
            检索方式
            <select
              value={questionMode}
              onChange={(event) =>
                setQuestionMode(
                  event.target.value as "exact_substring" | "semantic",
                )
              }
            >
              <option value="exact_substring">精确证据</option>
              <option value="semantic">语义证据</option>
            </select>
          </label>
        )}
        <label>
          问题
          <input
            value={question}
            minLength={2}
            maxLength={300}
            required
            onChange={(event) => setQuestion(event.target.value)}
          />
        </label>
        <button
          disabled={
            questionLoading ||
            (questionKind === "comment_count" && !questionEventId)
          }
        >
          {questionLoading ? "计算中…" : "提交问题"}
        </button>
        {questionError && <p role="alert">{questionError}</p>}
        {answer && (
          <output className="knowledge-answer" aria-live="polite">
            <span className="badge">
              {answer.status === "answered" ? "已有依据" : "依据不足"}
            </span>
            <p>{answer.answer}</p>
            {answer.statistics && (
              <div className="knowledge-statistics">
                <strong>统计结果：{answer.statistics.total} 条</strong>
                <span>
                  平台：
                  {Object.entries(answer.statistics.by_source)
                    .map(([source, count]) => `${source} ${String(count)}`)
                    .join(" / ") || "无"}
                </span>
                <span>
                  类型：
                  {Object.entries(answer.statistics.by_kind)
                    .map(([kind, count]) => `${kind} ${String(count)}`)
                    .join(" / ") || "无"}
                </span>
                <span>
                  范围：{new Date(answer.statistics.since).toLocaleString()} 至{" "}
                  {new Date(answer.statistics.until).toLocaleString()} ·
                  事件修订 v{answer.statistics.event_revision}
                </span>
              </div>
            )}
            {!!answer.citations?.length && (
              <div className="knowledge-citations">
                <strong>回答依据</strong>
                {answer.citations.map((citation) => (
                  <div key={citation.content_version_id}>
                    {citation.text}
                    {citation.canonical_url && (
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
            )}
          </output>
        )}
      </form>
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
