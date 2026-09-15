import { useEffect, useState } from "react";
import {
  createEventAnalysisRun,
  getAnalysisRun,
  labelAnalysisSample,
  listEventAnalysisRuns,
  recomputeAnalysisRun,
} from "../../api/analysis";
import { publishAnalysisKnowledge } from "../../api/knowledge";
import { message } from "../../request";

type Props = {
  eventId: string;
  eventRevision: number;
};

const statusLabels: Record<API.AnalysisRunView["status"], string> = {
  pending: "待人工标注",
  succeeded: "基线已完成",
  stale: "输入已变化",
};

const sentimentLabels: Record<API.AnalysisLabelInput["sentiment"], string> = {
  positive: "正向",
  negative: "负向",
  neutral: "中性",
  mixed: "混合",
  unknown: "未知",
};

const stanceLabels: Record<API.AnalysisLabelInput["stance"], string> = {
  support: "支持",
  oppose: "反对",
  neutral: "中立",
  mixed: "混合",
  unknown: "未知",
};

function SampleLabelForm({
  runId,
  sample,
  onSaved,
}: {
  runId: string;
  sample: API.AnalysisSampleView;
  onSaved: (value: API.AnalysisRunView) => void;
}) {
  const [topic, setTopic] = useState("");
  const [target, setTarget] = useState("");
  const [request, setRequest] = useState("");
  const [sentiment, setSentiment] =
    useState<API.AnalysisLabelInput["sentiment"]>("neutral");
  const [stance, setStance] =
    useState<API.AnalysisLabelInput["stance"]>("neutral");
  const [abstained, setAbstained] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const sampleContext = sample.contexts.find(
    (context) => context.role === "sample",
  );
  return (
    <form
      className="analysis-label-form"
      onSubmit={(event) => {
        event.preventDefault();
        if (!sampleContext) return;
        setBusy(true);
        setError("");
        void labelAnalysisSample(
          { identity: runId, sample_id: sample.id },
          abstained
            ? {
                topic: "",
                target: "",
                request: "",
                sentiment: "unknown",
                stance: "unknown",
                abstained: true,
                citation_content_version_ids: [],
              }
            : {
                topic,
                target,
                request,
                sentiment,
                stance,
                abstained: false,
                citation_content_version_ids: [
                  sampleContext.content_version_id,
                ],
              },
        )
          .then(onSaved)
          .catch((reason: unknown) => setError(message(reason)))
          .finally(() => setBusy(false));
      }}
    >
      <label className="analysis-abstain">
        <input
          type="checkbox"
          checked={abstained}
          onChange={(event) => setAbstained(event.target.checked)}
        />
        上下文不足，弃判
      </label>
      {!abstained && (
        <>
          <label>
            主题
            <input
              required
              maxLength={80}
              value={topic}
              onChange={(event) => setTopic(event.target.value)}
            />
          </label>
          <label>
            讨论对象
            <input
              required
              maxLength={160}
              value={target}
              onChange={(event) => setTarget(event.target.value)}
            />
          </label>
          <label>
            对象情绪
            <select
              value={sentiment}
              onChange={(event) =>
                setSentiment(
                  event.target.value as API.AnalysisLabelInput["sentiment"],
                )
              }
            >
              {Object.entries(sentimentLabels).map(([value, label]) => (
                <option key={value} value={value}>
                  {label}
                </option>
              ))}
            </select>
          </label>
          <label>
            对主张立场
            <select
              value={stance}
              onChange={(event) =>
                setStance(
                  event.target.value as API.AnalysisLabelInput["stance"],
                )
              }
            >
              {Object.entries(stanceLabels).map(([value, label]) => (
                <option key={value} value={value}>
                  {label}
                </option>
              ))}
            </select>
          </label>
          <label>
            诉求
            <input
              maxLength={500}
              value={request}
              onChange={(event) => setRequest(event.target.value)}
            />
          </label>
        </>
      )}
      <button disabled={busy || !sampleContext}>
        {busy ? "保存中…" : "保存人工标签"}
      </button>
      {error && <p className="error small">{error}</p>}
    </form>
  );
}

export function EventAnalysis({ eventId, eventRevision }: Props) {
  const [run, setRun] = useState<API.AnalysisRunView | null>(null);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [savedAnalysisId, setSavedAnalysisId] = useState<string | null>(null);

  useEffect(() => {
    const abort = new AbortController();
    setLoading(true);
    setError("");
    listEventAnalysisRuns(
      { identity: eventId, limit: 1 },
      { signal: abort.signal },
    )
      .then((page) => {
        const latest = page.items[0];
        if (!latest) return null;
        return getAnalysisRun(
          { identity: latest.id },
          { signal: abort.signal },
        );
      })
      .then((value) => {
        if (value && !abort.signal.aborted) setRun(value);
      })
      .catch((reason: unknown) => {
        if (!abort.signal.aborted) setError(message(reason));
      })
      .finally(() => {
        if (!abort.signal.aborted) setLoading(false);
      });
    return () => abort.abort();
  }, [eventId, eventRevision]);

  function freeze() {
    const cutoff = new Date();
    const since = new Date(cutoff.getTime() - 7 * 24 * 60 * 60 * 1000);
    setBusy(true);
    setError("");
    void createEventAnalysisRun(
      { identity: eventId },
      {
        expected_event_revision: eventRevision,
        since: since.toISOString(),
        until: cutoff.toISOString(),
        cutoff: cutoff.toISOString(),
        max_items: 24,
      },
    )
      .then(setRun)
      .catch((reason: unknown) => setError(message(reason)))
      .finally(() => setBusy(false));
  }

  function recompute() {
    if (!run) return;
    setBusy(true);
    setError("");
    void recomputeAnalysisRun({ identity: run.id })
      .then(setRun)
      .catch((reason: unknown) => setError(message(reason)))
      .finally(() => setBusy(false));
  }

  function publishKnowledge() {
    if (!run) return;
    setBusy(true);
    setError("");
    void publishAnalysisKnowledge({ identity: run.id })
      .then(() => setSavedAnalysisId(run.id))
      .catch((reason: unknown) => setError(message(reason)))
      .finally(() => setBusy(false));
  }

  return (
    <section className="event-analysis" aria-label="评论观点分析">
      <div className="analysis-heading">
        <div>
          <h4>评论观点样本</h4>
          <p className="muted small">
            固定最近 7 天评论版本；当前为人工基线，不代表全网观点。
          </p>
        </div>
        <button className="secondary" disabled={busy} onClick={freeze}>
          {busy ? "处理中…" : "冻结新样本"}
        </button>
      </div>
      {loading && <p role="status">正在读取分析样本…</p>}
      {error && <p className="error">{error}</p>}
      {run && (
        <div className="analysis-body">
          <div className="analysis-summary">
            <span className={`badge analysis-${run.status}`}>
              {statusLabels[run.status]}
            </span>
            <span>
              固定分母 {run.sample_count} · 已标注 {run.labeled_count} · 弃判{" "}
              {run.abstained_count}
            </span>
            <span>
              {Object.entries(run.composition.platforms)
                .map(([source, count]) => `${source} ${count}`)
                .join(" · ")}
            </span>
            <span>
              {run.composition.roots} 个根帖 · provider_default{" "}
              {run.composition.ordering_origins.provider_default ?? 0}
            </span>
          </div>
          <div className="actions">
            <button className="secondary" disabled={busy} onClick={recompute}>
              按冻结清单重算
            </button>
            {run.status === "succeeded" && (
              <button
                className="secondary"
                disabled={busy || savedAnalysisId === run.id}
                onClick={publishKnowledge}
              >
                {savedAnalysisId === run.id ? "已保存到知识库" : "保存到知识库"}
              </button>
            )}
          </div>
          {run.viewpoints.length > 0 && (
            <div className="analysis-viewpoints">
              <h5>已支持的观点</h5>
              {run.viewpoints.map((viewpoint) => (
                <article
                  className="analysis-viewpoint"
                  key={`${viewpoint.topic}-${viewpoint.target}-${viewpoint.stance}`}
                >
                  <strong>
                    {viewpoint.topic} · {viewpoint.target}
                  </strong>
                  <span>
                    {sentimentLabels[viewpoint.sentiment]} ·{" "}
                    {stanceLabels[viewpoint.stance]} · 本次样本{" "}
                    {viewpoint.sample_count} 条
                  </span>
                  {viewpoint.request && <span>诉求：{viewpoint.request}</span>}
                  {viewpoint.citations.map((citation) => (
                    <blockquote key={citation.content_version_id}>
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
                    </blockquote>
                  ))}
                </article>
              ))}
            </div>
          )}
          <div className="analysis-samples">
            {run.samples.map((sample) => {
              const content = sample.contexts.find(
                (context) => context.role === "sample",
              );
              return (
                <article className="analysis-sample" key={sample.id}>
                  <div className="sample-meta">
                    样本 {sample.position} · {sample.source} · {sample.kind} ·{" "}
                    {new Date(sample.published_at).toLocaleString()}
                  </div>
                  <p>{content?.text ?? "引用已失效"}</p>
                  <details>
                    <summary>查看冻结上下文与版本</summary>
                    {sample.contexts.map((context) => (
                      <div key={context.content_version_id}>
                        <strong>{context.role}</strong> ·{" "}
                        {context.content_version_id}
                        <p>{context.text ?? "该引用当前不可用"}</p>
                      </div>
                    ))}
                  </details>
                  {sample.label ? (
                    <p className="muted small">
                      {sample.label.abstained
                        ? "人工弃判"
                        : `${sample.label.topic} · ${sample.label.target} · ${sentimentLabels[sample.label.sentiment]} · ${stanceLabels[sample.label.stance]}`}
                    </p>
                  ) : (
                    <SampleLabelForm
                      runId={run.id}
                      sample={sample}
                      onSaved={setRun}
                    />
                  )}
                </article>
              );
            })}
          </div>
        </div>
      )}
      {!loading && !run && !error && (
        <p className="muted small">尚未冻结评论样本。</p>
      )}
    </section>
  );
}
