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
  TrendingDown,
  TrendingUp,
} from "lucide-react";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
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
import { getEventsIdUpdates } from "@/services/hotkey/hotkey-server/events";
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
  const query = searchParams.get("q")?.trim().toLocaleLowerCase("zh-CN") || "";
  const requestedEventId = Number(searchParams.get("event")) || undefined;
  const [windowValue, setWindowValue] = useState<RadarWindow>("24h");
  const [sort, setSort] = useState<RadarSort>("momentum");
  const [events, setEvents] = useState<HotKeyAPI.RadarEventResponse[]>([]);
  const [selectedId, setSelectedId] = useState<number>();
  const [updates, setUpdates] = useState<HotKeyAPI.EventUpdateResponse[]>([]);
  const [asOf, setAsOf] = useState<string>();
  const [loading, setLoading] = useState(true);
  const [detailLoading, setDetailLoading] = useState(false);
  const [error, setError] = useState<string>();

  const loadRadar = useCallback(async () => {
    setLoading(true);
    setError(undefined);
    try {
      const result = await getRadarEvents({
        window: windowValue,
        sort,
        limit: 50,
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
  }, [requestedEventId, sort, windowValue]);

  useEffect(() => {
    void loadRadar();
  }, [loadRadar]);

  useEffect(() => {
    if (selectedId == null) {
      setUpdates([]);
      return;
    }
    let active = true;
    setDetailLoading(true);
    getEventsIdUpdates({ id: selectedId, limit: 20 })
      .then((result) => {
        if (active) setUpdates(result.data?.items ?? []);
      })
      .catch(() => {
        if (active) setUpdates([]);
      })
      .finally(() => {
        if (active) setDetailLoading(false);
      });
    return () => {
      active = false;
    };
  }, [selectedId]);

  const visibleEvents = useMemo(() => {
    if (!query) return events;
    return events.filter((event) =>
      [getRadarEventTitle(event), event.summary, event.latest_update?.summary]
        .filter(Boolean)
        .join(" ")
        .toLocaleLowerCase("zh-CN")
        .includes(query),
    );
  }, [events, query]);

  const selected = events.find((event) => event.event_id === selectedId);

  return (
    <div className="app-page radar-page !max-w-[1536px]">
      <header className="flex flex-col gap-5 border-b pb-6 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <h1 className="text-3xl font-semibold tracking-tight text-foreground">
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
            <SelectItem value="relevance">监控相关性</SelectItem>
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
        <Card className="mt-6 border-dashed shadow-none">
          <CardContent className="flex h-80 flex-col items-center justify-center text-center">
            <SearchX className="h-6 w-6 text-muted-foreground" />
            <p className="mt-3 text-sm font-medium">没有符合当前条件的事件</p>
            <p className="mt-1 text-xs text-muted-foreground">
              调整时间窗口或清除搜索后重试。
            </p>
          </CardContent>
        </Card>
      ) : (
        <div className="mt-6 grid min-h-[620px] gap-6 xl:grid-cols-[minmax(0,1fr)_380px]">
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
                    <TableHead>来源广度</TableHead>
                    <TableHead>首次发现</TableHead>
                    <TableHead>趋势</TableHead>
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
                                tone === "muted" && "text-muted-foreground",
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
                        <TableCell>
                          {event.independent_source_count ?? 0} 个
                        </TableCell>
                        <TableCell>
                          {formatRadarTime(event.first_seen_at)}
                        </TableCell>
                        <TableCell>
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

                    <section className="border-t pt-5">
                      <div className="flex items-center justify-between">
                        <h3 className="text-sm font-semibold text-foreground">
                          最新变化
                        </h3>
                        {detailLoading ? (
                          <Loader2 className="h-4 w-4 animate-spin text-primary" />
                        ) : null}
                      </div>
                      {updates.length ? (
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
