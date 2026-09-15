import { useState, type FormEvent } from "react";
import { createMonitor, updateMonitor } from "../../api/monitoring";
import { previewSourceQueries } from "../../api/sources";
import { message } from "../../request";

type Monitor = API.MonitorView;
type Source = API.SourceView;

export const sourceLabels: Record<Source["id"], string> = {
  x: "X",
  bilibili: "B站",
  weibo: "微博",
  xiaohongshu: "小红书",
  douyin: "抖音",
  bluesky: "Bluesky",
};

const modeLabels: Record<API.QueryRuleExecution["mode"], string> = {
  native: "平台查询",
  local_filter: "入库前过滤",
  unsupported: "当前不支持",
};

function lines(value: FormDataEntryValue | null) {
  return String(value ?? "")
    .split("\n")
    .map((item) => item.trim())
    .filter(Boolean);
}

function input(form: HTMLFormElement, sources: Source[]): API.MonitorInput {
  const values = new FormData(form);
  const selected = sources
    .filter((source) => values.getAll("source_ids").includes(source.id))
    .map((source) => source.id);
  return {
    title: String(values.get("title")),
    query_spec: {
      include_any: lines(values.get("include_any")),
      include_all: lines(values.get("include_all")),
      exclude: lines(values.get("exclude")),
      aliases: lines(values.get("aliases")),
    },
    source_ids: selected,
    schedule: {
      interval_minutes: Number(values.get("interval_minutes")),
      retention_days: Number(values.get("retention_days")),
    },
    budget: {
      daily_requests: Number(values.get("daily_requests")),
      content_purchase_cost: 0,
    },
  };
}

export function MonitorEditor({
  monitor,
  sources,
  onSaved,
  onClose,
}: {
  monitor?: Monitor;
  sources: Source[];
  onSaved: () => void;
  onClose: () => void;
}) {
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [preview, setPreview] = useState<API.QueryPreview | null>(null);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const body = input(event.currentTarget, sources);
    if (!body.source_ids.length) {
      setError("请至少选择一个来源。");
      return;
    }
    setBusy(true);
    setError("");
    try {
      if (monitor)
        await updateMonitor(
          { identity: monitor.id },
          { ...body, expected_version: monitor.current_version },
        );
      else await createMonitor(body);
      onSaved();
    } catch (error) {
      setError(message(error));
    } finally {
      setBusy(false);
    }
  }

  async function showPreview(form: HTMLFormElement) {
    const body = input(form, sources);
    if (!body.query_spec.include_any.length || !body.source_ids.length) {
      setError("请先填写关键词并选择来源。");
      return;
    }
    const until = new Date();
    const since = new Date(until.getTime() - 24 * 60 * 60 * 1000);
    setBusy(true);
    setError("");
    try {
      setPreview(
        await previewSourceQueries({
          query_spec: body.query_spec,
          source_ids: body.source_ids,
          since: since.toISOString(),
          until: until.toISOString(),
        }),
      );
    } catch (error) {
      setError(message(error));
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="panel editor" aria-labelledby="editor-title">
      <h2 id="editor-title">{monitor ? "编辑监控草稿" : "新建监控草稿"}</h2>
      <form onSubmit={submit}>
        <label>
          监控名称
          <input
            name="title"
            required
            maxLength={100}
            defaultValue={monitor?.title}
          />
        </label>
        <label>
          任一关键词（每行一个，最多 20 个）
          <textarea
            name="include_any"
            required
            rows={4}
            defaultValue={monitor?.query_spec.include_any.join("\n")}
          />
        </label>
        <div className="form-grid">
          <label>
            必须同时包含
            <textarea
              name="include_all"
              rows={3}
              defaultValue={monitor?.query_spec.include_all?.join("\n")}
            />
          </label>
          <label>
            排除词
            <textarea
              name="exclude"
              rows={3}
              defaultValue={monitor?.query_spec.exclude?.join("\n")}
            />
          </label>
          <label>
            别名
            <textarea
              name="aliases"
              rows={3}
              defaultValue={monitor?.query_spec.aliases?.join("\n")}
            />
          </label>
        </div>
        <fieldset>
          <legend>关注来源</legend>
          <div className="checks">
            {sources.map((source) => (
              <label key={source.id}>
                <input
                  type="checkbox"
                  name="source_ids"
                  value={source.id}
                  defaultChecked={monitor?.source_ids.includes(source.id)}
                />
                {sourceLabels[source.id]}
              </label>
            ))}
          </div>
        </fieldset>
        <div className="form-grid">
          <label>
            检查周期（分钟）
            <input
              name="interval_minutes"
              type="number"
              min={15}
              max={1440}
              required
              defaultValue={monitor?.schedule.interval_minutes ?? 60}
            />
          </label>
          <label>
            每日请求上限
            <input
              name="daily_requests"
              type="number"
              min={1}
              max={1000}
              required
              defaultValue={monitor?.budget.daily_requests ?? 96}
            />
          </label>
          <label>
            证据保留（天）
            <input
              name="retention_days"
              type="number"
              min={1}
              max={365}
              required
              defaultValue={monitor ? monitor.schedule.retention_days : 7}
            />
          </label>
        </div>
        <p className="notice">
          内容采购预算固定为 0；查询预览不会访问外部平台。
        </p>
        {preview && (
          <div className="query-preview" aria-live="polite">
            <h3>查询预览 · 预计 {preview.estimated_requests} 次请求</h3>
            {preview.sources.map((source) => (
              <div key={source.source}>
                <strong>{sourceLabels[source.source]}</strong>
                <p>{source.queries.join(" / ") || "当前无法编译查询"}</p>
                <small>
                  {source.rules
                    .map((rule) => modeLabels[rule.mode])
                    .filter(
                      (label, index, labels) => labels.indexOf(label) === index,
                    )
                    .join(" · ")}
                </small>
              </div>
            ))}
          </div>
        )}
        {error && <p role="alert">{error}</p>}
        <div className="actions">
          <button disabled={busy}>{busy ? "正在处理…" : "保存草稿"}</button>
          <button
            type="button"
            className="secondary"
            disabled={busy}
            onClick={(event) => {
              const form = event.currentTarget.form;
              if (form) void showPreview(form);
            }}
          >
            预览查询
          </button>
          <button
            type="button"
            className="secondary"
            disabled={busy}
            onClick={onClose}
          >
            取消
          </button>
        </div>
      </form>
    </section>
  );
}
