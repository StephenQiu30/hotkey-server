import { useCallback, useEffect, useRef, useState } from "react";
import { createCollectionRun, listCollectionRuns } from "../api/collection";
import { listInboxContents, withdrawContent } from "../api/contents";
import {
  addEventMember,
  createEvent,
  listEvents,
  mergeEvent,
  removeEventMember,
  splitEvent,
} from "../api/events";
import { createDiagnosticJob, cancelJob, listJobs } from "../api/jobs";
import { getSession, logout } from "../api/identity";
import {
  activateMonitor,
  listMonitors,
  pauseMonitor,
  reviewMonitorMatch,
  startCommentTracking,
} from "../api/monitoring";
import { listNotifications, markNotificationRead } from "../api/notifications";
import { listSources } from "../api/sources";
import { Login } from "../features/identity/Login";
import { Runs } from "../features/collection/Runs";
import { Inbox, type InboxFilters } from "../features/contents/Inbox";
import { EventDossiers } from "../features/events/EventDossiers";
import { KnowledgeSearch } from "../features/knowledge/KnowledgeSearch";
import {
  MonitorEditor,
  sourceLabels,
} from "../features/monitors/MonitorEditor";
import { NotificationInbox } from "../features/notifications/NotificationInbox";
import {
  canCollect,
  SourceCapabilities,
} from "../features/sources/SourceCapabilities";
import { errorCode, message } from "../request";
type Job = API.JobView;
type Monitor = API.MonitorView;
type Source = API.SourceView;
type Content = API.InboxItem;
type CollectionRun = API.CollectionRunView;
type EventDossier = API.EventView;
type Notification = API.NotificationView;
const statuses: Record<Job["status"], string> = {
  queued: "排队中",
  running: "执行中",
  succeeded: "已完成",
  failed: "失败",
  cancelled: "已取消",
};
const monitorStates: Record<Monitor["state"], string> = {
  draft: "草稿",
  active: "运行中",
  paused: "已暂停",
};
const POLL_INTERVAL_MS = 2000;
const initialInboxFilters: InboxFilters = {
  monitorId: "",
  source: "",
  reviewState: "",
  discoveredPeriod: "",
  discoveredSince: "",
};

function inboxParams(
  filters: InboxFilters,
  cursor?: string | null,
): API.listInboxContentsParams {
  return {
    cursor: cursor ?? undefined,
    monitor_id: filters.monitorId || undefined,
    source: filters.source || undefined,
    review_state: filters.reviewState || undefined,
    discovered_since: filters.discoveredSince || undefined,
  };
}

function jobIsActive(job: Job): boolean {
  return job.status === "queued" || job.status === "running";
}

function runIsActive(run: CollectionRun): boolean {
  return run.state === "queued" || run.state === "running";
}

export function App() {
  const [session, setSession] = useState<"loading" | "in" | "out">("loading");
  const [error, setError] = useState("");
  const [monitors, setMonitors] = useState<Monitor[]>([]);
  const [jobs, setJobs] = useState<Job[]>([]);
  const [sources, setSources] = useState<Source[]>([]);
  const [contents, setContents] = useState<Content[]>([]);
  const [runs, setRuns] = useState<CollectionRun[]>([]);
  const [events, setEvents] = useState<EventDossier[]>([]);
  const [notifications, setNotifications] = useState<Notification[]>([]);
  const [unreadNotifications, setUnreadNotifications] = useState(0);
  const [editor, setEditor] = useState<Monitor | "new" | null>(null);
  const [busy, setBusy] = useState(false);
  const [loading, setLoading] = useState(true);
  const [monitorCursor, setMonitorCursor] = useState<string | null>(null);
  const [jobCursor, setJobCursor] = useState<string | null>(null);
  const [contentCursor, setContentCursor] = useState<string | null>(null);
  const [inboxFilters, setInboxFilters] = useState(initialInboxFilters);
  const [runCursor, setRunCursor] = useState<string | null>(null);
  const [eventCursor, setEventCursor] = useState<string | null>(null);
  const [notificationCursor, setNotificationCursor] = useState<string | null>(
    null,
  );
  const generation = useRef(0);
  const diagnosticKey = useRef<string | null>(null);
  const collectionKeys = useRef(new Map<string, string>());
  const refresh = useCallback(async () => {
    const current = ++generation.current;
    setLoading(true);
    setError("");
    try {
      const [m, j, s, c, r, e, n] = await Promise.all([
        listMonitors({}),
        listJobs({}),
        listSources(),
        listInboxContents(inboxParams(inboxFilters)),
        listCollectionRuns({}),
        listEvents({}),
        listNotifications({}),
      ]);
      if (current !== generation.current) return;
      setMonitors(m.items);
      setMonitorCursor(m.next_cursor);
      setJobs(j.items);
      setJobCursor(j.next_cursor);
      setSources(s);
      setContents(c.items);
      setContentCursor(c.next_cursor);
      setRuns(r.items);
      setRunCursor(r.next_cursor);
      setEvents(e.items);
      setEventCursor(e.next_cursor);
      setNotifications(n.items);
      setNotificationCursor(n.next_cursor);
      setUnreadNotifications(n.unread_count);
    } catch (e) {
      if (current !== generation.current) return;
      if (errorCode(e) === "authentication_required") setSession("out");
      else setError(message(e));
    } finally {
      if (current === generation.current) setLoading(false);
    }
  }, [inboxFilters]);
  useEffect(() => {
    const abort = new AbortController();
    getSession({ signal: abort.signal })
      .then(() => {
        if (abort.signal.aborted) return;
        setSession("in");
      })
      .catch((error: unknown) => {
        if (!abort.signal.aborted) {
          if (errorCode(error) !== "authentication_required")
            setError(message(error));
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
  const hasActiveWork = jobs.some(jobIsActive) || runs.some(runIsActive);
  useEffect(() => {
    if (session !== "in" || !hasActiveWork) return;
    let stopped = false;
    let timer: ReturnType<typeof setTimeout> | undefined;
    let activeController: AbortController | undefined;

    const schedule = () => {
      if (
        stopped ||
        document.visibilityState !== "visible" ||
        timer ||
        activeController
      )
        return;
      timer = setTimeout(() => {
        timer = undefined;
        void poll();
      }, POLL_INTERVAL_MS);
    };

    const poll = async () => {
      if (stopped || document.visibilityState !== "visible") return;
      const controller = new AbortController();
      activeController = controller;
      const observedGeneration = generation.current;
      let continuePolling = true;
      try {
        const [nextJobs, nextRuns] = await Promise.all([
          listJobs({}, { signal: controller.signal }),
          listCollectionRuns({}, { signal: controller.signal }),
        ]);
        if (
          stopped ||
          controller.signal.aborted ||
          observedGeneration !== generation.current
        )
          return;
        setJobs(nextJobs.items);
        setJobCursor(nextJobs.next_cursor);
        setRuns(nextRuns.items);
        setRunCursor(nextRuns.next_cursor);
        continuePolling =
          nextJobs.items.some(jobIsActive) || nextRuns.items.some(runIsActive);
        if (!continuePolling) await refresh();
      } catch (error: unknown) {
        if (!controller.signal.aborted && !stopped) setError(message(error));
      } finally {
        if (activeController === controller) activeController = undefined;
        if (continuePolling) schedule();
      }
    };

    const visibilityChanged = () => {
      if (document.visibilityState === "visible") {
        schedule();
        return;
      }
      if (timer) clearTimeout(timer);
      timer = undefined;
      activeController?.abort();
    };

    document.addEventListener("visibilitychange", visibilityChanged);
    schedule();
    return () => {
      stopped = true;
      if (timer) clearTimeout(timer);
      activeController?.abort();
      document.removeEventListener("visibilitychange", visibilityChanged);
    };
  }, [hasActiveWork, refresh, session]);
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
  async function cancelTrackedJob(jobId: string) {
    await action(async () => {
      await cancelJob({ identity: jobId });
      await refresh();
    });
  }
  async function runMonitor(monitor: Monitor) {
    await action(async () => {
      const key = collectionKeys.current.get(monitor.id) ?? crypto.randomUUID();
      collectionKeys.current.set(monitor.id, key);
      await createCollectionRun(
        { identity: monitor.id },
        {
          expected_version: monitor.current_version,
          idempotency_key: key,
        },
      );
      collectionKeys.current.delete(monitor.id);
      await refresh();
    });
  }
  async function more(
    kind:
      "monitors" | "jobs" | "contents" | "runs" | "events" | "notifications",
  ) {
    await action(async () => {
      if (kind === "monitors" && monitorCursor) {
        const page = await listMonitors({ cursor: monitorCursor });
        setMonitors((old) => [...old, ...page.items]);
        setMonitorCursor(page.next_cursor);
      } else if (kind === "jobs" && jobCursor) {
        const page = await listJobs({ cursor: jobCursor });
        setJobs((old) => [...old, ...page.items]);
        setJobCursor(page.next_cursor);
      } else if (kind === "contents" && contentCursor) {
        const page = await listInboxContents(
          inboxParams(inboxFilters, contentCursor),
        );
        setContents((old) => [...old, ...page.items]);
        setContentCursor(page.next_cursor);
      } else if (kind === "runs" && runCursor) {
        const page = await listCollectionRuns({ cursor: runCursor });
        setRuns((old) => [...old, ...page.items]);
        setRunCursor(page.next_cursor);
      } else if (kind === "events" && eventCursor) {
        const page = await listEvents({ cursor: eventCursor });
        setEvents((old) => [...old, ...page.items]);
        setEventCursor(page.next_cursor);
      } else if (kind === "notifications" && notificationCursor) {
        const page = await listNotifications({ cursor: notificationCursor });
        setNotifications((old) => [...old, ...page.items]);
        setNotificationCursor(page.next_cursor);
        setUnreadNotifications(page.unread_count);
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
              await logout();
              generation.current++;
              setMonitors([]);
              setJobs([]);
              setContents([]);
              setRuns([]);
              setEvents([]);
              setNotifications([]);
              setUnreadNotifications(0);
              collectionKeys.current.clear();
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
          配置历史与查询预览已接入。来源通过用途权限和采集连接检查后才能启用。
        </div>
        {loading && <p role="status">正在加载工作台数据…</p>}
        {error && <p role="alert">{error}</p>}
        {editor && (
          <MonitorEditor
            key={
              typeof editor === "string"
                ? "new"
                : editor.id + editor.current_version
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
        <SourceCapabilities sources={sources} />
        <Runs
          items={runs}
          nextCursor={runCursor}
          busy={busy}
          onMore={() => void more("runs")}
          onCancel={(jobId) => void cancelTrackedJob(jobId)}
        />
        <Inbox
          items={contents}
          nextCursor={contentCursor}
          busy={busy}
          events={events}
          monitors={monitors}
          sources={sources}
          filters={inboxFilters}
          onFiltersChange={setInboxFilters}
          onReview={async (matchId, reviewState) => {
            await action(async () => {
              await reviewMonitorMatch(
                { identity: matchId },
                { review_state: reviewState },
              );
              await refresh();
            });
          }}
          onTrackComments={async (matchId) => {
            await action(async () => {
              await startCommentTracking({ identity: matchId });
              await refresh();
            });
          }}
          onAssign={async (eventId, contentId) => {
            await action(async () => {
              await addEventMember(
                { identity: eventId },
                { content_id: contentId },
              );
              await refresh();
            });
          }}
          onWithdraw={async (contentId) => {
            await action(async () => {
              await withdrawContent(
                { identity: contentId },
                { reason: "purpose_revoked" },
              );
              await refresh();
            });
          }}
          onMore={() => void more("contents")}
        />
        <NotificationInbox
          items={notifications}
          unreadCount={unreadNotifications}
          nextCursor={notificationCursor}
          events={events}
          busy={busy}
          onRead={async (identity) => {
            await action(async () => {
              await markNotificationRead({ identity });
              await refresh();
            });
          }}
          onMore={() => void more("notifications")}
        />
        <EventDossiers
          items={events}
          nextCursor={eventCursor}
          busy={busy}
          onCreate={async (title, summary) => {
            await action(async () => {
              await createEvent({ title, summary });
              await refresh();
            });
          }}
          onRemove={async (eventId, contentId) => {
            await action(async () => {
              await removeEventMember({
                identity: eventId,
                content_id: contentId,
              });
              await refresh();
            });
          }}
          onMerge={async (
            sourceId,
            targetId,
            sourceRevision,
            targetRevision,
          ) => {
            await action(async () => {
              await mergeEvent(
                { identity: targetId },
                {
                  source_event_id: sourceId,
                  expected_source_revision: sourceRevision,
                  expected_target_revision: targetRevision,
                },
              );
              await refresh();
            });
          }}
          onSplit={async (sourceId, title, contentIds, revision) => {
            await action(async () => {
              await splitEvent(
                { identity: sourceId },
                {
                  title,
                  content_ids: contentIds,
                  expected_revision: revision,
                },
              );
              await refresh();
            });
          }}
          onNotificationsChanged={refresh}
          onMore={() => void more("events")}
        />
        <KnowledgeSearch events={events} />
        <section>
          <div className="section-title">
            <h2>
              监控配置 <span className="count">{monitors.length}</span>
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
                  <span className="badge">
                    {monitorStates[m.state]} · v{m.current_version}
                  </span>
                  <h3>{m.title}</h3>
                  <div className="tags">
                    {m.query_spec.include_any.map((k) => (
                      <span key={k}>{k}</span>
                    ))}
                  </div>
                  <p className="muted">
                    {m.source_ids.map((s) => sourceLabels[s] ?? s).join(" / ")}
                    {" · "}每 {m.schedule.interval_minutes} 分钟 · 每日最多{" "}
                    {m.budget.daily_requests} 次请求 · 证据保留{" "}
                    {m.schedule.retention_days} 天
                  </p>
                  <div className="actions">
                    {m.state !== "active" && (
                      <button
                        className="secondary"
                        aria-label={`编辑 ${m.title}`}
                        onClick={() => setEditor(m)}
                      >
                        编辑草稿
                      </button>
                    )}
                    {m.state !== "active" ? (
                      <button
                        className="secondary"
                        aria-label={`启用 ${m.title}`}
                        disabled={
                          busy ||
                          m.source_ids.some(
                            (id) =>
                              !sources.some(
                                (source) =>
                                  source.id === id && canCollect(source),
                              ),
                          )
                        }
                        onClick={() =>
                          void action(async () => {
                            await activateMonitor(
                              { identity: m.id },
                              { expected_version: m.current_version },
                            );
                            await refresh();
                          })
                        }
                      >
                        启用监控
                      </button>
                    ) : (
                      <>
                        <button
                          aria-label={`立即采集 ${m.title}`}
                          disabled={busy}
                          onClick={() => void runMonitor(m)}
                        >
                          立即采集
                        </button>
                        <button
                          className="secondary"
                          aria-label={`暂停 ${m.title}`}
                          disabled={busy}
                          onClick={() =>
                            void action(async () => {
                              await pauseMonitor(
                                { identity: m.id },
                                { expected_version: m.current_version },
                              );
                              await refresh();
                            })
                          }
                        >
                          暂停监控
                        </button>
                      </>
                    )}
                  </div>
                  {m.state !== "active" &&
                    m.source_ids.some(
                      (id) =>
                        !sources.some(
                          (source) => source.id === id && canCollect(source),
                        ),
                    ) && (
                      <p className="muted small">来源未准入，暂不能启用。</p>
                    )}
                </article>
              ))}
            </div>
          ) : (
            <div className="empty">
              <h3>从一个你关心的话题开始</h3>
              <p>创建配置，预览关键词如何映射到各信息来源。</p>
            </div>
          )}
          {monitorCursor && (
            <button disabled={busy} onClick={() => void more("monitors")}>
              更多配置
            </button>
          )}
        </section>
        <section className="panel">
          <div className="section-title">
            <div>
              <h2>任务链路诊断</h2>
              <p className="muted small">
                验证任务投递与结果入库，不采集社交媒体数据。活动任务会自动更新。
              </p>
            </div>
            <button
              disabled={busy}
              onClick={() =>
                void action(async () => {
                  diagnosticKey.current ??= crypto.randomUUID();
                  await createDiagnosticJob(
                    { kind: "verify_pipeline" },
                    {
                      headers: {
                        "Idempotency-Key": diagnosticKey.current,
                      },
                    },
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
                            onClick={() => void cancelTrackedJob(j.id)}
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
