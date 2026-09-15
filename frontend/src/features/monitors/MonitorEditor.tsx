import { useState, type FormEvent } from "react";
import { createMonitor, updateMonitor } from "../../api/monitoring";
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
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const values = new FormData(event.currentTarget);
    const selected = sources
      .filter((s) => values.getAll("sources").includes(s.id))
      .map((s) => s.id);
    if (!selected.length) {
      setError("请至少选择一个来源。");
      return;
    }
    const body = {
      title: String(values.get("title")),
      keywords: String(values.get("keywords"))
        .split("\n")
        .filter((v) => v.trim()),
      sources: selected,
    };
    setBusy(true);
    setError("");
    try {
      if (monitor)
        await updateMonitor(
          { identity: monitor.id },
          { ...body, version: monitor.version },
        );
      else await createMonitor(body);
      onSaved();
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
          关键词（每行一个，最多 20 个）
          <textarea
            name="keywords"
            required
            rows={4}
            defaultValue={monitor?.keywords.join("\n")}
          />
        </label>
        <fieldset>
          <legend>关注来源</legend>
          <div className="checks">
            {sources.map((s) => (
              <label key={s.id}>
                <input
                  type="checkbox"
                  name="sources"
                  value={s.id}
                  defaultChecked={monitor?.sources.includes(s.id)}
                />
                {sourceLabels[s.id]}
              </label>
            ))}
          </div>
        </fieldset>
        <p className="notice">当前保存为草稿；来源连接完成后才能开启采集。</p>
        {error && <p role="alert">{error}</p>}
        <div className="actions">
          <button disabled={busy}>{busy ? "正在保存…" : "保存草稿"}</button>
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
