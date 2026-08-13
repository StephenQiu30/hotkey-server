"use client";

import { Suspense, useCallback, useEffect, useState } from "react";
import { FilterX, Flame, Loader2, RefreshCw, Search } from "lucide-react";
import { useRouter, useSearchParams } from "next/navigation";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
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
  CursorPagination,
  DEFAULT_PAGE_SIZE,
} from "@/components/dashboard/CursorPagination";
import { HotspotCard } from "@/components/dashboard/HotspotCard";
import { PageHeader } from "@/components/dashboard/PageHeader";
import { getHotspots } from "@/services/hotkey/hotkey-server/hotspots";
import { getMonitors } from "@/services/hotkey/hotkey-server/monitors";
import { getSourceConnections } from "@/services/hotkey/hotkey-server/sources";

type HotspotSort = NonNullable<HotKeyAPI.getHotspotsParams["sort"]>;

const sortLabels: Readonly<Record<HotspotSort, string>> = {
  discovered: "最新发现",
  published: "最新发布",
  importance: "重要性",
  relevance: "相关性",
  heat: "热度",
};

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

  return (
    <main className="app-page">
      <PageHeader
        action={
          <Button
            className="gap-2"
            onClick={() => void load()}
            variant="outline"
          >
            <RefreshCw className={loading ? "animate-spin" : ""} />
            刷新热点
          </Button>
        }
        description="持续查看监控扫描发现的文章、帖子和视频，并按来源、监控与热度快速定位。"
        eyebrow="HOTSPOT RADAR"
        title="热点雷达"
      />

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

      <Card className="mt-6 shadow-none">
        <CardContent className="grid gap-4 p-4 md:grid-cols-2 xl:grid-cols-6">
          <div className="space-y-2 md:col-span-2">
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
          <div className="space-y-2">
            <Label>来源</Label>
            <Select
              value={sourceID?.toString() ?? "all"}
              onValueChange={(value) => {
                const next = value === "all" ? undefined : Number(value);
                setSourceID(next);
                resetPage({ source: next?.toString() });
              }}
            >
              <SelectTrigger aria-label="热点来源">
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
          <div className="space-y-2">
            <Label>监控</Label>
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
              <SelectTrigger aria-label="热点监控">
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
          <div className="space-y-2 md:col-span-2 xl:col-span-2">
            <Label>排序</Label>
            <Select
              value={sort}
              onValueChange={(value) => {
                const next = value as HotspotSort;
                setSort(next);
                resetPage({ sort: next === "discovered" ? undefined : next });
              }}
            >
              <SelectTrigger aria-label="热点排序">
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
          <div className="flex items-end md:col-span-2 xl:col-span-4">
            <Button
              disabled={!filtered}
              onClick={clearFilters}
              variant="outline"
            >
              <FilterX />
              清除筛选
            </Button>
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
        <div className="flex min-h-80 items-center justify-center">
          <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
        </div>
      ) : null}

      {!loading && !error && items.length === 0 ? (
        <div className="mt-6 rounded-xl border border-dashed border-border px-6 py-14 text-center">
          <Flame className="mx-auto h-6 w-6 text-muted-foreground" />
          <h2 className="mt-4 font-medium">暂时没有热点</h2>
          <p className="mt-2 text-sm text-muted-foreground">
            {filtered
              ? "调整或清除筛选条件。"
              : "创建监控并立即扫描后，新内容会出现在这里。"}
          </p>
        </div>
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
          <div className="overflow-hidden rounded-xl border border-border">
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
          </div>
        </section>
      ) : null}
    </main>
  );
}

export default function ContentsPage() {
  return (
    <Suspense
      fallback={
        <div className="flex min-h-80 items-center justify-center">
          <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
        </div>
      }
    >
      <HotspotRadar />
    </Suspense>
  );
}
