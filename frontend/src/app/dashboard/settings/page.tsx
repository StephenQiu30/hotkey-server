"use client";

import { type FormEvent, useCallback, useEffect, useState } from "react";
import Link from "next/link";
import {
  Archive,
  BrainCircuit,
  Eye,
  Loader2,
  ListChecks,
  MoreHorizontal,
  Pause,
  Pencil,
  Play,
  Plus,
  Radar,
  RotateCcw,
  Search,
  Sparkles,
  Trash2,
} from "lucide-react";
import { toast } from "sonner";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty";
import { ConfirmDeleteDialog } from "@/components/dashboard/ConfirmDeleteDialog";
import { AICandidateDialog } from "@/components/dashboard/AICandidateDialog";
import {
  CursorPagination,
  DEFAULT_PAGE_SIZE,
  hasNextCursor,
} from "@/components/dashboard/CursorPagination";
import { MonitorDetailDialog } from "@/components/dashboard/MonitorDetailDialog";
import { MonitorDraftDialog } from "@/components/dashboard/MonitorDraftDialog";
import { PageHeader } from "@/components/dashboard/PageHeader";
import { ManualSearchQuota } from "@/components/dashboard/ManualSearchQuota";
import { MonitorRegion, MonitorStatus, UserRole } from "@/lib/domainEnums";
import { monitorStatusLabel } from "@/lib/domainPresentation";
import {
  buildMonitorDraftRequest,
  createMonitorRule,
  monitorToDraftForm,
  selectAllMonitorSources,
  validateMonitorDraft,
  type MonitorDraftForm,
} from "@/lib/monitorDraft";
import {
  deleteMonitorsId,
  getMonitors,
  getMonitorsId,
  getMonitorsIdVersions,
  postMonitors,
  postMonitorsIdArchive,
  postMonitorsIdDraftAiCandidates,
  postMonitorsIdDraftRulesRuleIdApproval,
  postMonitorsIdPause,
  postMonitorsIdPreview,
  postMonitorsIdPublish,
  postMonitorsIdResume,
  postMonitorsIdRestore,
  putMonitorsIdDraft,
} from "@/services/hotkey/hotkey-server/monitors";
import { postMonitorsIdCollect } from "@/services/hotkey/hotkey-server/collectionRuns";
import { getSourceConnections } from "@/services/hotkey/hotkey-server/sources";
import { useAuthStore } from "@/stores/authStore";

const initialForm = (): MonitorDraftForm => ({
  name: "",
  description: "",
  rules: [createMonitorRule()],
  languages: ["zh"],
  region: MonitorRegion.China,
  interval: 900,
  relevance: 60,
  event: 70,
  retention: 30,
  sourceIds: [],
  alertHeat: 70,
  alertMomentum: 55,
  alertBreadth: 25,
  alertWarning: 75,
  alertCritical: 90,
  alertCooldown: 60,
  alertEmailEnabled: false,
  alertEmailMinSeverity: "critical",
});

type DraftDialogState = {
  mode: "create" | "edit";
  monitor?: HotKeyAPI.MonitorResponse;
};
type DetailState = {
  error?: string;
  history: HotKeyAPI.MonitorConfigResponse[];
  loading: boolean;
  monitor?: HotKeyAPI.MonitorResponse;
  open: boolean;
};

export default function MonitorsPage() {
  const user = useAuthStore((state) => state.user);
  const canEdit =
    user?.role === UserRole.Editor || user?.role === UserRole.Admin;
  const canAdmin = user?.role === UserRole.Admin;
  const [monitors, setMonitors] = useState<HotKeyAPI.MonitorResponse[]>([]);
  const [sources, setSources] = useState<HotKeyAPI.SourceReadResponse[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string>();
  const [pageSize, setPageSize] = useState(DEFAULT_PAGE_SIZE);
  const [page, setPage] = useState(1);
  const [cursors, setCursors] = useState<(string | undefined)[]>([undefined]);
  const [nextCursor, setNextCursor] = useState<string>();
  const [busyID, setBusyID] = useState<number>();
  const [saving, setSaving] = useState(false);
  const [draftDialog, setDraftDialog] = useState<DraftDialogState>();
  const [form, setForm] = useState<MonitorDraftForm>(initialForm);
  const [deleteTarget, setDeleteTarget] = useState<HotKeyAPI.MonitorResponse>();
  const [previewTarget, setPreviewTarget] =
    useState<HotKeyAPI.MonitorResponse>();
  const [preview, setPreview] = useState<HotKeyAPI.PreviewResponse>();
  const [detail, setDetail] = useState<DetailState>({
    history: [],
    loading: false,
    open: false,
  });
  const [candidateTarget, setCandidateTarget] =
    useState<HotKeyAPI.MonitorResponse>();
  const [approvingRuleID, setApprovingRuleID] = useState<number>();
  const [quotaRefreshKey, setQuotaRefreshKey] = useState(0);

  const load = useCallback(async () => {
    setLoading(true);
    setLoadError(undefined);
    try {
      const monitorRequest = getMonitors({ limit: pageSize });
      const [monitorResult, sourceResult] = await Promise.all([
        monitorRequest,
        canEdit
          ? getSourceConnections({ limit: 100 })
          : Promise.resolve(undefined),
      ]);
      setMonitors(monitorResult.data?.items ?? []);
      setSources(
        (sourceResult?.data?.items ?? []).filter(
          (source) => source.enabled && !source.deleted
        )
      );
      setPage(1);
      setCursors([undefined]);
      setNextCursor(monitorResult.data?.next_cursor);
    } catch (reason) {
      const message = reason instanceof Error ? reason.message : "监控加载失败";
      setLoadError(message);
    } finally {
      setLoading(false);
    }
  }, [canEdit, pageSize]);

  useEffect(() => {
    void load();
  }, [load]);

  const loadMonitorPage = async (
    cursor: string | undefined,
    pageNumber: number
  ) => {
    setLoading(true);
    setLoadError(undefined);
    try {
      const result = await getMonitors({
        limit: pageSize,
        ...(cursor ? { cursor } : {}),
      });
      setMonitors(result.data?.items ?? []);
      setPage(pageNumber);
      setNextCursor(result.data?.next_cursor);
    } catch (reason) {
      setLoadError(reason instanceof Error ? reason.message : "监控加载失败");
    } finally {
      setLoading(false);
    }
  };

  const nextPage = () => {
    if (!nextCursor) return;
    const nextPageNumber = page + 1;
    setCursors((history) => [...history.slice(0, page), nextCursor]);
    void loadMonitorPage(nextCursor, nextPageNumber);
  };
  const previousPage = () =>
    page > 1 && void loadMonitorPage(cursors[page - 2], page - 1);

  const openCreate = () => {
    setForm({
      ...initialForm(),
      sourceIds: selectAllMonitorSources(
        sources.flatMap((source) => (source.id == null ? [] : [source.id]))
      ),
    });
    setDraftDialog({ mode: "create" });
  };
  const openEdit = (monitor: HotKeyAPI.MonitorResponse) => {
    setForm(monitorToDraftForm(monitor));
    setDraftDialog({ mode: "edit", monitor });
  };

  const saveDraft = async (event: FormEvent) => {
    event.preventDefault();
    const normalized = {
      ...form,
      name:
        form.name.trim() ||
        `热点监控 · ${
          form.rules
            .find((rule) => rule.value.trim())
            ?.value.trim()
            .slice(0, 48) ?? "未命名"
        }`,
    };
    const validation = validateMonitorDraft(normalized);
    if (validation) return toast.error(validation);
    setSaving(true);
    try {
      const request = buildMonitorDraftRequest(normalized);
      const target = draftDialog?.monitor;
      if (draftDialog?.mode === "edit" && target?.id != null) {
        await putMonitorsIdDraft({ id: target.id }, {
          ...request,
          expected_monitor_version: target.version ?? 0,
          expected_draft_version: target.draft?.version ?? null,
        } as unknown as HotKeyAPI.ReplaceDraftRequest);
        toast.success("监控草稿已保存");
      } else {
        await postMonitors(request);
        toast.success("监控草稿已创建");
      }
      setDraftDialog(undefined);
      await load();
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "草稿保存失败");
    } finally {
      setSaving(false);
    }
  };

  const previewMonitor = async (monitor: HotKeyAPI.MonitorResponse) => {
    if (monitor.id == null) return;
    setBusyID(monitor.id);
    try {
      const result = await postMonitorsIdPreview({ id: monitor.id });
      setPreview(result.data);
      setPreviewTarget(monitor);
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "配置预览失败");
    } finally {
      setBusyID(undefined);
    }
  };

  const publish = async () => {
    if (previewTarget?.id == null || previewTarget.draft?.version == null)
      return;
    setBusyID(previewTarget.id);
    try {
      await postMonitorsIdPublish(
        { id: previewTarget.id },
        {
          expected_monitor_version: previewTarget.version ?? 0,
          expected_draft_version: previewTarget.draft.version,
        }
      );
      setPreviewTarget(undefined);
      setPreview(undefined);
      await load();
      toast.success("监控已发布");
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "监控发布失败");
    } finally {
      setBusyID(undefined);
    }
  };

  const lifecycle = async (
    monitor: HotKeyAPI.MonitorResponse,
    action: "pause" | "resume" | "archive" | "restore"
  ) => {
    if (monitor.id == null) return;
    setBusyID(monitor.id);
    const body = {
      expected_monitor_version: monitor.version ?? 0,
    } as unknown as HotKeyAPI.LifecycleRequest;
    try {
      if (action === "pause")
        await postMonitorsIdPause({ id: monitor.id }, body);
      else if (action === "resume")
        await postMonitorsIdResume({ id: monitor.id }, body);
      else if (action === "restore")
        await postMonitorsIdRestore({ id: monitor.id }, body);
      else await postMonitorsIdArchive({ id: monitor.id }, body);
      await load();
      toast.success("监控状态已更新");
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "监控操作失败");
    } finally {
      setBusyID(undefined);
    }
  };

  const collectNow = async (monitor: HotKeyAPI.MonitorResponse) => {
    if (!canEdit || monitor.id == null) return;
    setBusyID(monitor.id);
    try {
      const result = await postMonitorsIdCollect({ id: monitor.id });
      const created = result.data?.created ?? 0;
      const reused = result.data?.reused ?? 0;
      const cooldown = result.data?.cooldown_until
        ? new Intl.DateTimeFormat("zh-CN", {
            hour: "2-digit",
            minute: "2-digit",
          }).format(new Date(result.data.cooldown_until))
        : "稍后";
      toast.success(
        `立即搜索已提交：新建 ${created}，复用 ${reused}；${cooldown} 后可再次提交。可前往采集内容查看进度。`
      );
      if (created > 0) setQuotaRefreshKey((value) => value + 1);
    } catch (reason) {
      toast.error(
        reason instanceof Error ? reason.message : "立即搜索提交失败"
      );
    } finally {
      setBusyID(undefined);
    }
  };

  const deleteMonitor = async () => {
    if (deleteTarget?.id == null) return;
    setBusyID(deleteTarget.id);
    try {
      await deleteMonitorsId({ id: deleteTarget.id }, {
        expected_monitor_version: deleteTarget.version ?? 0,
      } as unknown as HotKeyAPI.LifecycleRequest);
      setDeleteTarget(undefined);
      await load();
      toast.success("监控已删除，历史事件与报告仍保留");
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "监控删除失败");
    } finally {
      setBusyID(undefined);
    }
  };

  const openDetail = async (monitor: HotKeyAPI.MonitorResponse) => {
    if (monitor.id == null) return;
    setDetail({ history: [], loading: true, monitor, open: true });
    try {
      const [monitorResult, historyResult] = await Promise.all([
        getMonitorsId({ id: monitor.id }),
        getMonitorsIdVersions({ id: monitor.id }),
      ]);
      setDetail({
        history: historyResult.data?.items ?? [],
        loading: false,
        monitor: monitorResult.data,
        open: true,
      });
    } catch (reason) {
      setDetail({
        error: reason instanceof Error ? reason.message : "监控详情加载失败",
        history: [],
        loading: false,
        monitor,
        open: true,
      });
    }
  };

  const addCandidate = async (candidate: {
    ruleType: import("@/lib/monitorDraft").MonitorRuleType;
    value: string;
  }) => {
    const target = candidateTarget;
    if (target?.id == null || target.draft?.version == null) return;
    setSaving(true);
    try {
      await postMonitorsIdDraftAiCandidates(
        { id: target.id },
        {
          expected_monitor_version: target.version ?? 0,
          expected_draft_version: target.draft.version,
          operator: "contains",
          priority: 100,
          rule_type: candidate.ruleType,
          value: candidate.value,
          weight: candidate.ruleType === "exclude_keyword" ? 0 : 60,
        }
      );
      setCandidateTarget(undefined);
      await load();
      toast.success("AI 候选已加入草稿，发布前需审批");
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "AI 候选导入失败");
    } finally {
      setSaving(false);
    }
  };

  const approveCandidate = async (
    rule: HotKeyAPI.MonitorRuleResponse,
    approval: "approved" | "rejected"
  ) => {
    const target = detail.monitor;
    if (target?.id == null || target.draft?.version == null || rule.id == null)
      return;
    setApprovingRuleID(rule.id);
    try {
      await postMonitorsIdDraftRulesRuleIdApproval(
        { id: target.id, rule_id: rule.id },
        {
          approval,
          expected_monitor_version: target.version ?? 0,
          expected_draft_version: target.draft.version,
        }
      );
      const refreshed = await getMonitorsId({ id: target.id });
      if (refreshed.data) await openDetail(refreshed.data);
      await load();
      toast.success(
        approval === "approved" ? "AI 候选已批准" : "AI 候选已拒绝"
      );
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "候选审批失败");
    } finally {
      setApprovingRuleID(undefined);
    }
  };

  return (
    <div className="app-page">
      <PageHeader
        eyebrow="Monitoring"
        title="热点监控"
        description="用正式来源、查询规则和阈值建立可发布的监控。"
        action={
          canEdit ? (
            <Button className="gap-2" onClick={openCreate}>
              <Plus />
              新建监控
            </Button>
          ) : undefined
        }
      />
      {canEdit ? <ManualSearchQuota refreshKey={quotaRefreshKey} /> : null}
      {!canEdit && (
        <Card className="mt-6 flex-row items-center justify-between gap-4 px-5 py-4">
          <div>
            <p className="text-sm font-medium">只读监控目录</p>
            <p className="mt-1 text-xs text-muted-foreground">
              查看者可以检查已发布配置与版本历史，不能修改运行状态。
            </p>
          </div>
          <Badge variant="secondary">viewer</Badge>
        </Card>
      )}
      {loading ? (
        <div className="flex h-72 items-center justify-center">
          <Loader2 className="animate-spin text-muted-foreground" />
        </div>
      ) : loadError ? (
        <Card role="alert" className="mt-6 items-center px-6 py-12 text-center">
          <p className="text-sm font-medium">监控加载失败</p>
          <p className="text-sm text-muted-foreground">{loadError}</p>
          <Button variant="outline" onClick={() => void load()}>
            重试
          </Button>
        </Card>
      ) : !monitors.length ? (
        <Card className="mt-6 gap-0 overflow-hidden py-0">
          <Empty className="h-72">
            <EmptyHeader>
              <EmptyMedia variant="icon">
                <Radar />
              </EmptyMedia>
              <EmptyTitle>还没有热点监控</EmptyTitle>
              <EmptyDescription>
                {canEdit
                  ? sources.length > 0
                    ? "点击“新建监控”配置规则并选择已启用来源。"
                    : "至少需要一个已启用来源才能创建监控。"
                  : "当前没有可查看的已发布监控。"}
              </EmptyDescription>
            </EmptyHeader>
          </Empty>
          <CursorPagination
            hasNext={hasNextCursor(nextCursor)}
            loading={loading}
            onNext={nextPage}
            onPageSizeChange={setPageSize}
            onPrevious={previousPage}
            page={page}
            pageSize={pageSize}
          />
        </Card>
      ) : (
        <Card className="mt-6 gap-0 overflow-hidden py-0">
          <div className="hidden grid-cols-[minmax(0,1.3fr)_110px_100px_minmax(240px,1fr)] gap-4 border-b border-border px-5 py-3 text-xs text-muted-foreground lg:grid">
            <span>监控</span>
            <span>采集间隔</span>
            <span>状态</span>
            <span className="text-right">操作</span>
          </div>
          <div className="divide-y divide-border">
            {monitors.map((monitor) => {
              const config = monitor.draft ?? monitor.published;
              const rule = config?.rules?.[0];
              const busy = busyID === monitor.id;
              const monitorName = monitor.name || `监控 #${monitor.id}`;
              const hasConfigurationActions =
                monitor.status !== MonitorStatus.Archived ||
                (canAdmin && Boolean(monitor.draft));
              const hasRuntimeActions =
                (monitor.status === MonitorStatus.Active &&
                  Boolean(monitor.published)) ||
                (canAdmin &&
                  (monitor.status === MonitorStatus.Active ||
                    monitor.status === MonitorStatus.Paused));
              const hasLifecycleActions = canAdmin;
              return (
                <div
                  key={monitor.id}
                  className="grid gap-3 px-4 py-4 lg:grid-cols-[minmax(0,1.3fr)_110px_100px_minmax(240px,1fr)] lg:items-center lg:gap-4 lg:px-5"
                >
                  <div className="min-w-0">
                    <div className="flex items-center gap-2">
                      <Search className="h-3.5 w-3.5 text-muted-foreground" />
                      <p className="truncate text-sm font-medium">
                        {monitorName}
                      </p>
                    </div>
                    <p className="mono mt-2 truncate text-xs text-muted-foreground">
                      {rule?.value || monitor.description || "暂无规则"}
                    </p>
                  </div>
                  <span className="mono text-xs text-muted-foreground">
                    {config?.collection_interval_seconds
                      ? `${config.collection_interval_seconds}s`
                      : "—"}
                  </span>
                  <Badge variant="outline" className="w-fit">
                    {monitorStatusLabel(monitor.status)}
                  </Badge>
                  <div className="flex items-center justify-start gap-1 lg:justify-end">
                    {monitor.id != null ? (
                      <Button asChild className="gap-1.5" size="sm" variant="ghost">
                        <Link href={`/dashboard/settings/monitors/${monitor.id}/matches`}>
                          <ListChecks />
                          相关性判定
                        </Link>
                      </Button>
                    ) : null}
                    <Button
                      variant="ghost"
                      size="sm"
                      className="gap-1.5"
                      onClick={() => void openDetail(monitor)}
                    >
                      <Eye />
                      查看详情
                    </Button>
                    {canEdit ? (
                      <DropdownMenu modal={false}>
                        <DropdownMenuTrigger asChild>
                          <Button
                            aria-label={`${monitorName} 操作`}
                            className="h-8 w-8 px-0"
                            disabled={busy}
                            size="sm"
                            variant="ghost"
                          >
                            {busy ? (
                              <Loader2 className="animate-spin" />
                            ) : (
                              <MoreHorizontal />
                            )}
                          </Button>
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end" className="w-52">
                          <DropdownMenuLabel>监控操作</DropdownMenuLabel>
                          {monitor.status !== MonitorStatus.Archived ? (
                            <DropdownMenuItem
                              onSelect={() => openEdit(monitor)}
                            >
                              <Pencil />
                              编辑草稿
                            </DropdownMenuItem>
                          ) : null}
                          {monitor.status !== MonitorStatus.Archived &&
                          monitor.draft &&
                          monitor.id != null ? (
                            <DropdownMenuItem asChild>
                              <Link
                                href={`/dashboard/settings/monitors/${monitor.id}/intent`}
                              >
                                <BrainCircuit />
                                编辑语义意图
                              </Link>
                            </DropdownMenuItem>
                          ) : null}
                          {canAdmin && monitor.draft ? (
                            <DropdownMenuItem
                              onSelect={() => setCandidateTarget(monitor)}
                            >
                              <Sparkles />
                              导入 AI 候选
                            </DropdownMenuItem>
                          ) : null}
                          {monitor.draft ? (
                            <DropdownMenuItem
                              disabled={busy}
                              onSelect={() => void previewMonitor(monitor)}
                            >
                              <Eye />
                              {canAdmin ? "预览并发布" : "预览配置"}
                            </DropdownMenuItem>
                          ) : null}

                          {hasConfigurationActions && hasRuntimeActions ? (
                            <DropdownMenuSeparator />
                          ) : null}
                          {monitor.status === MonitorStatus.Active &&
                          monitor.published ? (
                            <DropdownMenuItem
                              disabled={busy}
                              onSelect={() => void collectNow(monitor)}
                            >
                              <Search />
                              立即搜索
                            </DropdownMenuItem>
                          ) : null}
                          {canAdmin &&
                          monitor.status === MonitorStatus.Active ? (
                            <DropdownMenuItem
                              disabled={busy}
                              onSelect={() =>
                                void lifecycle(monitor, "pause")
                              }
                            >
                              <Pause />
                              暂停
                            </DropdownMenuItem>
                          ) : null}
                          {canAdmin &&
                          monitor.status === MonitorStatus.Paused ? (
                            <DropdownMenuItem
                              disabled={busy}
                              onSelect={() =>
                                void lifecycle(monitor, "resume")
                              }
                            >
                              <Play />
                              恢复运行
                            </DropdownMenuItem>
                          ) : null}

                          {hasLifecycleActions &&
                          (hasConfigurationActions || hasRuntimeActions) ? (
                            <DropdownMenuSeparator />
                          ) : null}
                          {canAdmin &&
                          monitor.status !== MonitorStatus.Archived ? (
                            <DropdownMenuItem
                              disabled={busy}
                              onSelect={() =>
                                void lifecycle(monitor, "archive")
                              }
                            >
                              <Archive />
                              归档
                            </DropdownMenuItem>
                          ) : null}
                          {canAdmin &&
                          monitor.status === MonitorStatus.Archived ? (
                            <>
                              <DropdownMenuItem
                                disabled={busy}
                                onSelect={() =>
                                  void lifecycle(monitor, "restore")
                                }
                              >
                                <RotateCcw />
                                恢复
                              </DropdownMenuItem>
                              <DropdownMenuItem
                                className="text-destructive focus:text-destructive"
                                disabled={busy}
                                onSelect={() => setDeleteTarget(monitor)}
                              >
                                <Trash2 />
                                删除
                              </DropdownMenuItem>
                            </>
                          ) : null}
                        </DropdownMenuContent>
                      </DropdownMenu>
                    ) : null}
                  </div>
                </div>
              );
            })}
          </div>
          <CursorPagination
            hasNext={hasNextCursor(nextCursor)}
            loading={loading}
            onNext={nextPage}
            onPageSizeChange={setPageSize}
            onPrevious={previousPage}
            page={page}
            pageSize={pageSize}
          />
        </Card>
      )}

      <MonitorDraftDialog
        busy={saving}
        form={form}
        mode={draftDialog?.mode ?? "create"}
        onFormChange={setForm}
        onOpenChange={(open) => !open && setDraftDialog(undefined)}
        onSubmit={saveDraft}
        open={draftDialog != null}
        sources={sources}
      />
      <MonitorDetailDialog
        approvingRuleID={approvingRuleID}
        canAdmin={canAdmin}
        error={detail.error}
        history={detail.history}
        loading={detail.loading}
        monitor={detail.monitor}
        onCandidateApproval={approveCandidate}
        onOpenChange={(open) => setDetail((current) => ({ ...current, open }))}
        open={detail.open}
      />
      <AICandidateDialog
        busy={saving}
        onOpenChange={(open) => !open && setCandidateTarget(undefined)}
        onSubmit={addCandidate}
        open={candidateTarget != null}
      />
      <Dialog
        open={previewTarget != null}
        onOpenChange={(open) => {
          if (!open) {
            setPreviewTarget(undefined);
            setPreview(undefined);
          }
        }}
      >
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>{canAdmin ? "发布预览" : "配置预览"}</DialogTitle>
            <DialogDescription>
              检查命中资格、预计请求量和配置哈希后再决定是否发布。
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4 py-2">
            <div className="flex items-center justify-between rounded-md border border-border p-4">
              <div>
                <p className="text-sm font-medium">
                  {preview?.eligible ? "配置可以发布" : "配置暂不可发布"}
                </p>
                <p className="mt-1 text-xs text-muted-foreground">
                  {preview?.eligible
                    ? "发布时服务端会再次校验同一草稿。"
                    : "请修正规则或来源后重新预览。"}
                </p>
              </div>
              <Badge variant={preview?.eligible ? "default" : "destructive"}>
                {preview?.eligible ? "通过" : "阻止"}
              </Badge>
            </div>
            <p className="text-sm font-medium">
              预计请求 {preview?.estimated_requests ?? 0} 次
            </p>
            <div className="space-y-2">
              {preview?.sources?.map((source) => {
                const sourceName =
                  sources.find(
                    (candidate) => candidate.id === source.source_connection_id
                  )?.name ?? `来源 #${source.source_connection_id}`;
                return (
                  <div
                    key={source.source_connection_id}
                    className="rounded-md border border-border p-3"
                  >
                    <div className="flex items-center justify-between gap-3">
                      <p className="text-sm font-medium">{sourceName}</p>
                      <Badge variant="secondary">
                        {source.query_mode ?? "local_filter"}
                      </Badge>
                    </div>
                    <p className="mono mt-3 break-all rounded bg-muted/50 px-2 py-1.5 text-xs">
                      {source.compiled_query || "无有效查询"}
                    </p>
                    <div className="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground">
                      <span>
                        语言 {(source.languages ?? []).join(" / ") || "全部"}
                      </span>
                      <span>上限 {source.max_query_bytes ?? 2048} bytes</span>
                      <span>
                        包含 {source.included_term_count ?? source.included_rule_ids?.length ?? 0}
                      </span>
                      <span>
                        排除 {source.excluded_term_count ?? source.excluded_rule_ids?.length ?? 0}
                      </span>
                    </div>
                    <p className="mono mt-2 break-all text-[11px] text-muted-foreground">
                      signature {source.query_signature || "—"}
                    </p>
                  </div>
                );
              })}
            </div>
            <p className="mono break-all text-xs text-muted-foreground">
              配置哈希 {preview?.config_hash || "—"}
            </p>
            {!!preview?.warnings?.length && (
              <div
                role="alert"
                className="rounded-md border border-amber-500/30 bg-amber-500/5 p-3 text-sm"
              >
                {preview.warnings.join("；")}
              </div>
            )}
          </div>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setPreviewTarget(undefined)}
            >
              关闭
            </Button>
            {canAdmin && (
              <Button
                disabled={!preview?.eligible || busyID === previewTarget?.id}
                onClick={() => void publish()}
              >
                {busyID === previewTarget?.id && (
                  <Loader2 className="animate-spin" />
                )}
                确认发布
              </Button>
            )}
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <ConfirmDeleteDialog
        open={deleteTarget != null}
        onOpenChange={(open) => !open && setDeleteTarget(undefined)}
        title="删除监控"
        description="监控将从工作区隐藏，已采集内容、历史事件、报告和审计记录仍会保留。"
        resourceName={deleteTarget?.name || `监控 #${deleteTarget?.id ?? ""}`}
        onConfirm={deleteMonitor}
        loading={busyID === deleteTarget?.id}
      />
    </div>
  );
}
