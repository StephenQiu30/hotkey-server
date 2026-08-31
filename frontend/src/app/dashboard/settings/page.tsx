"use client";

import {
  useCallback,
  useEffect,
  useMemo,
  useState,
  type FormEvent,
} from "react";
import { Loader2, Plus, Radar } from "lucide-react";
import { toast } from "sonner";
import { MonitorCard } from "@/components/dashboard/MonitorCard";
import { MonitorFormDialog } from "@/components/dashboard/MonitorFormDialog";
import { ConfirmDeleteDialog } from "@/components/dashboard/ConfirmDeleteDialog";
import {
  CursorPagination,
  DEFAULT_PAGE_SIZE,
  hasNextCursor,
} from "@/components/dashboard/CursorPagination";
import { PageHeader } from "@/components/dashboard/PageHeader";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty";
import { PageShell } from "@/layouts/PageShell";
import { UserRole } from "@/lib/domainEnums";
import {
  compileAndPublishSimpleMonitor,
  emptyMonitorForm,
  monitorQuery,
  type MonitorScanState,
  type SimpleMonitorForm,
} from "@/lib/monitorWorkflow";
import { HotKeyAPIError } from "@/lib/request";
import {
  deleteMonitorsId,
  getMonitors,
  postMonitors,
  postMonitorsIdArchive,
  postMonitorsIdPause,
  postMonitorsIdResume,
  postMonitorsIdRestore,
} from "@/services/hotkey/hotkey-server/monitors";
import {
  getMonitorsIdScans,
  postMonitorsIdCollect,
} from "@/services/hotkey/hotkey-server/collectionRuns";
import { getSourceConnections } from "@/services/hotkey/hotkey-server/sources";
import { useAuthStore } from "@/stores/authStore";

type MonitorLifecycleAction = "pause" | "resume" | "archive" | "restore";

export default function MonitorsPage() {
  const user = useAuthStore((state) => state.user);
  const canContribute =
    user?.role === UserRole.Analyst ||
    user?.role === UserRole.Editor ||
    user?.role === UserRole.Admin;
  const canAdmin = user?.role === UserRole.Admin;
  const [monitors, setMonitors] = useState<HotKeyAPI.MonitorResponse[]>([]);
  const [sources, setSources] = useState<HotKeyAPI.SourceReadResponse[]>([]);
  const [scans, setScans] = useState<Record<number, MonitorScanState>>({});
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string>();
  const [forbidden, setForbidden] = useState(false);
  const [busyID, setBusyID] = useState<number>();
  const [saving, setSaving] = useState(false);
  const [createOpen, setCreateOpen] = useState(false);
  const [editTarget, setEditTarget] = useState<HotKeyAPI.MonitorResponse>();
  const [form, setForm] = useState<SimpleMonitorForm>(emptyMonitorForm);
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
      const result = await getMonitorsIdScans({ id: monitorID, limit: 1 });
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
      ...emptyMonitorForm(),
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
      query: monitorQuery(monitor),
      interval: String(monitor.collection_interval_seconds ?? 1800),
      alertEmailEnabled: monitor.alert_email_enabled ?? false,
      sourceIds: (monitor.sources ?? []).flatMap((source) =>
        source.source_connection_id == null
          ? []
          : [source.source_connection_id]
      ),
    });
    setCreateOpen(true);
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
        await compileAndPublishSimpleMonitor(
          editTarget.id,
          editTarget.version,
          fields
        );
      } else {
        const created = await postMonitors(fields);
        const monitorID = created.data?.id;
        const monitorVersion = created.data?.version;
        if (monitorID == null || monitorVersion == null) {
          throw new Error("监控创建响应无效");
        }
        await compileAndPublishSimpleMonitor(monitorID, monitorVersion, fields);
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
      globalThis.setTimeout(() => void loadScan(monitor.id as number), 1_500);
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "扫描提交失败");
    } finally {
      setBusyID(undefined);
    }
  }

  async function lifecycle(
    monitor: HotKeyAPI.MonitorResponse,
    action: MonitorLifecycleAction
  ) {
    if (
      !canContributeTo(monitor) ||
      monitor.id == null ||
      monitor.version == null
    )
      return;
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
    <PageShell>
      <PageHeader
        eyebrow="MONITORING"
        title="监控任务"
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
        <Card className="mb-6 p-5 text-sm">
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
        <Card role="alert" aria-label="权限不足" className="p-8 text-center">
          <p className="font-medium">权限不足</p>
          <p className="mt-2 text-sm text-muted-foreground">
            当前账号没有查看监控与扫描记录的权限，请联系管理员。
          </p>
        </Card>
      ) : loadError ? (
        <Card role="alert" className="p-8 text-center">
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
        <Card>
          <Empty className="h-72">
            <EmptyHeader>
              <EmptyMedia variant="icon">
                <Radar />
              </EmptyMedia>
              <EmptyTitle>还没有监控任务</EmptyTitle>
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
            return (
              <MonitorCard
                key={monitorID}
                monitor={monitor}
                scan={scans[monitorID]}
                busy={busyID === monitorID}
                canManage={canContributeTo(monitor)}
                canAdmin={canAdmin}
                onCollect={(target) => void collectNow(target)}
                onEdit={openEdit}
                onLifecycle={(target, action) => void lifecycle(target, action)}
                onDelete={setDeleteTarget}
              />
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

      <MonitorFormDialog
        open={createOpen}
        editTarget={editTarget}
        form={form}
        sources={enabledSources}
        saving={saving}
        onOpenChange={(open) => {
          setCreateOpen(open);
          if (!open) setEditTarget(undefined);
        }}
        onFormChange={setForm}
        onSubmit={saveMonitor}
      />

      <ConfirmDeleteDialog
        description="删除后不能恢复，已收集的信号记录仍会保留。"
        loading={busyID === deleteTarget?.id}
        onConfirm={deleteMonitor}
        onOpenChange={(open) => !open && setDeleteTarget(undefined)}
        open={deleteTarget != null}
        resourceName={deleteTarget?.name ?? "未命名监控"}
        title="删除监控？"
      />
    </PageShell>
  );
}
