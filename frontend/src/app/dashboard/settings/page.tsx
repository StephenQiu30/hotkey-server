"use client";

import {
  useCallback,
  useEffect,
  useMemo,
  useState,
  type FormEvent,
} from "react";
import {
  Archive,
  Loader2,
  Pause,
  Pencil,
  Play,
  Plus,
  Radar,
  RotateCcw,
  Search,
  Trash2,
} from "lucide-react";
import { toast } from "sonner";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { ConfirmDeleteDialog } from "@/components/dashboard/ConfirmDeleteDialog";
import {
  CursorPagination,
  DEFAULT_PAGE_SIZE,
  hasNextCursor,
} from "@/components/dashboard/CursorPagination";
import { PageHeader } from "@/components/dashboard/PageHeader";
import { MonitorStatus, UserRole } from "@/lib/domainEnums";
import { monitorStatusLabel } from "@/lib/domainPresentation";
import { HotKeyAPIError } from "@/lib/request";
import { sourceTypeLabel } from "@/lib/sourceLabels";
import {
  deleteMonitorsId,
  getMonitors,
  postMonitors,
  postMonitorsIdArchive,
  postMonitorsIdPause,
  postMonitorsIdResume,
  postMonitorsIdRestore,
  putMonitorsId,
} from "@/services/hotkey/hotkey-server/monitors";
import {
  getMonitorsIdScans,
  postMonitorsIdCollect,
} from "@/services/hotkey/hotkey-server/collectionRuns";
import { getSourceConnections } from "@/services/hotkey/hotkey-server/sources";
import { useAuthStore } from "@/stores/authStore";

type SimpleMonitorForm = {
  name: string;
  query: string;
  interval: string;
  alertEmailEnabled: boolean;
  sourceIds: number[];
};

type ScanState = {
  queued?: boolean;
  items: HotKeyAPI.MonitorScanResponse[];
};

const emptyForm = (): SimpleMonitorForm => ({
  name: "",
  query: "",
  interval: "1800",
  alertEmailEnabled: true,
  sourceIds: [],
});

const scanStatusLabels: Readonly<Record<string, string>> = {
  queued: "已排队",
  running: "扫描中",
  succeeded: "成功",
  partial: "部分成功",
  failed: "失败",
  cancelled: "已取消",
};

function queryOf(monitor: HotKeyAPI.MonitorResponse) {
  return monitor.query ?? monitor.name ?? "";
}

function intervalLabel(seconds: number | undefined) {
  if (!seconds) return "—";
  if (seconds % 3600 === 0) return `${seconds / 3600} 小时`;
  return `${seconds / 60} 分钟`;
}

function formatTime(value: string | undefined) {
  if (!value) return "尚未运行";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "尚未运行";
  return new Intl.DateTimeFormat("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  }).format(date);
}

export default function MonitorsPage() {
  const user = useAuthStore((state) => state.user);
  const canContribute =
    user?.role === UserRole.Analyst ||
    user?.role === UserRole.Editor ||
    user?.role === UserRole.Admin;
  const canAdmin = user?.role === UserRole.Admin;
  const [monitors, setMonitors] = useState<HotKeyAPI.MonitorResponse[]>([]);
  const [sources, setSources] = useState<HotKeyAPI.SourceReadResponse[]>([]);
  const [scans, setScans] = useState<Record<number, ScanState>>({});
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string>();
  const [forbidden, setForbidden] = useState(false);
  const [busyID, setBusyID] = useState<number>();
  const [saving, setSaving] = useState(false);
  const [createOpen, setCreateOpen] = useState(false);
  const [editTarget, setEditTarget] = useState<HotKeyAPI.MonitorResponse>();
  const [form, setForm] = useState<SimpleMonitorForm>(emptyForm);
  const [deleteTarget, setDeleteTarget] = useState<HotKeyAPI.MonitorResponse>();
  const [pageSize, setPageSize] = useState(DEFAULT_PAGE_SIZE);
  const [page, setPage] = useState(1);
  const [cursors, setCursors] = useState<(string | undefined)[]>([undefined]);
  const [nextCursor, setNextCursor] = useState<string>();

  const enabledSources = useMemo(
    () => sources.filter((source) => source.enabled && !source.deleted),
    [sources]
  );

  const loadScan = useCallback(async (monitorID: number) => {
    try {
      const result = await getMonitorsIdScans({ id: monitorID, limit: 20 });
      setScans((current) => ({
        ...current,
        [monitorID]: { items: result.data?.items ?? [] },
      }));
    } catch {
      setScans((current) => ({ ...current, [monitorID]: { items: [] } }));
    }
  }, []);

  const loadPage = useCallback(
    async (cursor: string | undefined, pageNumber: number) => {
      setLoading(true);
      setLoadError(undefined);
      setForbidden(false);
      try {
        const [monitorResult, sourceResult] = await Promise.all([
          getMonitors({ limit: pageSize, ...(cursor ? { cursor } : {}) }),
          canContribute
            ? getSourceConnections({ limit: 100 })
            : Promise.resolve(undefined),
        ]);
        const items = monitorResult.data?.items ?? [];
        setMonitors(items);
        setSources(sourceResult?.data?.items ?? []);
        setPage(pageNumber);
        setNextCursor(monitorResult.data?.next_cursor);
        await Promise.all(
          items.flatMap((monitor) =>
            monitor.id == null ? [] : [loadScan(monitor.id)]
          )
        );
      } catch (reason) {
        setForbidden(reason instanceof HotKeyAPIError && reason.status === 403);
        setLoadError(reason instanceof Error ? reason.message : "监控加载失败");
      } finally {
        setLoading(false);
      }
    },
    [canContribute, loadScan, pageSize]
  );

  const canContributeTo = useCallback(
    (monitor: HotKeyAPI.MonitorResponse) =>
      user?.role === UserRole.Editor ||
      user?.role === UserRole.Admin ||
      (user?.role === UserRole.Analyst &&
        user.id != null &&
        monitor.created_by_user_id === user.id),
    [user?.id, user?.role]
  );

  useEffect(() => {
    void loadPage(undefined, 1);
  }, [loadPage]);

  function openCreate() {
    setEditTarget(undefined);
    setForm({
      ...emptyForm(),
      sourceIds: enabledSources.flatMap((source) =>
        source.id == null ? [] : [source.id]
      ),
    });
    setCreateOpen(true);
  }

  function openEdit(monitor: HotKeyAPI.MonitorResponse) {
    setEditTarget(monitor);
    setForm({
      name: monitor.name ?? "",
      query: queryOf(monitor),
      interval: String(monitor.collection_interval_seconds ?? 1800),
      alertEmailEnabled: monitor.alert_email_enabled ?? false,
      sourceIds: (monitor.sources ?? []).flatMap((source) =>
        source.source_connection_id == null ? [] : [source.source_connection_id]
      ),
    });
    setCreateOpen(true);
  }

  function toggleSource(sourceID: number) {
    setForm((current) => ({
      ...current,
      sourceIds: current.sourceIds.includes(sourceID)
        ? current.sourceIds.filter((value) => value !== sourceID)
        : [...current.sourceIds, sourceID].slice(0, 10),
    }));
  }

  async function saveMonitor(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const name = form.name.trim();
    const query = form.query.trim();
    if (!name || !query || form.sourceIds.length === 0) {
      toast.error("请填写名称、监控词并至少选择一个来源");
      return;
    }
    setSaving(true);
    try {
      const fields = {
        name,
        query,
        source_connection_ids: form.sourceIds,
        collection_interval_seconds: Number(form.interval),
        alert_email_enabled: form.alertEmailEnabled,
      };
      if (editTarget?.id != null && editTarget.version != null) {
        await putMonitorsId(
          { id: editTarget.id },
          { ...fields, expected_monitor_version: editTarget.version }
        );
      } else {
        await postMonitors(fields);
      }
      setCreateOpen(false);
      setEditTarget(undefined);
      await loadPage(undefined, 1);
      toast.success(editTarget ? "监控已更新" : "监控已创建并启用");
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "监控保存失败");
    } finally {
      setSaving(false);
    }
  }

  async function collectNow(monitor: HotKeyAPI.MonitorResponse) {
    if (!canContributeTo(monitor) || monitor.id == null) return;
    setBusyID(monitor.id);
    try {
      await postMonitorsIdCollect({ id: monitor.id });
      setScans((current) => ({
        ...current,
        [monitor.id as number]: {
          queued: true,
          items: current[monitor.id as number]?.items ?? [],
        },
      }));
      toast.success("扫描任务已提交；重复点击会复用当前五分钟任务");
      window.setTimeout(() => void loadScan(monitor.id as number), 1_500);
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "扫描提交失败");
    } finally {
      setBusyID(undefined);
    }
  }

  async function lifecycle(
    monitor: HotKeyAPI.MonitorResponse,
    action: "pause" | "resume" | "archive" | "restore"
  ) {
    if (!canContributeTo(monitor) || monitor.id == null || monitor.version == null) return;
    setBusyID(monitor.id);
    const body = { expected_monitor_version: monitor.version };
    try {
      if (action === "pause")
        await postMonitorsIdPause({ id: monitor.id }, body);
      else if (action === "resume")
        await postMonitorsIdResume({ id: monitor.id }, body);
      else if (action === "archive")
        await postMonitorsIdArchive({ id: monitor.id }, body);
      else await postMonitorsIdRestore({ id: monitor.id }, body);
      await loadPage(undefined, 1);
      toast.success("监控状态已更新");
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "监控操作失败");
    } finally {
      setBusyID(undefined);
    }
  }

  async function deleteMonitor() {
    if (!canAdmin || deleteTarget?.id == null || deleteTarget.version == null)
      return;
    setBusyID(deleteTarget.id);
    try {
      await deleteMonitorsId(
        { id: deleteTarget.id },
        { expected_monitor_version: deleteTarget.version }
      );
      setDeleteTarget(undefined);
      await loadPage(undefined, 1);
      toast.success("监控已删除");
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "监控删除失败");
    } finally {
      setBusyID(undefined);
    }
  }

  function nextPage() {
    if (!nextCursor) return;
    const nextPageNumber = page + 1;
    setCursors((history) => [...history.slice(0, page), nextCursor]);
    void loadPage(nextCursor, nextPageNumber);
  }

  function previousPage() {
    if (page <= 1) return;
    void loadPage(cursors[page - 2], page - 1);
  }

  return (
    <div className="app-page">
      <PageHeader
        eyebrow="MONITORING"
        title="热点监控"
        description="填写监控词并选择来源后立即启用。系统定时扫描，也可以随时手动触发一次。"
        action={
          canContribute ? (
            <Button onClick={openCreate} disabled={enabledSources.length === 0}>
              <Plus className="h-4 w-4" />
              新建监控
            </Button>
          ) : undefined
        }
      />

      {canContribute && enabledSources.length === 0 && !loading ? (
        <Card className="mb-6 border border-border bg-card p-5 text-sm">
          尚无可用来源，请管理员先在“来源”中接入 Hacker News、RSS 或授权平台。
        </Card>
      ) : null}

      {loading ? (
        <div
          role="status"
          aria-label="正在加载监控"
          className="flex h-72 items-center justify-center"
        >
          <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
        </div>
      ) : forbidden ? (
        <Card
          role="alert"
          aria-label="权限不足"
          className="border border-border bg-card p-8 text-center"
        >
          <p className="font-medium">权限不足</p>
          <p className="mt-2 text-sm text-muted-foreground">
            当前账号没有查看监控与扫描记录的权限，请联系管理员。
          </p>
        </Card>
      ) : loadError ? (
        <Card
          role="alert"
          className="border border-border bg-card p-8 text-center"
        >
          <p className="font-medium">监控加载失败</p>
          <p className="mt-2 text-sm text-muted-foreground">{loadError}</p>
          <Button
            className="mt-4"
            variant="outline"
            onClick={() => void loadPage(undefined, 1)}
          >
            重试
          </Button>
        </Card>
      ) : monitors.length === 0 ? (
        <Card className="border border-border bg-card">
          <Empty className="h-72">
            <EmptyHeader>
              <EmptyMedia variant="icon">
                <Radar />
              </EmptyMedia>
              <EmptyTitle>还没有热点监控</EmptyTitle>
              <EmptyDescription>
                {canContribute
                  ? "新建一个监控，系统会按设定间隔持续发现新内容。"
                  : "当前没有可查看的监控。"}
              </EmptyDescription>
            </EmptyHeader>
          </Empty>
        </Card>
      ) : (
        <div className="space-y-4">
          {monitors.map((monitor) => {
            const monitorID = monitor.id ?? 0;
            const query = queryOf(monitor);
            const scan = scans[monitorID];
            const latest = scan?.items[0];
            const busy = busyID === monitorID;
            const canManageMonitor = canContributeTo(monitor);
            return (
              <Card key={monitorID} className="border border-border bg-card">
                <CardHeader className="gap-4 p-5 pb-3 sm:p-6 sm:pb-3">
                  <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
                    <div>
                      <div className="flex flex-wrap items-center gap-2">
                        <CardTitle className="text-xl">
                          <h2>{monitor.name}</h2>
                        </CardTitle>
                        <Badge variant="outline">
                          {monitorStatusLabel(monitor.status)}
                        </Badge>
                      </div>
                      <p className="mt-2 text-sm text-muted-foreground">
                        监控词：{query}
                      </p>
                      <p className="mt-1 text-xs text-muted-foreground">
                        每 {intervalLabel(monitor.collection_interval_seconds)}
                        扫描 · {monitor.sources?.length ?? 0} 个来源
                      </p>
                    </div>
                    <div className="flex flex-wrap gap-2">
                      {canManageMonitor && monitor.status === MonitorStatus.Active ? (
                        <Button
                          aria-label={`立即扫描 ${monitor.name}`}
                          disabled={busy}
                          size="sm"
                          onClick={() => void collectNow(monitor)}
                        >
                          {busy ? (
                            <Loader2 className="animate-spin" />
                          ) : (
                            <Search />
                          )}
                          立即扫描
                        </Button>
                      ) : null}
                      {canManageMonitor && monitor.status !== MonitorStatus.Archived ? (
                        <Button
                          aria-label={`编辑 ${monitor.name}`}
                          size="sm"
                          variant="outline"
                          disabled={busy}
                          onClick={() => openEdit(monitor)}
                        >
                          <Pencil />
                          编辑
                        </Button>
                      ) : null}
                      {canManageMonitor && monitor.status === MonitorStatus.Active ? (
                        <Button
                          size="sm"
                          variant="outline"
                          disabled={busy}
                          onClick={() => void lifecycle(monitor, "pause")}
                        >
                          <Pause />
                          暂停
                        </Button>
                      ) : null}
                      {canManageMonitor && monitor.status === MonitorStatus.Paused ? (
                        <Button
                          size="sm"
                          variant="outline"
                          disabled={busy}
                          onClick={() => void lifecycle(monitor, "resume")}
                        >
                          <Play />
                          恢复
                        </Button>
                      ) : null}
                      {canManageMonitor && monitor.status !== MonitorStatus.Archived ? (
                        <Button
                          size="sm"
                          variant="ghost"
                          disabled={busy}
                          onClick={() => void lifecycle(monitor, "archive")}
                        >
                          <Archive />
                          归档
                        </Button>
                      ) : null}
                      {canManageMonitor && monitor.status === MonitorStatus.Archived ? (
                        <>
                          <Button
                            size="sm"
                            variant="outline"
                            disabled={busy}
                            onClick={() => void lifecycle(monitor, "restore")}
                          >
                            <RotateCcw />
                            恢复
                          </Button>
                          {canAdmin ? (
                            <Button
                              size="sm"
                              variant="ghost"
                              disabled={busy}
                              onClick={() => setDeleteTarget(monitor)}
                            >
                              <Trash2 />
                              删除
                            </Button>
                          ) : null}
                        </>
                      ) : null}
                    </div>
                  </div>
                </CardHeader>
                <CardContent className="p-5 pt-2 sm:p-6 sm:pt-2">
                  <h3 className="mb-3 text-sm font-medium">最近扫描</h3>
                  {scan?.queued ? (
                    <div className="mb-3 flex items-center gap-2 rounded-lg border border-border px-3 py-2 text-sm">
                      <Loader2 className="h-4 w-4 animate-spin" />
                      已排队，等待来源返回
                    </div>
                  ) : null}
                  {latest ? (
                    <div className="space-y-3">
                      <div className="flex flex-wrap items-center justify-between gap-2 text-sm">
                        <div className="flex items-center gap-2">
                          <Badge
                            variant={
                              latest.status === "failed"
                                ? "destructive"
                                : "secondary"
                            }
                          >
                            {scanStatusLabels[latest.status ?? ""] ??
                              latest.status}
                          </Badge>
                          <span className="text-muted-foreground">
                            接受 {latest.accepted_count ?? 0} / 候选{" "}
                            {latest.candidate_count ?? 0}
                          </span>
                        </div>
                        <span className="text-xs text-muted-foreground">
                          {formatTime(
                            latest.finished_at ||
                              latest.started_at ||
                              latest.scheduled_at
                          )}
                        </span>
                      </div>
                      <div className="grid gap-2 md:grid-cols-2">
                        {(latest.sources ?? []).map((item) => (
                          <div
                            key={`${item.run_id}-${item.source_connection_id}`}
                            className="rounded-lg border border-border p-3"
                          >
                            <div className="flex items-center justify-between gap-3">
                              <p className="text-sm font-medium">
                                {item.source_name ||
                                  sourceTypeLabel(item.source_type)}
                              </p>
                              <Badge
                                variant={
                                  item.status === "failed"
                                    ? "destructive"
                                    : "secondary"
                                }
                              >
                                {scanStatusLabels[item.status ?? ""] ??
                                  item.status}
                              </Badge>
                            </div>
                            <p className="mt-2 text-xs text-muted-foreground">
                              {item.status === "succeeded"
                                ? `成功 · 接受 ${
                                    item.accepted_count ?? 0
                                  } / 候选 ${item.candidate_count ?? 0}`
                                : item.error_code || "等待来源返回"}
                            </p>
                            <p className="mt-1 text-xs text-muted-foreground">
                              {formatTime(
                                item.finished_at ||
                                  item.started_at ||
                                  item.scheduled_at
                              )}
                            </p>
                          </div>
                        ))}
                      </div>
                    </div>
                  ) : !scan?.queued ? (
                    <p className="text-sm text-muted-foreground">
                      尚无扫描记录。
                    </p>
                  ) : null}
                </CardContent>
              </Card>
            );
          })}
          <CursorPagination
            hasNext={hasNextCursor(nextCursor)}
            loading={loading}
            onNext={nextPage}
            onPageSizeChange={setPageSize}
            onPrevious={previousPage}
            page={page}
            pageSize={pageSize}
          />
        </div>
      )}

      <Dialog
        open={createOpen}
        onOpenChange={(open) => {
          setCreateOpen(open);
          if (!open) setEditTarget(undefined);
        }}
      >
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>
              {editTarget ? "编辑热点监控" : "新建热点监控"}
            </DialogTitle>
            <DialogDescription>
              {editTarget
                ? "修改简单字段后立即生效；暂停中的监控仍保持暂停。"
                : "只需填写监控词和来源；创建后立即启用。"}
            </DialogDescription>
          </DialogHeader>
          <form className="space-y-5" onSubmit={saveMonitor}>
            <div>
              <Label htmlFor="monitor-name">监控名称</Label>
              <Input
                id="monitor-name"
                className="mt-2"
                value={form.name}
                maxLength={120}
                onChange={(event) =>
                  setForm((current) => ({
                    ...current,
                    name: event.target.value,
                  }))
                }
              />
            </div>
            <div>
              <Label htmlFor="monitor-query">监控词</Label>
              <Input
                id="monitor-query"
                className="mt-2"
                value={form.query}
                maxLength={160}
                placeholder="例如 Claude、OpenAI、具身智能"
                onChange={(event) =>
                  setForm((current) => ({
                    ...current,
                    query: event.target.value,
                  }))
                }
              />
            </div>
            <div>
              <Label htmlFor="monitor-interval">扫描间隔</Label>
              <Select
                value={form.interval}
                onValueChange={(interval) =>
                  setForm((current) => ({ ...current, interval }))
                }
              >
                <SelectTrigger
                  id="monitor-interval"
                  aria-label="扫描间隔"
                  className="mt-2"
                >
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="900">15 分钟</SelectItem>
                  <SelectItem value="1800">30 分钟</SelectItem>
                  <SelectItem value="3600">1 小时</SelectItem>
                  <SelectItem value="21600">6 小时</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <fieldset>
              <legend className="text-sm font-medium">来源</legend>
              <div className="mt-3 grid gap-3 sm:grid-cols-2">
                {enabledSources.map((source) =>
                  source.id == null ? null : (
                    <label
                      key={source.id}
                      className="flex items-center gap-2 text-sm"
                    >
                      <Checkbox
                        checked={form.sourceIds.includes(source.id)}
                        onCheckedChange={() =>
                          toggleSource(source.id as number)
                        }
                      />
                      {source.name || sourceTypeLabel(source.source_type)}
                    </label>
                  )
                )}
              </div>
            </fieldset>
            <label className="flex items-start gap-3 rounded-lg border border-border p-3">
              <Checkbox
                aria-label="高优先级邮件提醒"
                checked={form.alertEmailEnabled}
                onCheckedChange={(checked) =>
                  setForm((current) => ({
                    ...current,
                    alertEmailEnabled: checked === true,
                  }))
                }
              />
              <span>
                <span className="block text-sm font-medium">
                  高优先级邮件提醒
                </span>
                <span className="mt-1 block text-xs text-muted-foreground">
                  仅在热点达到高或紧急等级时发送到当前账号邮箱。
                </span>
              </span>
            </label>
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => setCreateOpen(false)}
              >
                取消
              </Button>
              <Button
                type="submit"
                disabled={saving || form.sourceIds.length === 0}
              >
                {saving ? <Loader2 className="animate-spin" /> : <RotateCcw />}
                {editTarget ? "保存修改" : "创建并启用"}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <ConfirmDeleteDialog
        description="删除后不能恢复，已收集的热点记录仍会保留。"
        loading={busyID === deleteTarget?.id}
        onConfirm={deleteMonitor}
        onOpenChange={(open) => !open && setDeleteTarget(undefined)}
        open={deleteTarget != null}
        resourceName={deleteTarget?.name ?? "未命名监控"}
        title="删除监控？"
      />
    </div>
  );
}
