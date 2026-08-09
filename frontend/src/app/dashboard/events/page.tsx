"use client";

import { Suspense, useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import {
  ArrowUpRight,
  CalendarRange,
  CircleDot,
  ExternalLink,
  Filter,
  FilterX,
  Loader2,
  RefreshCw,
  Search,
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
import { Input } from "@/components/ui/input";
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
import {
  CursorPagination,
  DEFAULT_PAGE_SIZE,
} from "@/components/dashboard/CursorPagination";

type RadarWindow = NonNullable<HotKeyAPI.getRadarEventsParams["window"]>;
type RadarSort = NonNullable<HotKeyAPI.getRadarEventsParams["sort"]>;
type RadarLifecycle = NonNullable<
  HotKeyAPI.getRadarEventsParams["lifecycle"]
>[number];
type RadarTrend = NonNullable<HotKeyAPI.getRadarEventsParams["trend"]>[number];
type RadarVerification = NonNullable<
  HotKeyAPI.getRadarEventsParams["verification"]
>[number];

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
  const router = useRouter();
  const searchParams = useSearchParams();
  const serializedSearch = searchParams.toString();
  const role = useAuthStore((state) => state.user?.role);
  const initialQuery = searchParams.get("q")?.trim() || "";
  const initialCursor = searchParams.get("cursor") || undefined;
  const initialPage = Number(searchParams.get("page")) || 1;
  const [query, setQuery] = useState(initialQuery);
  const [searchInput, setSearchInput] = useState(initialQuery);
  const requestedEventId = Number(searchParams.get("event")) || undefined;
  const [windowValue, setWindowValue] = useState<RadarWindow>(
    (searchParams.get("window") as RadarWindow) || "24h"
  );
  const [sort, setSort] = useState<RadarSort>(
    (searchParams.get("sort") as RadarSort) || "momentum"
  );
  const [lifecycle, setLifecycle] = useState<RadarLifecycle | undefined>(
    (searchParams.get("lifecycle") as RadarLifecycle) || undefined
  );
  const [trend, setTrend] = useState<RadarTrend | undefined>(
    (searchParams.get("trend") as RadarTrend) || undefined
  );
  const [verification, setVerification] = useState<
    RadarVerification | undefined
  >((searchParams.get("verification") as RadarVerification) || undefined);
  const [minHeat, setMinHeat] = useState<number | undefined>(() => {
    const value = Number(searchParams.get("min_heat"));
    return value > 0 ? value : undefined;
  });
  const [events, setEvents] = useState<HotKeyAPI.RadarEventResponse[]>([]);
  const [monitors, setMonitors] = useState<HotKeyAPI.MonitorResponse[]>([]);
  const [monitorId, setMonitorId] = useState<number | undefined>(() => {
    const value = Number(searchParams.get("monitor"));
    return value > 0 ? value : undefined;
  });
  const [page, setPage] = useState(initialPage);
  const [cursors, setCursors] = useState<(string | undefined)[]>(() => {
    const history = Array<string | undefined>(initialPage).fill(undefined);
    history[initialPage - 1] = initialCursor;
    return history;
  });
  const [currentCursor, setCurrentCursor] = useState(initialCursor);
  const [nextCursor, setNextCursor] = useState<string>();
  const [pageSize, setPageSize] = useState(() => {
    const value = Number(searchParams.get("limit"));
    return value > 0 ? value : 50;
  });
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

  const replaceURL = useCallback(
    (changes: Record<string, string | undefined>) => {
      const next = new URLSearchParams(searchParams.toString());
      Object.entries(changes).forEach(([key, value]) =>
        value ? next.set(key, value) : next.delete(key)
      );
      router.replace(
        `/dashboard/events${next.size ? `?${next.toString()}` : ""}`,
        { scroll: false }
      );
    },
    [router, searchParams]
  );

  useEffect(() => {
    const params = new URLSearchParams(serializedSearch);
    const nextQuery = params.get("q")?.trim() ?? "";
    const nextMonitor = Number(params.get("monitor")) || undefined;
    const nextSort = (params.get("sort") as RadarSort) || "momentum";
    setQuery(nextQuery);
    setSearchInput(nextQuery);
    setWindowValue((params.get("window") as RadarWindow) || "24h");
    setMonitorId(nextMonitor);
    setSort(nextMonitor || nextSort !== "relevance" ? nextSort : "momentum");
    setLifecycle((params.get("lifecycle") as RadarLifecycle) || undefined);
    setTrend((params.get("trend") as RadarTrend) || undefined);
    setVerification(
      (params.get("verification") as RadarVerification) || undefined
    );
    setMinHeat(Number(params.get("min_heat")) || undefined);
    setPage(Number(params.get("page")) || 1);
    setCurrentCursor(params.get("cursor") || undefined);
    setPageSize(Number(params.get("limit")) || 50);
  }, [serializedSearch]);

  const loadRadar = useCallback(async () => {
    setLoading(true);
    setError(undefined);
    try {
      const result = await getRadarEvents({
        window: windowValue,
        sort,
        limit: pageSize,
        ...(query ? { q: query } : {}),
        ...(currentCursor ? { cursor: currentCursor } : {}),
        ...(monitorId != null ? { monitor_id: monitorId } : {}),
        ...(lifecycle ? { lifecycle: [lifecycle] } : {}),
        ...(trend ? { trend: [trend] } : {}),
        ...(verification ? { verification: [verification] } : {}),
        ...(minHeat != null ? { min_heat: minHeat } : {}),
      });
      const items = result.data?.items ?? [];
      setEvents(items);
      setAsOf(result.data?.as_of);
      setNextCursor(result.data?.next_cursor);
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
  }, [
    currentCursor,
    lifecycle,
    minHeat,
    monitorId,
    pageSize,
    query,
    requestedEventId,
    sort,
    trend,
    verification,
    windowValue,
  ]);

  useEffect(() => {
    const timer = window.setTimeout(() => {
      const next = searchInput.trim();
      if (next === query) return;
      setQuery(next);
      setPage(1);
      setCursors([undefined]);
      setCurrentCursor(undefined);
      replaceURL({ q: next || undefined, cursor: undefined, page: undefined });
    }, 300);
    return () => window.clearTimeout(timer);
  }, [query, replaceURL, searchInput]);

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

  const selected = events.find((event) => event.event_id === selectedId);

  const resetPage = (changes: Record<string, string | undefined>) => {
    setPage(1);
    setCursors([undefined]);
    setCurrentCursor(undefined);
    replaceURL({ ...changes, cursor: undefined, page: undefined });
  };

  const clearFilters = () => {
    setSearchInput("");
    setQuery("");
    setMonitorId(undefined);
    setWindowValue("24h");
    setSort("momentum");
    setLifecycle(undefined);
    setTrend(undefined);
    setVerification(undefined);
    setMinHeat(undefined);
    setPage(1);
    setCursors([undefined]);
    setCurrentCursor(undefined);
    replaceURL({
      q: undefined,
      monitor: undefined,
      window: undefined,
      sort: undefined,
      lifecycle: undefined,
      trend: undefined,
      verification: undefined,
      min_heat: undefined,
      cursor: undefined,
      page: undefined,
    });
  };

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

      <Card className="mt-5 shadow-none">
        <CardContent className="flex flex-wrap items-end gap-3 p-4">
          <div className="relative min-w-[240px] flex-1">
            <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              aria-label="搜索事件"
              className="pl-9"
              maxLength={100}
              placeholder="搜索事件标题或摘要"
              type="search"
              value={searchInput}
              onChange={(event) => setSearchInput(event.target.value)}
              onKeyDown={(event) => {
                if (event.key === "Enter") {
                  event.preventDefault();
                  const next = searchInput.trim();
                  setQuery(next);
                  resetPage({ q: next || undefined });
                }
              }}
            />
          </div>
          <Select
            value={monitorId?.toString() ?? "all"}
            onValueChange={(value) => {
              if (value === "all") {
                setMonitorId(undefined);
                if (sort === "relevance") setSort("momentum");
                resetPage({
                  monitor: undefined,
                  sort: sort === "relevance" ? undefined : sort,
                });
                return;
              }
              setMonitorId(Number(value));
              resetPage({ monitor: value });
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
            onValueChange={(value) => {
              setWindowValue(value as RadarWindow);
              resetPage({ window: value === "24h" ? undefined : value });
            }}
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
            onValueChange={(value) => {
              setSort(value as RadarSort);
              resetPage({ sort: value === "momentum" ? undefined : value });
            }}
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
          <Select
            value={lifecycle ?? "all"}
            onValueChange={(value) => {
              const next =
                value === "all" ? undefined : (value as RadarLifecycle);
              setLifecycle(next);
              resetPage({ lifecycle: next });
            }}
          >
            <SelectTrigger aria-label="生命周期" className="w-[150px]">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">全部生命周期</SelectItem>
              <SelectItem value="detected">已发现</SelectItem>
              <SelectItem value="active">活跃</SelectItem>
              <SelectItem value="cooling">降温</SelectItem>
              <SelectItem value="closed">已关闭</SelectItem>
            </SelectContent>
          </Select>
          <Select
            value={trend ?? "all"}
            onValueChange={(value) => {
              const next = value === "all" ? undefined : (value as RadarTrend);
              setTrend(next);
              resetPage({ trend: next });
            }}
          >
            <SelectTrigger aria-label="趋势状态" className="w-[140px]">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">全部趋势</SelectItem>
              <SelectItem value="emerging">新兴</SelectItem>
              <SelectItem value="rising">上升</SelectItem>
              <SelectItem value="stable">稳定</SelectItem>
              <SelectItem value="falling">下降</SelectItem>
              <SelectItem value="dormant">沉寂</SelectItem>
            </SelectContent>
          </Select>
          <Select
            value={verification ?? "all"}
            onValueChange={(value) => {
              const next =
                value === "all" ? undefined : (value as RadarVerification);
              setVerification(next);
              resetPage({ verification: next });
            }}
          >
            <SelectTrigger aria-label="证据状态" className="w-[150px]">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">全部证据状态</SelectItem>
              <SelectItem value="corroborated">多源印证</SelectItem>
              <SelectItem value="disputed">存在争议</SelectItem>
              <SelectItem value="single_source">单一来源</SelectItem>
              <SelectItem value="unverified">未验证</SelectItem>
              <SelectItem value="insufficient">证据不足</SelectItem>
            </SelectContent>
          </Select>
          <Select
            value={minHeat?.toString() ?? "all"}
            onValueChange={(value) => {
              const next = value === "all" ? undefined : Number(value);
              setMinHeat(next);
              resetPage({ min_heat: next?.toString() });
            }}
          >
            <SelectTrigger aria-label="最低热度" className="w-[130px]">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">不限热度</SelectItem>
              <SelectItem value="40">热度 ≥ 40</SelectItem>
              <SelectItem value="70">热度 ≥ 70</SelectItem>
            </SelectContent>
          </Select>
          <Button onClick={clearFilters} variant="outline">
            <FilterX />
            清除筛选
          </Button>
          {query ? (
            <Button asChild variant="ghost">
              <Link href={`/dashboard/contents?q=${encodeURIComponent(query)}`}>
                在内容中搜索
              </Link>
            </Button>
          ) : null}
        </CardContent>
      </Card>

      {error ? (
        <Alert variant="destructive" className="mt-6">
          <CircleDot className="h-4 w-4" />
          <AlertTitle>事件雷达加载失败</AlertTitle>
          <AlertDescription className="flex flex-wrap items-center justify-between gap-3">
            <span>{error}</span>
            <Button
              size="sm"
              variant="outline"
              onClick={() => void loadRadar()}
              aria-label="重试事件"
            >
              重试
            </Button>
          </AlertDescription>
        </Alert>
      ) : loading ? (
        <div className="flex h-96 items-center justify-center">
          <Loader2 className="h-5 w-5 animate-spin text-primary" />
        </div>
      ) : events.length === 0 ? (
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
        <div className="mt-6 grid min-h-[620px] items-start gap-x-8 gap-y-10 2xl:grid-cols-2 2xl:gap-y-0">
          <section
            aria-label="事件列表"
            className="min-w-0"
            data-layout-panel="event-workspace"
          >
            <div className="mb-3 flex h-6 items-center justify-between gap-3">
              <h2 className="flex items-center gap-2 text-sm font-semibold text-foreground">
                <span className="h-2 w-2 rounded-full bg-destructive" />
                需要关注
              </h2>
              <span className="text-xs text-muted-foreground">
                {events.length} 个事件
              </span>
            </div>
            <Card className="overflow-hidden shadow-none">
              <Table aria-label="热点事件列表" scrollAreaLabel="热点事件列表">
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
                  {events.map((event, index) => {
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
                            onClick={() => {
                              setSelectedId(event.event_id);
                              replaceURL({ event: event.event_id?.toString() });
                            }}
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
              <CursorPagination
                hasNext={Boolean(nextCursor)}
                loading={loading}
                onNext={() => {
                  if (!nextCursor) return;
                  const nextPage = page + 1;
                  setCursors((history) => [
                    ...history.slice(0, page),
                    nextCursor,
                  ]);
                  setCurrentCursor(nextCursor);
                  setPage(nextPage);
                  replaceURL({ cursor: nextCursor, page: String(nextPage) });
                }}
                onPageSizeChange={(value) => {
                  setPageSize(value);
                  resetPage({
                    limit:
                      value === DEFAULT_PAGE_SIZE ? undefined : String(value),
                  });
                }}
                onPrevious={() => {
                  if (page <= 1) return;
                  const previousPage = page - 1;
                  const cursor = cursors[previousPage - 1];
                  setCurrentCursor(cursor);
                  setPage(previousPage);
                  replaceURL({
                    cursor,
                    page: previousPage === 1 ? undefined : String(previousPage),
                  });
                }}
                page={page}
                pageSize={pageSize}
              />
            </Card>
          </section>

          <aside
            aria-label="事件研判"
            className="h-fit min-w-0 2xl:sticky 2xl:top-[96px]"
            data-layout-panel="event-workspace"
          >
            <div className="mb-3 flex h-6 items-center justify-between gap-3">
              <h2 className="flex items-center gap-2 text-sm font-semibold text-foreground">
                <span className="h-2 w-2 rounded-full bg-destructive" />
                事件研判
              </h2>
              <span className="text-xs text-muted-foreground">持续更新</span>
            </div>
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
