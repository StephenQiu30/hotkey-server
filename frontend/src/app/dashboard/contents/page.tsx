"use client";

import { Suspense, useCallback, useEffect, useState } from "react";
import {
  FilterX,
  Flame,
  Loader2,
  RefreshCw,
  Search,
  ShieldAlert,
} from "lucide-react";
import { useRouter, useSearchParams } from "next/navigation";
import { PageShell } from "@/layouts/PageShell";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Surface } from "@/components/ui/surface";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  CursorPagination,
  DEFAULT_PAGE_SIZE,
} from "@/components/dashboard/CursorPagination";
import { HotspotCard } from "@/components/dashboard/HotspotCard";
import { PageHeader } from "@/components/dashboard/PageHeader";
import { getHotspots } from "@/services/hotkey/hotkey-server/hotspots";
import { getMonitors } from "@/services/hotkey/hotkey-server/monitors";
import { getSourceConnections } from "@/services/hotkey/hotkey-server/sources";
import { HotKeyAPIError } from "@/lib/request";

type HotspotSort = NonNullable<HotKeyAPI.getHotspotsParams["sort"]>;

const sortLabels: Readonly<Record<HotspotSort, string>> = {
  discovered: "最新发现",
  published: "最新发布",
  importance: "重要性",
  relevance: "相关性",
  heat: "热度",
};

function HotspotLoadingState() {
  return (
    <div
      aria-label="正在加载热点"
      aria-live="polite"
      className="flex min-h-80 items-center justify-center"
      role="status"
    >
      <Loader2
        aria-hidden="true"
        className="h-5 w-5 animate-spin text-muted-foreground"
      />
      <span className="sr-only">正在加载热点</span>
    </div>
  );
}

function positiveNumber(value: string | null) {
  const parsed = Number(value);
  return Number.isInteger(parsed) && parsed > 0 ? parsed : undefined;
}

function HotspotRadar() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const initialPage = positiveNumber(searchParams.get("page")) ?? 1;
  const initialCursor = searchParams.get("cursor") || undefined;
  const [query, setQuery] = useState(searchParams.get("q")?.trim() ?? "");
  const [queryInput, setQueryInput] = useState(query);
  const [sourceID, setSourceID] = useState(
    positiveNumber(searchParams.get("source"))
  );
  const [monitorID, setMonitorID] = useState(
    positiveNumber(searchParams.get("monitor"))
  );
  const [publishedFrom, setPublishedFrom] = useState(
    searchParams.get("from") ?? ""
  );
  const [publishedTo, setPublishedTo] = useState(searchParams.get("to") ?? "");
  const [sort, setSort] = useState<HotspotSort>(
    (searchParams.get("sort") as HotspotSort) || "discovered"
  );
  const [pageSize, setPageSize] = useState(
    positiveNumber(searchParams.get("limit")) ?? DEFAULT_PAGE_SIZE
  );
  const [page, setPage] = useState(initialPage);
  const [cursor, setCursor] = useState(initialCursor);
  const [cursors, setCursors] = useState<(string | undefined)[]>(() => {
    const history = Array<string | undefined>(initialPage).fill(undefined);
    history[initialPage - 1] = initialCursor;
    return history;
  });
  const [nextCursor, setNextCursor] = useState<string>();
  const [items, setItems] = useState<HotKeyAPI.HotspotCardResponse[]>([]);
  const [summary, setSummary] = useState<HotKeyAPI.HotspotSummaryResponse>({});
  const [sources, setSources] = useState<HotKeyAPI.SourceReadResponse[]>([]);
  const [monitors, setMonitors] = useState<HotKeyAPI.MonitorResponse[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string>();
  const [forbidden, setForbidden] = useState(false);

  const replaceURL = useCallback(
    (changes: Record<string, string | undefined>) => {
      const next = new URLSearchParams(searchParams.toString());
      Object.entries(changes).forEach(([key, value]) =>
        value ? next.set(key, value) : next.delete(key)
      );
      router.replace(
        `/dashboard/contents${next.size ? `?${next.toString()}` : ""}`,
        { scroll: false }
      );
    },
    [router, searchParams]
  );

  const resetPage = useCallback(
    (changes: Record<string, string | undefined>) => {
      setPage(1);
      setCursor(undefined);
      setCursors([undefined]);
      replaceURL({ ...changes, cursor: undefined, page: undefined });
    },
    [replaceURL]
  );

  const load = useCallback(async () => {
    setLoading(true);
    setError(undefined);
    setForbidden(false);
    try {
      const result = await getHotspots({
        limit: pageSize,
        ...(cursor ? { cursor } : {}),
        ...(query ? { q: query } : {}),
        ...(sourceID ? { source_connection_id: sourceID } : {}),
        ...(monitorID ? { monitor_id: monitorID } : {}),
        ...(publishedFrom
          ? { published_from: `${publishedFrom}T00:00:00Z` }
          : {}),
        ...(publishedTo ? { published_to: `${publishedTo}T23:59:59Z` } : {}),
        sort,
      });
      setItems(result.data?.items ?? []);
      setSummary(result.data?.summary ?? {});
      setNextCursor(result.data?.next_cursor);
    } catch (reason) {
      setItems([]);
      setNextCursor(undefined);
      setForbidden(reason instanceof HotKeyAPIError && reason.status === 403);
      setError(reason instanceof Error ? reason.message : "热点加载失败");
    } finally {
      setLoading(false);
    }
  }, [
    cursor,
    monitorID,
    pageSize,
    publishedFrom,
    publishedTo,
    query,
    sort,
    sourceID,
  ]);

  useEffect(() => {
    void load();
  }, [load]);

  useEffect(() => {
    let active = true;
    Promise.allSettled([
      getSourceConnections({ limit: 100 }),
      getMonitors({ limit: 100 }),
    ]).then(([sourceResult, monitorResult]) => {
      if (!active) return;
      setSources(
        sourceResult.status === "fulfilled"
          ? sourceResult.value.data?.items ?? []
          : []
      );
      setMonitors(
        monitorResult.status === "fulfilled"
          ? monitorResult.value.data?.items ?? []
          : []
      );
    });
    return () => {
      active = false;
    };
  }, []);

  useEffect(() => {
    const timer = window.setTimeout(() => {
      const next = queryInput.trim();
      if (next === query) return;
      setQuery(next);
      resetPage({ q: next || undefined });
    }, 300);
    return () => window.clearTimeout(timer);
  }, [query, queryInput, resetPage]);

  const filtered = Boolean(
    query ||
      sourceID ||
      monitorID ||
      publishedFrom ||
      publishedTo ||
      sort !== "discovered"
  );
  const activeMonitors = monitors.filter(
    (monitor) => monitor.status === "active"
  ).length;

  function clearFilters() {
    setQuery("");
    setQueryInput("");
    setSourceID(undefined);
    setMonitorID(undefined);
    setPublishedFrom("");
    setPublishedTo("");
    setSort("discovered");
    resetPage({
      q: undefined,
      source: undefined,
      monitor: undefined,
      from: undefined,
      to: undefined,
      sort: undefined,
    });
  }

  function nextPage() {
    if (!nextCursor) return;
    const nextPageNumber = page + 1;
    setCursors((history) => [...history.slice(0, page), nextCursor]);
    setCursor(nextCursor);
    setPage(nextPageNumber);
    replaceURL({ cursor: nextCursor, page: String(nextPageNumber) });
  }

  function previousPage() {
    if (page <= 1) return;
    const previousPageNumber = page - 1;
    const previousCursor = cursors[previousPageNumber - 1];
    setCursor(previousCursor);
    setPage(previousPageNumber);
    replaceURL({
      cursor: previousCursor,
      page: previousPageNumber === 1 ? undefined : String(previousPageNumber),
    });
  }

  const pageHeader = (
    <PageHeader
      action={
        forbidden ? undefined : (
          <Button
            className="gap-2"
            onClick={() => void load()}
            variant="outline"
          >
            <RefreshCw className={loading ? "animate-spin" : ""} />
            刷新热点
          </Button>
        )
      }
      description="持续查看监控扫描发现的文章、帖子和视频，并按来源、监控与热度快速定位。"
      eyebrow="HOTSPOT RADAR"
      title="热点雷达"
    />
  );

  if (forbidden) {
    return (
      <PageShell>
        {pageHeader}
        <Alert aria-label="热点访问权限不足" className="mt-6">
          <ShieldAlert />
          <AlertTitle>热点访问权限不足</AlertTitle>
          <AlertDescription>
            当前账号无法读取热点内容。请联系管理员核对工作区角色。
          </AlertDescription>
        </Alert>
      </PageShell>
    );
  }

  return (
    <PageShell>
      {pageHeader}

      <section
        aria-label="热点统计"
        className="mt-6 grid gap-3 sm:grid-cols-2 xl:grid-cols-4"
      >
        {[
          ["总热点", summary.total ?? 0],
          ["今日新增", summary.today ?? 0],
          ["紧急热点", summary.urgent ?? 0],
          ["启用监控", activeMonitors],
        ].map(([label, value]) => (
          <Card key={label}>
            <CardContent className="p-4">
              <p className="text-xs text-muted-foreground">{label}</p>
              <p className="mono mt-3 text-2xl font-medium">{value}</p>
            </CardContent>
          </Card>
        ))}
      </section>

      <Card className="mt-6 overflow-hidden" role="region" aria-label="热点筛选">
        <CardHeader className="flex flex-col gap-4 border-b px-5 py-4 sm:flex-row sm:items-center sm:justify-between">
          <div className="space-y-1">
            <CardTitle className="text-sm">筛选热点</CardTitle>
            <CardDescription>
              条件会同步到地址栏，便于保留和分享当前视图。
            </CardDescription>
          </div>
          <Button
            className="w-full sm:w-auto"
            disabled={!filtered}
            onClick={clearFilters}
            size="sm"
            variant="outline"
          >
            <FilterX />
            清除筛选
          </Button>
        </CardHeader>
        <CardContent className="grid gap-4 p-5 md:grid-cols-2 xl:grid-cols-12">
          <div className="space-y-2 md:col-span-2 xl:col-span-3">
            <Label htmlFor="hotspot-search">搜索热点</Label>
            <div className="relative">
              <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                className="pl-9"
                id="hotspot-search"
                maxLength={100}
                onChange={(event) => setQueryInput(event.target.value)}
                placeholder="搜索标题或摘要"
                type="search"
                value={queryInput}
              />
            </div>
          </div>
          <div className="space-y-2 xl:col-span-2">
            <Label htmlFor="hotspot-source">来源</Label>
            <Select
              value={sourceID?.toString() ?? "all"}
              onValueChange={(value) => {
                const next = value === "all" ? undefined : Number(value);
                setSourceID(next);
                resetPage({ source: next?.toString() });
              }}
            >
              <SelectTrigger aria-label="热点来源" id="hotspot-source">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">全部来源</SelectItem>
                {sources
                  .filter((source) => source.id != null && !source.deleted)
                  .map((source) => (
                    <SelectItem key={source.id} value={String(source.id)}>
                      {source.name || `来源 #${source.id}`}
                    </SelectItem>
                  ))}
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-2 xl:col-span-2">
            <Label htmlFor="hotspot-monitor">监控</Label>
            <Select
              value={monitorID?.toString() ?? "all"}
              onValueChange={(value) => {
                const next = value === "all" ? undefined : Number(value);
                setMonitorID(next);
                if (!next && sort === "relevance") setSort("discovered");
                resetPage({
                  monitor: next?.toString(),
                  sort:
                    !next && sort === "relevance"
                      ? undefined
                      : sort === "discovered"
                      ? undefined
                      : sort,
                });
              }}
            >
              <SelectTrigger aria-label="热点监控" id="hotspot-monitor">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">全部监控</SelectItem>
                {monitors
                  .filter(
                    (monitor) =>
                      monitor.id != null && monitor.status !== "archived"
                  )
                  .map((monitor) => (
                    <SelectItem key={monitor.id} value={String(monitor.id)}>
                      {monitor.name || `监控 #${monitor.id}`}
                    </SelectItem>
                  ))}
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-2 xl:col-span-2">
            <Label htmlFor="hotspot-sort">排序</Label>
            <Select
              value={sort}
              onValueChange={(value) => {
                const next = value as HotspotSort;
                setSort(next);
                resetPage({ sort: next === "discovered" ? undefined : next });
              }}
            >
              <SelectTrigger aria-label="热点排序" id="hotspot-sort">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {(
                  Object.entries(sortLabels) as Array<[HotspotSort, string]>
                ).map(([value, label]) => (
                  <SelectItem
                    disabled={value === "relevance" && !monitorID}
                    key={value}
                    value={value}
                  >
                    {label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="grid grid-cols-1 gap-4 md:col-span-2 md:grid-cols-2 xl:col-span-3">
            <div className="space-y-2">
              <Label htmlFor="hotspot-from">开始日期</Label>
              <Input
                id="hotspot-from"
                type="date"
                value={publishedFrom}
                onChange={(event) => {
                  setPublishedFrom(event.target.value);
                  resetPage({ from: event.target.value || undefined });
                }}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="hotspot-to">结束日期</Label>
              <Input
                id="hotspot-to"
                type="date"
                value={publishedTo}
                onChange={(event) => {
                  setPublishedTo(event.target.value);
                  resetPage({ to: event.target.value || undefined });
                }}
              />
            </div>
          </div>
        </CardContent>
      </Card>

      {error ? (
        <Alert className="mt-6" variant="destructive">
          <AlertTitle>热点加载失败</AlertTitle>
          <AlertDescription className="flex flex-wrap items-center justify-between gap-3">
            <span>{error}</span>
            <Button onClick={() => void load()} size="sm" variant="outline">
              重试
            </Button>
          </AlertDescription>
        </Alert>
      ) : null}

      {loading && items.length === 0 ? (
        <HotspotLoadingState />
      ) : null}

      {!loading && !error && items.length === 0 ? (
        <Card className="mt-6" variant="subtle">
          <Empty className="min-h-72 border-0">
            <EmptyHeader>
              <EmptyMedia variant="icon">
                <Flame />
              </EmptyMedia>
              <EmptyTitle>暂时没有热点</EmptyTitle>
              <EmptyDescription>
                {filtered
                  ? "当前筛选条件没有结果，可以清除筛选后查看全部热点。"
                  : "创建监控并立即扫描后，新内容会出现在这里。"}
              </EmptyDescription>
            </EmptyHeader>
            {filtered ? (
              <EmptyContent>
                <Button onClick={clearFilters} variant="outline">
                  <FilterX />
                  清除筛选
                </Button>
              </EmptyContent>
            ) : null}
          </Empty>
        </Card>
      ) : null}

      {items.length ? (
        <section aria-live="polite" className="mt-6 space-y-4">
          {items.map((card, index) => (
            <HotspotCard
              card={card}
              key={
                card.id ?? `${card.source_type}-${card.external_id}-${index}`
              }
            />
          ))}
          <Surface className="overflow-hidden" variant="ring">
            <CursorPagination
              hasNext={Boolean(nextCursor)}
              loading={loading}
              onNext={nextPage}
              onPageSizeChange={(value) => {
                setPageSize(value);
                resetPage({
                  limit:
                    value === DEFAULT_PAGE_SIZE ? undefined : String(value),
                });
              }}
              onPrevious={previousPage}
              page={page}
              pageSize={pageSize}
            />
          </Surface>
        </section>
      ) : null}
    </PageShell>
  );
}

export default function ContentsPage() {
  return (
    <Suspense
      fallback={
        <HotspotLoadingState />
      }
    >
      <HotspotRadar />
    </Suspense>
  );
}
