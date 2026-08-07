"use client";

import { Suspense, useCallback, useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import {
  ArrowUpRight,
  CalendarRange,
  CircleDot,
  ExternalLink,
  Filter,
  Loader2,
  RefreshCw,
  SearchX,
  Target,
  TrendingDown,
  TrendingUp,
} from "lucide-react";
import { EventIntelligencePanel } from "@/components/dashboard/EventIntelligencePanel";
import { EventGovernancePanel } from "@/components/dashboard/EventGovernancePanel";
import { EventHeatPanel } from "@/components/dashboard/EventHeatPanel";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  getEventsIdContents,
  getEventsIdHeat,
  getEventsIdIntelligence,
  getEventsIdUpdates,
  postEventsIdContentsContentIdLock,
  postEventsIdLifecycle,
  postEventsIdMerge,
  postEventsIdSplit,
} from "@/services/hotkey/hotkey-server/events";
import { getMonitors } from "@/services/hotkey/hotkey-server/monitors";
import { getRadarEvents } from "@/services/hotkey/hotkey-server/radar";
import {
  confirmationLabel,
  formatRadarScore,
  formatRadarTime,
  getRadarEventTitle,
  reasonLabel,
  trendLabel,
  trendTone,
  updateKindLabel,
} from "@/lib/radarPresentation";
import { cn } from "@/lib/utils";
import { useAuthStore } from "@/stores/authStore";

type RadarWindow = NonNullable<HotKeyAPI.getRadarEventsParams["window"]>;
type RadarSort = NonNullable<HotKeyAPI.getRadarEventsParams["sort"]>;

function SignalIcon({ trend }: { trend?: string }) {
  if (trend === "rising" || trend === "emerging") {
    return <TrendingUp className="h-4 w-4" />;
  }
  if (trend === "falling" || trend === "dormant") {
    return <TrendingDown className="h-4 w-4" />;
  }
  return <CircleDot className="h-4 w-4" />;
}

function EventsWorkspace() {
  const searchParams = useSearchParams();
  const role = useAuthStore((state) => state.user?.role);
  const query = searchParams.get("q")?.trim().toLocaleLowerCase("zh-CN") || "";
  const requestedEventId = Number(searchParams.get("event")) || undefined;
  const [windowValue, setWindowValue] = useState<RadarWindow>("24h");
  const [sort, setSort] = useState<RadarSort>("momentum");
  const [events, setEvents] = useState<HotKeyAPI.RadarEventResponse[]>([]);
  const [monitors, setMonitors] = useState<HotKeyAPI.MonitorResponse[]>([]);
  const [monitorId, setMonitorId] = useState<number>();
  const [selectedId, setSelectedId] = useState<number>();
  const [detailRevision, setDetailRevision] = useState(0);
  const [updates, setUpdates] = useState<HotKeyAPI.EventUpdateResponse[]>([]);
  const [intelligence, setIntelligence] =
    useState<HotKeyAPI.EventIntelligenceResponse>();
  const [heat, setHeat] = useState<HotKeyAPI.HeatResponse>();
  const [members, setMembers] = useState<HotKeyAPI.EventMemberResponse[]>([]);
  const [asOf, setAsOf] = useState<string>();
  const [loading, setLoading] = useState(true);
  const [detailLoading, setDetailLoading] = useState(false);
  const [updatesError, setUpdatesError] = useState(false);
  const [intelligenceLoading, setIntelligenceLoading] = useState(false);
  const [intelligenceError, setIntelligenceError] = useState(false);
  const [heatLoading, setHeatLoading] = useState(false);
  const [heatError, setHeatError] = useState(false);
  const [membersLoading, setMembersLoading] = useState(false);
  const [membersError, setMembersError] = useState(false);
  const [governanceBusy, setGovernanceBusy] = useState(false);
  const [governanceError, setGovernanceError] = useState<string>();
  const [error, setError] = useState<string>();

  const loadRadar = useCallback(async () => {
    setLoading(true);
    setError(undefined);
    try {
      const result = await getRadarEvents({
        window: windowValue,
        sort,
        limit: 50,
        ...(monitorId != null ? { monitor_id: monitorId } : {}),
      });
      const items = result.data?.items ?? [];
      setEvents(items);
      setAsOf(result.data?.as_of);
      setSelectedId((current) => {
        const preferred = requestedEventId ?? current;
        return items.some((item) => item.event_id === preferred)
          ? preferred
          : items[0]?.event_id;
      });
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "事件雷达加载失败");
      setEvents([]);
    } finally {
      setLoading(false);
    }
  }, [monitorId, requestedEventId, sort, windowValue]);

  useEffect(() => {
    void loadRadar();
  }, [loadRadar]);

  useEffect(() => {
    let active = true;
    getMonitors({ limit: 100 })
      .then((result) => {
        if (!active) return;
        setMonitors(
          (result.data?.items ?? []).filter(
            (monitor) =>
              monitor.id != null &&
              (monitor.status === "active" || monitor.status === "paused")
          )
        );
      })
      .catch(() => {
        if (active) setMonitors([]);
      });
    return () => {
      active = false;
    };
  }, []);

  useEffect(() => {
    if (selectedId == null) {
      setUpdates([]);
      setUpdatesError(false);
      setIntelligence(undefined);
      setHeat(undefined);
      setMembers([]);
      return;
    }
    let active = true;
    setDetailLoading(true);
    setUpdatesError(false);
    setIntelligenceLoading(true);
    setIntelligenceError(false);
    setIntelligence(undefined);
    setHeatLoading(true);
    setHeatError(false);
    setHeat(undefined);
    setMembersLoading(true);
    setMembersError(false);
    setMembers([]);
    setGovernanceError(undefined);

    Promise.allSettled([
      getEventsIdUpdates({ id: selectedId, limit: 20 }),
      getEventsIdIntelligence({ id: selectedId }),
      getEventsIdHeat({ id: selectedId }),
      getEventsIdContents({ id: selectedId }),
    ]).then(
      ([updatesResult, intelligenceResult, heatResult, membersResult]) => {
        if (!active) return;
        setUpdates(
          updatesResult.status === "fulfilled"
            ? updatesResult.value.data?.items ?? []
            : []
        );
        setUpdatesError(updatesResult.status === "rejected");
        if (intelligenceResult.status === "fulfilled") {
          setIntelligence(intelligenceResult.value.data);
        } else {
          setIntelligence(undefined);
          setIntelligenceError(true);
        }
        if (heatResult.status === "fulfilled") {
          setHeat(heatResult.value.data);
        } else {
          setHeatError(true);
        }
        if (membersResult.status === "fulfilled") {
          setMembers(membersResult.value.data?.items ?? []);
        } else {
          setMembersError(true);
        }
        setDetailLoading(false);
        setIntelligenceLoading(false);
        setHeatLoading(false);
        setMembersLoading(false);
      }
    );
    return () => {
      active = false;
    };
  }, [detailRevision, selectedId]);

  const visibleEvents = useMemo(() => {
    if (!query) return events;
    return events.filter((event) =>
      [getRadarEventTitle(event), event.summary, event.latest_update?.summary]
        .filter(Boolean)
        .join(" ")
        .toLocaleLowerCase("zh-CN")
        .includes(query)
    );
  }, [events, query]);

  const selected = events.find((event) => event.event_id === selectedId);

  const toggleMemberLock = useCallback(
    async (member: HotKeyAPI.EventMemberResponse) => {
      if (!selected?.event_id || !member.content_id || !member.version) return;
      setGovernanceBusy(true);
      setGovernanceError(undefined);
      try {
        const result = await postEventsIdContentsContentIdLock(
          { id: selected.event_id, content_id: member.content_id },
          {
            expected_version: member.version,
            locked: !member.manual_locked,
            reason: "manual_member_lock",
          }
        );
        if (result.data) {
          setMembers((current) =>
            current.map((item) =>
              item.content_id === member.content_id ? result.data! : item
            )
          );
        }
        setDetailRevision((current) => current + 1);
      } catch {
        setGovernanceError("成员锁定失败，数据可能已更新，请刷新后重试。");
      } finally {
        setGovernanceBusy(false);
      }
    },
    [selected?.event_id]
  );

  const transitionLifecycle = useCallback(
    async (status: string) => {
      if (!selected?.event_id || !selected.version) return;
      setGovernanceBusy(true);
      setGovernanceError(undefined);
      try {
        const result = await postEventsIdLifecycle(
          { id: selected.event_id },
          {
            expected_version: selected.version,
            reason: "manual_lifecycle_update",
            to: status,
          }
        );
        setEvents((current) =>
          current.map((item) =>
            item.event_id === selected.event_id
              ? {
                  ...item,
                  lifecycle_status: result.data?.lifecycle_status ?? status,
                  version: result.data?.version ?? item.version,
                }
              : item
          )
        );
        setDetailRevision((current) => current + 1);
      } catch {
        setGovernanceError("生命周期变更失败，请刷新后重试。");
      } finally {
        setGovernanceBusy(false);
      }
    },
    [selected]
  );

  const mergeEvent = useCallback(
    async (target: HotKeyAPI.RadarEventResponse) => {
      if (
        !selected?.event_id ||
        !selected.version ||
        !target.event_id ||
        !target.version
      ) {
        return;
      }
      setGovernanceBusy(true);
      setGovernanceError(undefined);
      try {
        await postEventsIdMerge(
          { id: selected.event_id },
          {
            reason: "manual_event_merge",
            source_expected_version: selected.version,
            target_event_id: target.event_id,
            target_expected_version: target.version,
          }
        );
        await loadRadar();
        setDetailRevision((current) => current + 1);
      } catch {
        setGovernanceError("事件合并失败，数据可能已更新，请刷新后重试。");
      } finally {
        setGovernanceBusy(false);
      }
    },
    [loadRadar, selected]
  );

  const splitEvent = useCallback(
    async (selectedMembers: HotKeyAPI.EventMemberResponse[]) => {
      if (!selected?.event_id || !selected.version) return;
      setGovernanceBusy(true);
      setGovernanceError(undefined);
      try {
        await postEventsIdSplit(
          { id: selected.event_id },
          {
            reason: "manual_event_split",
            source_expected_version: selected.version,
            members: selectedMembers.map((member) => ({
              content_id: member.content_id!,
              expected_version: member.version!,
            })),
          }
        );
        await loadRadar();
        setDetailRevision((current) => current + 1);
      } catch {
        setGovernanceError("事件拆分失败，数据可能已更新，请刷新后重试。");
      } finally {
        setGovernanceBusy(false);
      }
    },
    [loadRadar, selected]
  );

  return (
    <div className="app-page radar-page">
      <header className="flex flex-col gap-6 border-b pb-8 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <h1 className="text-3xl font-semibold tracking-[-0.045em] text-foreground">
            事件动态
          </h1>
          <p className="mt-2 text-sm leading-6 text-muted-foreground">
            AI 按变化速度、来源覆盖与证据状态筛选值得关注的热点事件。
          </p>
        </div>
        <div className="flex items-center gap-3">
          {asOf ? (
            <span className="hidden text-xs text-muted-foreground sm:inline">
              数据更新于 {formatRadarTime(asOf)}
            </span>
          ) : null}
          <Button
            variant="outline"
            size="sm"
            onClick={loadRadar}
            className="gap-2"
          >
            <RefreshCw className="h-4 w-4" />
            刷新
          </Button>
        </div>
      </header>

      <div className="mt-5 flex flex-wrap items-center gap-3">
        <Select
          value={monitorId?.toString() ?? "all"}
          onValueChange={(value) => {
            if (value === "all") {
              setMonitorId(undefined);
              if (sort === "relevance") setSort("momentum");
              return;
            }
            setMonitorId(Number(value));
          }}
        >
          <SelectTrigger aria-label="监控上下文" className="w-[180px]">
            <Target className="h-4 w-4 text-muted-foreground" />
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">全部事件</SelectItem>
            {monitors.map((monitor) => (
              <SelectItem key={monitor.id} value={String(monitor.id)}>
                {monitor.name || `监控 #${monitor.id}`}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Select
          value={windowValue}
          onValueChange={(value) => setWindowValue(value as RadarWindow)}
        >
          <SelectTrigger aria-label="时间窗口" className="w-[160px]">
            <CalendarRange className="h-4 w-4 text-muted-foreground" />
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="1h">过去 1 小时</SelectItem>
            <SelectItem value="6h">过去 6 小时</SelectItem>
            <SelectItem value="24h">过去 24 小时</SelectItem>
            <SelectItem value="7d">过去 7 天</SelectItem>
          </SelectContent>
        </Select>
        <Select
          value={sort}
          onValueChange={(value) => setSort(value as RadarSort)}
        >
          <SelectTrigger aria-label="排序方式" className="w-[160px]">
            <Filter className="h-4 w-4 text-muted-foreground" />
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="momentum">变化速度</SelectItem>
            <SelectItem value="attention">关注度</SelectItem>
            <SelectItem value="breadth">来源覆盖</SelectItem>
            <SelectItem value="latest">最新变化</SelectItem>
            <SelectItem value="relevance" disabled={monitorId == null}>
              监控相关性
            </SelectItem>
          </SelectContent>
        </Select>
        {query ? (
          <Badge variant="secondary" className="h-9 px-3 font-normal">
            搜索：{searchParams.get("q")}
          </Badge>
        ) : null}
      </div>

      {error ? (
        <Alert variant="destructive" className="mt-6">
          <CircleDot className="h-4 w-4" />
          <AlertTitle>事件雷达加载失败</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      ) : loading ? (
        <div className="flex h-96 items-center justify-center">
          <Loader2 className="h-5 w-5 animate-spin text-primary" />
        </div>
      ) : visibleEvents.length === 0 ? (
        <Card className="mt-6 border-dashed">
          <Empty className="h-80 border-0">
            <EmptyHeader>
              <EmptyMedia variant="icon">
                <SearchX />
              </EmptyMedia>
              <EmptyTitle className="text-sm">
                没有符合当前条件的事件
              </EmptyTitle>
              <EmptyDescription>
                调整时间窗口或清除搜索后重试。
              </EmptyDescription>
            </EmptyHeader>
          </Empty>
        </Card>
      ) : (
        <div className="mt-6 grid min-h-[620px] gap-6 xl:grid-cols-[minmax(0,1fr)_minmax(400px,460px)]">
          <section className="min-w-0">
            <div className="mb-3 flex items-center gap-2 text-sm font-semibold text-foreground">
              <span className="h-2 w-2 rounded-full bg-destructive" />
              需要关注
              <span className="font-normal text-muted-foreground">
                {visibleEvents.length}
              </span>
            </div>
            <Card className="overflow-hidden shadow-none">
              <Table aria-label="热点事件列表">
                <TableHeader>
                  <TableRow>
                    <TableHead className="min-w-[320px]">事件</TableHead>
                    <TableHead className="hidden sm:table-cell">
                      来源广度
                    </TableHead>
                    <TableHead className="hidden sm:table-cell">
                      首次发现
                    </TableHead>
                    <TableHead className="hidden sm:table-cell">趋势</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {visibleEvents.map((event, index) => {
                    const active = event.event_id === selectedId;
                    const tone = trendTone(event.trend_status);
                    return (
                      <TableRow
                        key={event.event_id ?? index}
                        data-state={active ? "selected" : undefined}
                      >
                        <TableCell className="py-3">
                          <Button
                            type="button"
                            variant="ghost"
                            onClick={() => setSelectedId(event.event_id)}
                            className="h-auto w-full justify-start gap-3 whitespace-normal px-0 py-0 text-left hover:bg-transparent"
                          >
                            <span
                              className={cn(
                                "shrink-0",
                                tone === "danger" && "text-destructive",
                                tone === "success" && "text-emerald-600",
                                tone === "muted" && "text-muted-foreground"
                              )}
                            >
                              <SignalIcon trend={event.trend_status} />
                            </span>
                            <span className="min-w-0">
                              <span className="block font-medium leading-6">
                                {getRadarEventTitle(event)}
                              </span>
                              <span className="mt-0.5 block line-clamp-1 text-xs font-normal text-muted-foreground">
                                {event.summary ||
                                  "正在聚合事件背景与最新进展。"}
                              </span>
                            </span>
                          </Button>
                        </TableCell>
                        <TableCell className="hidden sm:table-cell">
                          {event.independent_source_count ?? 0} 个
                        </TableCell>
                        <TableCell className="hidden sm:table-cell">
                          {formatRadarTime(event.first_seen_at)}
                        </TableCell>
                        <TableCell className="hidden sm:table-cell">
                          <Badge
                            variant="outline"
                            className="gap-1.5 font-normal"
                          >
                            <SignalIcon trend={event.trend_status} />
                            {trendLabel(event.trend_status)}
                          </Badge>
                        </TableCell>
                      </TableRow>
                    );
                  })}
                </TableBody>
              </Table>
            </Card>
          </section>

          <aside className="h-fit xl:sticky xl:top-[96px]">
            <Card className="overflow-hidden shadow-none">
              {selected ? (
                <>
                  <CardHeader className="border-b px-5 py-5">
                    <div className="flex items-start gap-3">
                      <span className="mt-0.5 text-destructive">
                        <SignalIcon trend={selected.trend_status} />
                      </span>
                      <div className="min-w-0">
                        <h2 className="text-base font-semibold leading-6 text-foreground">
                          当前事件：{getRadarEventTitle(selected)}
                        </h2>
                        <p className="mt-2 text-xs text-muted-foreground">
                          {selected.independent_source_count ?? 0} 个独立来源 ·{" "}
                          {confirmationLabel(selected.confirmation)} · 动量{" "}
                          {formatRadarScore(selected.momentum)}
                        </p>
                      </div>
                    </div>
                  </CardHeader>

                  <CardContent className="space-y-6 px-5 py-5">
                    <section>
                      <h3 className="text-sm font-semibold text-foreground">
                        发生了什么
                      </h3>
                      <p className="mt-2 text-sm leading-7 text-muted-foreground">
                        {selected.summary ||
                          selected.latest_update?.summary ||
                          "事件信息仍在持续聚合中。"}
                      </p>
                    </section>

                    <section className="border-t pt-5">
                      <h3 className="text-sm font-semibold text-foreground">
                        为什么值得关注
                      </h3>
                      <ul className="mt-3 space-y-2 text-sm leading-6 text-muted-foreground">
                        {(selected.reason_codes?.length
                          ? selected.reason_codes
                          : ["latest"]
                        )
                          .slice(0, 3)
                          .map((reason) => (
                            <li key={reason} className="flex gap-2">
                              <span className="mt-2 h-1.5 w-1.5 shrink-0 rounded-full bg-primary" />
                              {reasonLabel(reason)}
                            </li>
                          ))}
                      </ul>
                    </section>

                    <EventIntelligencePanel
                      event={selected}
                      intelligence={intelligence}
                      intelligenceLoading={intelligenceLoading}
                      intelligenceError={intelligenceError}
                      monitorSelected={monitorId != null}
                    />

                    <EventHeatPanel
                      heat={heat}
                      loading={heatLoading}
                      error={heatError}
                    />

                    <EventGovernancePanel
                      event={selected}
                      events={events}
                      members={members}
                      role={role}
                      loading={membersLoading}
                      error={membersError}
                      busy={governanceBusy}
                      operationError={governanceError}
                      onToggleLock={toggleMemberLock}
                      onLifecycle={transitionLifecycle}
                      onMerge={mergeEvent}
                      onSplit={splitEvent}
                    />

                    <section className="border-t pt-5">
                      <div className="flex items-center justify-between">
                        <h3 className="text-sm font-semibold text-foreground">
                          最新变化
                        </h3>
                        {detailLoading ? (
                          <Loader2 className="h-4 w-4 animate-spin text-primary" />
                        ) : null}
                      </div>
                      {updatesError ? (
                        <p className="mt-3 text-sm text-muted-foreground">
                          最新变化暂时不可用，请稍后重试。
                        </p>
                      ) : updates.length ? (
                        <ol className="mt-3 space-y-4">
                          {updates.slice(0, 5).map((update) => (
                            <li key={update.id} className="flex gap-3">
                              <span className="mt-1.5 h-2 w-2 shrink-0 rounded-full bg-primary ring-4 ring-secondary" />
                              <div>
                                <p className="text-xs font-medium text-foreground">
                                  {updateKindLabel(update.kind)} ·{" "}
                                  {formatRadarTime(update.observed_at)}
                                </p>
                                <p className="mt-1 text-sm leading-6 text-muted-foreground">
                                  {update.summary || "事件状态已更新"}
                                </p>
                              </div>
                            </li>
                          ))}
                        </ol>
                      ) : detailLoading ? null : (
                        <p className="mt-3 text-sm text-muted-foreground">
                          暂无可展示的变化记录。
                        </p>
                      )}
                    </section>
                  </CardContent>

                  <div className="border-t p-4">
                    <Button asChild className="w-full gap-2">
                      <Link href="/dashboard/contents">
                        查看采集内容
                        <ExternalLink className="h-4 w-4" />
                      </Link>
                    </Button>
                    <Link
                      href="/dashboard/reports"
                      className="mt-2 flex items-center justify-center gap-1 py-2 text-xs text-muted-foreground no-underline hover:text-primary"
                    >
                      用于简报研判
                      <ArrowUpRight className="h-3.5 w-3.5" />
                    </Link>
                  </div>
                </>
              ) : null}
            </Card>
          </aside>
        </div>
      )}
    </div>
  );
}

export default function EventsPage() {
  return (
    <Suspense
      fallback={
        <div className="flex min-h-[calc(100vh-72px)] items-center justify-center">
          <Loader2 className="h-5 w-5 animate-spin text-primary" />
        </div>
      }
    >
      <EventsWorkspace />
    </Suspense>
  );
}
