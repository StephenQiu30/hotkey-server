"use client";

import {
  type FormEvent,
  useCallback,
  useEffect,
  useMemo,
  useState,
} from "react";
import {
  Archive,
  Loader2,
  Pause,
  Play,
  Plus,
  Radar,
  RotateCcw,
  Rocket,
  Search,
  Trash2,
} from "lucide-react";
import { toast } from "sonner";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  deleteMonitorsId,
  getMonitors,
  postMonitors,
  postMonitorsIdArchive,
  postMonitorsIdPause,
  postMonitorsIdPublish,
  postMonitorsIdResume,
  postMonitorsIdRestore,
} from "@/services/hotkey/hotkey-server/monitors";
import { getSourceConnections } from "@/services/hotkey/hotkey-server/sources";
import { PageHeader } from "@/components/dashboard/PageHeader";
import { ConfirmDeleteDialog } from "@/components/dashboard/ConfirmDeleteDialog";
import {
  CursorPagination,
  DEFAULT_PAGE_SIZE,
  hasNextCursor,
} from "@/components/dashboard/CursorPagination";
import { MonitorAction, MonitorRegion, MonitorStatus } from "@/lib/domainEnums";
import { monitorStatusLabel } from "@/lib/domainPresentation";
import {
  MAX_MONITOR_SOURCES,
  MONITOR_LIMITS,
  buildMonitorDraftRequest,
  selectAllMonitorSources,
  toggleMonitorSource,
  validateMonitorDraft,
  type MonitorDraftForm,
} from "@/lib/monitorDraft";

const createInitialForm = (): MonitorDraftForm => ({
  name: "",
  description: "",
  query: "",
  language: "zh",
  region: MonitorRegion.China,
  interval: 900,
  relevance: 60,
  event: 70,
  retention: 30,
  sourceIds: [],
});

export default function MonitorsPage() {
  const [pageSize, setPageSize] = useState(DEFAULT_PAGE_SIZE);
  const [monitors, setMonitors] = useState<HotKeyAPI.MonitorResponse[]>([]);
  const [sources, setSources] = useState<HotKeyAPI.SourceReadResponse[]>([]);
  const [loading, setLoading] = useState(true);
  const [dialog, setDialog] = useState(false);
  const [action, setAction] = useState<number>();
  const [creating, setCreating] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<HotKeyAPI.MonitorResponse>();
  const [form, setForm] = useState<MonitorDraftForm>(createInitialForm);
  const [page, setPage] = useState(1);
  const [cursors, setCursors] = useState<(string | undefined)[]>([undefined]);
  const [nextCursor, setNextCursor] = useState<string>();

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const [monitorResult, sourceResult] = await Promise.all([
        getMonitors({ limit: pageSize }),
        getSourceConnections({ limit: 100 }),
      ]);
      setMonitors(monitorResult.data?.items ?? []);
      setPage(1);
      setCursors([undefined]);
      setNextCursor(monitorResult.data?.next_cursor);
      const enabledSources = (sourceResult.data?.items ?? []).filter(
        (source) => source.enabled && !source.deleted
      );
      setSources(enabledSources);
      setForm((current) => ({
        ...current,
        sourceIds: current.sourceIds.length
          ? current.sourceIds
          : selectAllMonitorSources(
              enabledSources.flatMap((source) =>
                source.id == null ? [] : [source.id]
              )
            ),
      }));
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "监控加载失败");
    } finally {
      setLoading(false);
    }
  }, [pageSize]);

  const loadMonitorPage = async (
    cursor: string | undefined,
    pageNumber: number,
  ) => {
    setLoading(true);
    try {
      const result = await getMonitors({
        limit: pageSize,
        ...(cursor ? { cursor } : {}),
      });
      setMonitors(result.data?.items ?? []);
      setPage(pageNumber);
      setNextCursor(result.data?.next_cursor);
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "监控加载失败");
    } finally {
      setLoading(false);
    }
  };

  const nextPage = () => {
    if (!nextCursor) return;
    const nextPageNumber = page + 1;
    const cursor = nextCursor;
    setCursors((history) => [...history.slice(0, page), cursor]);
    void loadMonitorPage(cursor, nextPageNumber);
  };

  const previousPage = () => {
    if (page <= 1) return;
    void loadMonitorPage(cursors[page - 2], page - 1);
  };

  const changePageSize = (nextPageSize: number) => {
    setPageSize(nextPageSize);
  };
  useEffect(() => {
    load();
  }, [load]);

  const create = async (event?: FormEvent) => {
    event?.preventDefault();
    const normalizedForm = {
      ...form,
      name:
        form.name.trim() ||
        `热点监控 · ${form.query.trim().slice(0, 48)}`,
    };
    const validationMessage = validateMonitorDraft(normalizedForm);
    if (validationMessage) {
      toast.error(validationMessage);
      return;
    }
    setCreating(true);
    try {
      await postMonitors(buildMonitorDraftRequest(normalizedForm));
      setDialog(false);
      setForm(createInitialForm());
      await load();
      toast.success("监控草稿已创建");
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "监控创建失败");
    } finally {
      setCreating(false);
    }
  };

  const operate = async (
    monitor: HotKeyAPI.MonitorResponse,
    kind: MonitorAction
  ) => {
    if (monitor.id == null) return;
    setAction(monitor.id);
    try {
      if (kind === MonitorAction.Publish)
        await postMonitorsIdPublish(
          { id: monitor.id },
          {
            expected_draft_version: monitor.draft?.version ?? 0,
            expected_monitor_version: monitor.version ?? 0,
          }
        );
      else if (kind === MonitorAction.Pause)
        await postMonitorsIdPause({ id: monitor.id }, {
          expected_monitor_version: monitor.version ?? 0,
        } as unknown as HotKeyAPI.LifecycleRequest);
      else if (kind === MonitorAction.Resume)
        await postMonitorsIdResume({ id: monitor.id }, {
          expected_monitor_version: monitor.version ?? 0,
        } as unknown as HotKeyAPI.LifecycleRequest);
      else if (kind === MonitorAction.Restore)
        await postMonitorsIdRestore({ id: monitor.id }, {
          expected_monitor_version: monitor.version ?? 0,
        } as unknown as HotKeyAPI.LifecycleRequest);
      else
        await postMonitorsIdArchive({ id: monitor.id }, {
          expected_monitor_version: monitor.version ?? 0,
        } as unknown as HotKeyAPI.LifecycleRequest);
      await load();
      toast.success("监控状态已更新");
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "监控操作失败");
    } finally {
      setAction(undefined);
    }
  };

  const deleteMonitor = async () => {
    if (deleteTarget?.id == null) return;
    setAction(deleteTarget.id);
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
      setAction(undefined);
    }
  };

  const availableSourceIds = useMemo(
    () => sources.flatMap((source) => (source.id == null ? [] : [source.id])),
    [sources]
  );
  const selectAllIds = useMemo(
    () => selectAllMonitorSources(availableSourceIds),
    [availableSourceIds]
  );
  const allSelected =
    selectAllIds.length > 0 &&
    selectAllIds.every((id) => form.sourceIds.includes(id)) &&
    form.sourceIds.length === selectAllIds.length;
  const selectionState = allSelected
    ? true
    : form.sourceIds.length > 0
    ? "indeterminate"
    : false;

  const toggleSource = (id: number, checked: boolean) =>
    setForm((current) => ({
      ...current,
      sourceIds: toggleMonitorSource(current.sourceIds, id, checked),
    }));

  return (
    <div className="app-page">
      <PageHeader
        eyebrow="Monitoring"
        title="热点监控"
        description="用正式来源、查询规则和阈值建立可发布的监控。"
        action={
          <Dialog
            open={dialog}
            onOpenChange={(open) => {
              setDialog(open);
              if (open) {
                setForm((current) => ({
                  ...current,
                  sourceIds: current.sourceIds.length
                    ? current.sourceIds
                    : selectAllIds,
                }));
              }
            }}
          >
            <DialogTrigger asChild>
              <Button className="gap-2">
                <Plus />
                新建监控
              </Button>
            </DialogTrigger>
            <DialogContent className="max-h-[90vh] overflow-hidden p-0 sm:max-w-xl">
              <DialogHeader className="border-b border-border px-6 py-5">
                <DialogTitle>新建监控草稿</DialogTitle>
              </DialogHeader>
              <form onSubmit={create}>
                <div className="grid max-h-[calc(90vh-9rem)] gap-4 overflow-y-auto px-6 py-5 sm:grid-cols-2">
                  <div className="sm:col-span-2">
                    <Label htmlFor="monitor-name">监控名称（可选）</Label>
                    <Input
                      id="monitor-name"
                      className="mt-2"
                      value={form.name}
                      onChange={(event) =>
                        setForm({ ...form, name: event.target.value })
                      }
                      placeholder="留空将根据关键词自动生成"
                    />
                  </div>
                  <div className="sm:col-span-2">
                    <Label htmlFor="monitor-query">关键词规则</Label>
                    <Input
                      id="monitor-query"
                      className="mt-2"
                      value={form.query}
                      onChange={(event) =>
                        setForm({ ...form, query: event.target.value })
                      }
                      placeholder="AI Agent OR 智能体 OR agentic"
                    />
                  </div>
                  <div className="sm:col-span-2">
                    <Label htmlFor="monitor-description">说明</Label>
                    <Input
                      id="monitor-description"
                      className="mt-2"
                      value={form.description}
                      onChange={(event) =>
                        setForm({ ...form, description: event.target.value })
                      }
                      placeholder="追踪创作工作流中的智能体产品与案例"
                    />
                  </div>
                  <div>
                    <Label>语言</Label>
                    <Select
                      value={form.language}
                      onValueChange={(value) =>
                        setForm({ ...form, language: value })
                      }
                    >
                      <SelectTrigger className="mt-2">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="zh">中文</SelectItem>
                        <SelectItem value="en">English</SelectItem>
                      </SelectContent>
                    </Select>
                  </div>
                  <div>
                    <Label>地区</Label>
                    <Select
                      value={form.region}
                      onValueChange={(value) =>
                        setForm({ ...form, region: value as MonitorRegion })
                      }
                    >
                      <SelectTrigger className="mt-2">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="CN">中国</SelectItem>
                        <SelectItem value="US">美国</SelectItem>
                        <SelectItem value={MonitorRegion.Global}>
                          全球
                        </SelectItem>
                      </SelectContent>
                    </Select>
                  </div>
                  {(
                    [
                      [
                        "采集间隔（秒）",
                        "interval",
                        MONITOR_LIMITS.interval.min,
                        MONITOR_LIMITS.interval.max,
                        MONITOR_LIMITS.interval.step,
                      ],
                      [
                        "相关性阈值",
                        "relevance",
                        MONITOR_LIMITS.relevance.min,
                        MONITOR_LIMITS.relevance.max,
                        1,
                      ],
                      [
                        "事件阈值",
                        "event",
                        MONITOR_LIMITS.event.min,
                        MONITOR_LIMITS.event.max,
                        1,
                      ],
                      [
                        "保留天数",
                        "retention",
                        MONITOR_LIMITS.retention.min,
                        MONITOR_LIMITS.retention.max,
                        1,
                      ],
                    ] as const
                  ).map(([label, key, min, max, step]) => (
                    <div key={String(key)}>
                      <Label htmlFor={`monitor-${key}`}>{label}</Label>
                      <Input
                        id={`monitor-${key}`}
                        type="number"
                        min={Number(min)}
                        max={Number(max)}
                        step={Number(step)}
                        className="mono mt-2"
                        value={form[key as "interval"]}
                        onChange={(event) =>
                          setForm({
                            ...form,
                            [key]: Number(event.target.value),
                          })
                        }
                      />
                    </div>
                  ))}
                  <div className="sm:col-span-2">
                    <div className="flex items-end justify-between gap-3">
                      <div>
                        <Label>数据来源</Label>
                        <p className="mt-1 text-xs text-muted-foreground">
                          系统已自动选择已启用来源，最多使用 {MAX_MONITOR_SOURCES}{" "}
                          个；如有需要可以手动调整。
                        </p>
                      </div>
                      <span className="mono shrink-0 text-xs text-muted-foreground">
                        已选 {form.sourceIds.length}/{MAX_MONITOR_SOURCES}
                      </span>
                    </div>
                    <div className="mt-2 overflow-hidden rounded-md border border-border">
                      {sources.length > 0 && (
                        <div className="flex items-center gap-3 border-b border-border bg-muted/30 px-3 py-2.5 text-sm">
                          <label className="flex cursor-pointer items-center gap-3 font-medium">
                            <Checkbox
                              aria-label="全选数据来源"
                              checked={selectionState}
                              onCheckedChange={(checked) =>
                                setForm((current) => ({
                                  ...current,
                                  sourceIds:
                                    checked === true ? selectAllIds : [],
                                }))
                              }
                            />
                            <span>全选</span>
                          </label>
                          {availableSourceIds.length > MAX_MONITOR_SOURCES && (
                            <span className="text-xs text-muted-foreground">
                              按列表顺序选择前 {MAX_MONITOR_SOURCES} 个
                            </span>
                          )}
                          <Button
                            type="button"
                            variant="ghost"
                            size="sm"
                            className="ml-auto h-7 px-2 text-xs"
                            disabled={!form.sourceIds.length}
                            onClick={() =>
                              setForm((current) => ({
                                ...current,
                                sourceIds: [],
                              }))
                            }
                          >
                            清空
                          </Button>
                        </div>
                      )}
                      <div className="max-h-64 divide-y divide-border overflow-y-auto">
                        {sources.length ? (
                          sources.map((source) => (
                            <label
                              key={source.id}
                              className="flex cursor-pointer items-center gap-3 px-3 py-3 text-sm has-[[data-disabled]]:cursor-not-allowed has-[[data-disabled]]:opacity-50"
                            >
                              <Checkbox
                                checked={
                                  source.id != null &&
                                  form.sourceIds.includes(source.id)
                                }
                                onCheckedChange={(checked) =>
                                  source.id != null &&
                                  toggleSource(source.id, checked === true)
                                }
                                disabled={
                                  source.id != null &&
                                  !form.sourceIds.includes(source.id) &&
                                  form.sourceIds.length >= MAX_MONITOR_SOURCES
                                }
                              />
                              <span>{source.name}</span>
                              <span className="ml-auto mono text-xs text-muted-foreground">
                                {source.source_type}
                              </span>
                            </label>
                          ))
                        ) : (
                          <p className="p-4 text-xs text-muted-foreground">
                            请先在来源管理中创建并启用来源。
                          </p>
                        )}
                      </div>
                    </div>
                  </div>
                </div>
                <DialogFooter className="border-t border-border px-6 py-4">
                  <Button
                    type="button"
                    variant="outline"
                    onClick={() => setDialog(false)}
                  >
                    取消
                  </Button>
                  <Button
                    type="submit"
                    disabled={
                      creating ||
                      !form.query.trim() ||
                      !form.sourceIds.length
                    }
                  >
                    {creating && <Loader2 className="animate-spin" />}
                    {creating ? "创建中" : "创建草稿"}
                  </Button>
                </DialogFooter>
              </form>
            </DialogContent>
          </Dialog>
        }
      />
      {loading ? (
        <div className="flex h-72 items-center justify-center">
          <Loader2 className="animate-spin text-muted-foreground" />
        </div>
      ) : !monitors.length ? (
        <div className="panel mt-6">
          <div className="flex h-72 flex-col items-center justify-center text-center">
            <Radar className="mb-3 h-6 w-6 text-muted-foreground" />
            <p className="text-sm font-medium">还没有热点监控</p>
            <p className="mt-1 text-xs text-muted-foreground">
              至少需要一个已启用来源才能创建监控。
            </p>
          </div>
          <CursorPagination
            hasNext={hasNextCursor(nextCursor)}
            loading={loading}
            onNext={nextPage}
            onPageSizeChange={changePageSize}
            onPrevious={previousPage}
            page={page}
            pageSize={pageSize}
          />
        </div>
      ) : (
        <div className="panel mt-6 overflow-hidden">
          <div className="hidden grid-cols-[minmax(0,1.3fr)_110px_100px_260px] gap-4 border-b border-border px-5 py-3 text-xs text-muted-foreground lg:grid">
            <span>监控</span>
            <span>采集间隔</span>
            <span>状态</span>
            <span className="text-right">操作</span>
          </div>
          <div className="divide-y divide-border">
            {monitors.map((monitor) => {
              const config = monitor.published ?? monitor.draft;
              const rule = config?.rules?.[0];
              return (
                <div
                  key={monitor.id}
                  className="grid gap-3 px-4 py-4 lg:grid-cols-[minmax(0,1.3fr)_110px_100px_260px] lg:items-center lg:gap-4 lg:px-5"
                >
                  <div className="min-w-0">
                    <div className="flex items-center gap-2">
                      <Search className="h-3.5 w-3.5 text-muted-foreground" />
                      <p className="truncate text-sm font-medium">
                        {monitor.name || `监控 #${monitor.id}`}
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
                  <div className="flex flex-wrap justify-start gap-2 lg:justify-end">
                    {monitor.status === MonitorStatus.Draft && (
                      <Button
                        size="sm"
                        onClick={() => operate(monitor, MonitorAction.Publish)}
                        disabled={action === monitor.id}
                        className="gap-1.5"
                      >
                        <Rocket />
                        发布
                      </Button>
                    )}
                    {monitor.status === MonitorStatus.Active && (
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => operate(monitor, MonitorAction.Pause)}
                        disabled={action === monitor.id}
                        className="gap-1.5"
                      >
                        <Pause />
                        暂停
                      </Button>
                    )}
                    {monitor.status === MonitorStatus.Paused && (
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => operate(monitor, MonitorAction.Resume)}
                        disabled={action === monitor.id}
                        className="gap-1.5"
                      >
                        <Play />
                        恢复
                      </Button>
                    )}
                    {monitor.status !== MonitorStatus.Archived && (
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => operate(monitor, MonitorAction.Archive)}
                        disabled={action === monitor.id}
                        className="gap-1.5 text-muted-foreground"
                      >
                        <Archive />
                        归档
                      </Button>
                    )}
                    {monitor.status === MonitorStatus.Archived && (
                      <>
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={() => operate(monitor, MonitorAction.Restore)}
                          disabled={action === monitor.id}
                          className="gap-1.5"
                        >
                          <RotateCcw />
                          恢复
                        </Button>
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => setDeleteTarget(monitor)}
                          disabled={action === monitor.id}
                          className="gap-1.5 text-destructive hover:text-destructive"
                        >
                          <Trash2 />
                          删除
                        </Button>
                      </>
                    )}
                  </div>
                </div>
              );
            })}
          </div>
          <CursorPagination
            hasNext={hasNextCursor(nextCursor)}
            loading={loading}
            onNext={nextPage}
            onPrevious={previousPage}
            page={page}
          />
        </div>
      )}
      <ConfirmDeleteDialog
        open={deleteTarget != null}
        onOpenChange={(open) => !open && setDeleteTarget(undefined)}
        title="删除监控"
        description="监控将从工作区隐藏，已采集内容、历史事件、报告和审计记录仍会保留。"
        resourceName={deleteTarget?.name || `监控 #${deleteTarget?.id ?? ""}`}
        onConfirm={deleteMonitor}
        loading={action === deleteTarget?.id}
      />
    </div>
  );
}
