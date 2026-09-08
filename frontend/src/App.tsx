import { useCallback, useEffect, useRef, useState } from "react";
import {
  api,
  message,
  unwrap,
  type Job,
  type Monitor,
  type Source,
} from "./client";
import { Login } from "./Login";
import { MonitorEditor, sourceLabels } from "./MonitorEditor";
const statuses: Record<Job["status"], string> = {
  queued: "排队中",
  running: "执行中",
  succeeded: "已完成",
  failed: "失败",
  cancelled: "已取消",
};
export function App() {
  const [session, setSession] = useState<"loading" | "in" | "out">("loading");
  const [error, setError] = useState("");
  const [monitors, setMonitors] = useState<Monitor[]>([]);
  const [jobs, setJobs] = useState<Job[]>([]);
  const [sources, setSources] = useState<Source[]>([]);
  const [editor, setEditor] = useState<Monitor | "new" | null>(null);
  const [busy, setBusy] = useState(false);
  const [loading, setLoading] = useState(true);
  const [monitorCursor, setMonitorCursor] = useState<string | null>(null);
  const [jobCursor, setJobCursor] = useState<string | null>(null);
  const generation = useRef(0);
  const diagnosticKey = useRef<string | null>(null);
  const refresh = useCallback(async () => {
    const current = ++generation.current;
    setLoading(true);
    setError("");
    try {
      const [m, j, s] = await Promise.all([
        api.GET("/api/v1/monitors"),
        api.GET("/api/v1/jobs"),
        api.GET("/api/v1/sources"),
      ]);
      const ms = unwrap(m),
        js = unwrap(j),
        ss = unwrap(s);
      if (current !== generation.current) return;
      setMonitors(ms.items);
      setMonitorCursor(ms.next_cursor);
      setJobs(js.items);
      setJobCursor(js.next_cursor);
      setSources(ss);
    } catch (e) {
      if (current !== generation.current) return;
      if (
        e &&
        typeof e === "object" &&
        "code" in e &&
        e.code === "authentication_required"
      )
        setSession("out");
      else setError(message(e));
    } finally {
      if (current === generation.current) setLoading(false);
    }
  }, []);
  useEffect(() => {
    const abort = new AbortController();
    api
      .GET("/api/v1/session", { signal: abort.signal })
      .then(({ data, error, response }) => {
        if (abort.signal.aborted) return;
        if (data) setSession("in");
        else if (response.status === 401) setSession("out");
        else {
          setError(message(error));
          setSession("out");
        }
      })
      .catch(() => {
        if (!abort.signal.aborted) {
          setError("服务连接失败。");
          setSession("out");
        }
      });
    return () => abort.abort();
  }, []);
  useEffect(() => {
    if (session === "in") void refresh();
    return () => {
      generation.current++;
    };
  }, [session, refresh]);
  async function action(work: () => Promise<void>) {
    setBusy(true);
    setError("");
    try {
      await work();
    } catch (e) {
      setError(message(e));
    } finally {
      setBusy(false);
    }
  }
  async function more(kind: "monitors" | "jobs") {
    await action(async () => {
      if (kind === "monitors" && monitorCursor) {
        const page = unwrap(
          await api.GET("/api/v1/monitors", {
            params: { query: { cursor: monitorCursor } },
          }),
        );
        setMonitors((old) => [...old, ...page.items]);
        setMonitorCursor(page.next_cursor);
      } else if (kind === "jobs" && jobCursor) {
        const page = unwrap(
          await api.GET("/api/v1/jobs", {
            params: { query: { cursor: jobCursor } },
          }),
        );
        setJobs((old) => [...old, ...page.items]);
        setJobCursor(page.next_cursor);
      }
    });
  }
  if (session === "loading")
    return (
      <main>
        <p role="status">正在连接工作台…</p>
      </main>
    );
  if (session === "out")
    return (
      <>
        <Login
          onLogin={() => {
            setError("");
            setSession("in");
          }}
        />
        {error && (
          <p role="alert" className="login">
            {error}
          </p>
        )}
      </>
    );
  return (
    <>
      <header>
        <a href="/" className="brand">
          HotKey<span>个人观察室</span>
        </a>
        <button
          className="secondary"
          disabled={busy}
          onClick={() =>
            void action(async () => {
              const result = await api.DELETE("/api/v1/session");
              if (result.error) throw result.error;
              generation.current++;
              setMonitors([]);
              setJobs([]);
              setEditor(null);
              setSession("out");
            })
          }
        >
          退出登录
        </button>
      </header>
      <main>
        <div className="intro">
          <div>
            <p className="eyebrow">WORKSPACE / 01</p>
            <h1>我的热点观察</h1>
            <p className="muted">整理关键词，记录关注的议题与信息来源。</p>
          </div>
          <button
            disabled={loading || !sources.length}
            onClick={() => setEditor("new")}
          >
            新建监控
          </button>
        </div>
        <div className="notice">
          学习版本 ·
          目前支持监控草稿与任务链路诊断。各平台关键词和评论采集尚未接入。
        </div>
        {loading && <p role="status">正在加载工作台数据…</p>}
        {error && <p role="alert">{error}</p>}
        {editor && (
          <MonitorEditor
            key={
              typeof editor === "string" ? "new" : editor.id + editor.version
            }
            monitor={editor === "new" ? undefined : editor}
            sources={sources}
            onClose={() => setEditor(null)}
            onSaved={() => {
              setEditor(null);
              void refresh();
            }}
          />
        )}
        <section>
          <div className="section-title">
            <h2>
              监控草稿 <span className="count">{monitors.length}</span>
            </h2>
            <button
              className="secondary"
              disabled={busy}
              onClick={() => void refresh()}
            >
              刷新
            </button>
          </div>
          {monitors.length ? (
            <div className="cards">
              {monitors.map((m) => (
                <article className="panel" key={m.id}>
                  <span className="badge">草稿 · v{m.version}</span>
                  <h3>{m.title}</h3>
                  <div className="tags">
                    {m.keywords.map((k) => (
                      <span key={k}>{k}</span>
                    ))}
                  </div>
                  <p className="muted">
                    {m.sources
                      .map((s) => sourceLabels[s as Source["id"]] ?? s)
                      .join(" / ")}
                  </p>
                  <button
                    className="secondary"
                    aria-label={`编辑 ${m.title}`}
                    onClick={() => setEditor(m)}
                  >
                    编辑草稿
                  </button>
                </article>
              ))}
            </div>
          ) : (
            <div className="empty">
              <h3>从一个你关心的话题开始</h3>
              <p>创建草稿，选定关键词与国内外信息来源。</p>
            </div>
          )}
          {monitorCursor && (
            <button disabled={busy} onClick={() => void more("monitors")}>
              更多草稿
            </button>
          )}
        </section>
        <section className="panel">
          <div className="section-title">
            <div>
              <h2>任务链路诊断</h2>
              <p className="muted small">
                验证任务投递与结果入库，不采集社交媒体数据。点击刷新查看最新结果。
              </p>
            </div>
            <button
              disabled={busy}
              onClick={() =>
                void action(async () => {
                  diagnosticKey.current ??= crypto.randomUUID();
                  unwrap(
                    await api.POST("/api/v1/jobs", {
                      params: {
                        header: { "idempotency-key": diagnosticKey.current },
                      },
                      body: { kind: "verify_pipeline" },
                    }),
                  );
                  diagnosticKey.current = null;
                  await refresh();
                })
              }
            >
              运行诊断
            </button>
          </div>
          {jobs.length ? (
            <div className="table-wrap">
              <table>
                <thead>
                  <tr>
                    <th>任务</th>
                    <th>状态</th>
                    <th>尝试次数</th>
                    <th>操作</th>
                  </tr>
                </thead>
                <tbody>
                  {jobs.map((j) => (
                    <tr key={j.id}>
                      <td>
                        <code>{j.id.slice(0, 8)}</code>
                      </td>
                      <td>{statuses[j.status]}</td>
                      <td>{j.attempts}</td>
                      <td>
                        {["queued", "running"].includes(j.status) ? (
                          <button
                            className="secondary"
                            disabled={busy}
                            onClick={() =>
                              void action(async () => {
                                unwrap(
                                  await api.POST(
                                    "/api/v1/jobs/{identity}/cancel",
                                    { params: { path: { identity: j.id } } },
                                  ),
                                );
                                await refresh();
                              })
                            }
                          >
                            取消任务
                          </button>
                        ) : (
                          "—"
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ) : (
            <p className="muted">还没有诊断任务。</p>
          )}
          {jobCursor && (
            <button disabled={busy} onClick={() => void more("jobs")}>
              更多任务
            </button>
          )}
        </section>
        <footer>HotKey · 从可验证的数据开始</footer>
      </main>
    </>
  );
}
