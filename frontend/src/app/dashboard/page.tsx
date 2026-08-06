"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import Link from "next/link";
import {
  ArrowRight,
  BellRing,
  Bot,
  CalendarDays,
  ChevronRight,
  CircleDot,
  Loader2,
  Plus,
  RefreshCw,
  TrendingUp,
} from "lucide-react";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { getAlerts } from "@/services/hotkey/hotkey-server/alerts";
import { getMonitors } from "@/services/hotkey/hotkey-server/monitors";
import { getRadarEvents } from "@/services/hotkey/hotkey-server/radar";
import {
  confirmationLabel,
  formatRadarTime,
  getRadarEventTitle,
  reasonLabel,
  trendLabel,
  trendTone,
} from "@/lib/radarPresentation";
import { cn } from "@/lib/utils";

function currentTimeContext() {
  const hour = new Date().getHours();
  const greeting =
    hour < 11
      ? "上午好"
      : hour < 14
        ? "中午好"
        : hour < 18
          ? "下午好"
          : "晚上好";
  const today = new Intl.DateTimeFormat("zh-CN", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    weekday: "short",
  }).format(new Date());
  return { greeting, today };
}

function EventMark({ trend }: { trend?: string }) {
  const tone = trendTone(trend);
  return (
    <span
      className={cn(
        "flex h-9 w-9 shrink-0 items-center justify-center rounded-full",
        tone === "danger" && "bg-red-50 text-red-600",
        tone === "success" && "bg-emerald-50 text-emerald-600",
        tone === "muted" && "bg-slate-100 text-slate-500",
      )}
    >
      {tone === "danger" ? (
        <TrendingUp className="h-4 w-4" />
      ) : (
        <CircleDot className="h-4 w-4" />
      )}
    </span>
  );
}

export default function DashboardPage() {
  const [events, setEvents] = useState<HotKeyAPI.RadarEventResponse[]>([]);
  const [monitors, setMonitors] = useState<HotKeyAPI.MonitorResponse[]>([]);
  const [openAlerts, setOpenAlerts] = useState(0);
  const [asOf, setAsOf] = useState<string>();
  const [timeContext, setTimeContext] = useState({
    greeting: "你好",
    today: "",
  });
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string>();

  const load = useCallback(async () => {
    setLoading(true);
    setError(undefined);
    const [radarResult, monitorResult, alertResult] = await Promise.allSettled([
      getRadarEvents({ window: "24h", sort: "momentum", limit: 12 }),
      getMonitors({ limit: 4 }),
      getAlerts({ state: "open", limit: 100 }),
    ]);

    if (radarResult.status === "rejected") {
      setError(
        radarResult.reason instanceof Error
          ? radarResult.reason.message
          : "热点雷达加载失败",
      );
      setEvents([]);
    } else {
      setEvents(radarResult.value.data?.items ?? []);
      setAsOf(radarResult.value.data?.as_of);
    }
    setMonitors(
      monitorResult.status === "fulfilled"
        ? (monitorResult.value.data?.items ?? [])
        : [],
    );
    setOpenAlerts(
      alertResult.status === "fulfilled"
        ? (alertResult.value.data?.items?.length ?? 0)
        : 0,
    );
    setLoading(false);
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  useEffect(() => {
    setTimeContext(currentTimeContext());
  }, []);

  const summaryEvents = useMemo(() => events.slice(0, 3), [events]);
  const focusEvents = useMemo(() => events.slice(0, 6), [events]);
  if (loading) {
    return (
      <div className="flex min-h-[calc(100vh-72px)] items-center justify-center">
        <Loader2 className="h-5 w-5 animate-spin text-primary" />
      </div>
    );
  }

  return (
    <div className="app-page radar-page" data-testid="dashboard-overview">
      <header className="flex flex-col gap-5 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-2xl font-medium tracking-tight text-foreground sm:text-[28px]">
            {timeContext.greeting}，这是今日值得关注的变化
          </h1>
          <div className="mt-3 flex flex-wrap items-center gap-3 text-sm text-muted-foreground">
            <span className="inline-flex items-center gap-1.5">
              <CalendarDays className="h-4 w-4" />
              {timeContext.today || "今日"}
            </span>
            <span aria-hidden>·</span>
            <span>公开信息监测</span>
            {asOf ? (
              <span className="ml-1 text-xs">
                更新于 {formatRadarTime(asOf)}
              </span>
            ) : null}
          </div>
        </div>
        {events.length > 0 ? (
          <Button asChild className="self-start gap-2 px-5 shadow-sm">
            <Link href="/dashboard/settings">
              <Plus className="h-4 w-4" />
              创建监控
            </Link>
          </Button>
        ) : null}
      </header>

      {error ? (
        <Alert variant="destructive" className="mt-8">
          <BellRing className="h-4 w-4" />
          <AlertTitle>热点雷达加载失败</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
          <Button
            variant="outline"
            size="sm"
            onClick={load}
            className="mt-4 gap-2"
          >
            <RefreshCw className="h-4 w-4" />
            重新加载
          </Button>
        </Alert>
      ) : events.length === 0 ? (
        <Card className="mt-8 border-dashed shadow-none">
          <CardContent className="flex min-h-80 flex-col items-center justify-center px-6 text-center">
            <BellRing className="h-7 w-7 text-muted-foreground" />
            <h2 className="mt-4 text-lg font-semibold">
              当前窗口内还没有热点事件
            </h2>
            <p className="mt-2 max-w-md text-sm leading-6 text-muted-foreground">
              创建并发布监控后，HotKey
              会持续聚合来源、识别事件变化并在这里给出解释。
            </p>
            <Button asChild className="mt-5">
              <Link href="/dashboard/settings">创建监控</Link>
            </Button>
          </CardContent>
        </Card>
      ) : (
        <>
          <Card className="mt-8 overflow-hidden shadow-none">
            <CardHeader className="flex-row flex-wrap items-center gap-2 space-y-0 border-b px-5 py-4 sm:px-6">
              <Bot className="h-4 w-4 text-primary" />
              <h2 className="text-sm font-semibold text-foreground">
                今日 AI 摘要
              </h2>
              <Badge variant="secondary" className="text-[11px] font-medium">
                Radar 综合
              </Badge>
              <p className="ml-auto text-xs text-muted-foreground">
                基于公开信息与事件信号自动整理
              </p>
            </CardHeader>
            <CardContent className="px-5 py-0 sm:px-6">
              <ol className="divide-y">
                {summaryEvents.map((event, index) => (
                  <li
                    key={event.event_id ?? index}
                    className="grid gap-3 py-5 sm:grid-cols-[36px_minmax(0,1fr)_auto] sm:items-center"
                  >
                    <span className="text-2xl font-light text-primary">
                      {index + 1}
                    </span>
                    <div className="min-w-0">
                      <p className="font-medium leading-6 text-foreground">
                        <span className="sr-only">摘要：</span>
                        {getRadarEventTitle(event)}
                      </p>
                      <p className="mt-1 line-clamp-2 text-sm leading-6 text-muted-foreground">
                        {event.summary ||
                          event.latest_update?.summary ||
                          "事件仍在持续跟踪中。"}
                      </p>
                    </div>
                    <div className="flex items-center gap-3 text-xs text-muted-foreground sm:justify-end">
                      <span>{confirmationLabel(event.confirmation)}</span>
                      <span className="text-primary">
                        {event.independent_source_count ?? 0} 个独立来源
                      </span>
                    </div>
                  </li>
                ))}
              </ol>
            </CardContent>
          </Card>

          <div className="mt-8 grid gap-8 xl:grid-cols-[minmax(0,1fr)_320px]">
            <section className="min-w-0">
              <div className="mb-3 flex items-center justify-between">
                <h2 className="text-base font-semibold text-foreground">
                  重点事件
                </h2>
                <Link
                  href="/dashboard/events"
                  aria-label="查看全部事件"
                  className="inline-flex items-center gap-1 text-sm text-primary no-underline hover:underline"
                >
                  查看全部
                  <ChevronRight className="h-4 w-4" />
                </Link>
              </div>
              <Card className="overflow-hidden shadow-none">
                <Table aria-label="重点事件列表">
                  <TableHeader>
                    <TableRow>
                      <TableHead className="min-w-[300px]">事件</TableHead>
                      <TableHead className="min-w-[180px]">变化原因</TableHead>
                      <TableHead className="min-w-[180px]">最新进展</TableHead>
                      <TableHead>状态</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {focusEvents.map((event, index) => (
                      <TableRow key={event.event_id ?? index}>
                        <TableCell className="py-3">
                          <Link
                            href={`/dashboard/events?event=${event.event_id ?? ""}`}
                            className="flex min-w-0 items-center gap-3 text-foreground no-underline"
                          >
                            <EventMark trend={event.trend_status} />
                            <span className="min-w-0">
                              <span className="block truncate text-sm font-medium">
                                {getRadarEventTitle(event)}
                              </span>
                              <span className="mt-1 block text-xs text-muted-foreground">
                                {event.independent_source_count ?? 0} 个独立来源
                                · {confirmationLabel(event.confirmation)}
                              </span>
                            </span>
                          </Link>
                        </TableCell>
                        <TableCell className="text-xs leading-5 text-muted-foreground">
                          {reasonLabel(event.reason_codes?.[0])}
                        </TableCell>
                        <TableCell className="text-xs leading-5 text-muted-foreground">
                          {event.latest_update?.summary ||
                            event.summary ||
                            "持续跟踪中"}
                        </TableCell>
                        <TableCell>
                          <Badge
                            variant="outline"
                            className="gap-1.5 font-normal"
                          >
                            <span className="h-1.5 w-1.5 rounded-full bg-current" />
                            {trendLabel(event.trend_status)}
                          </Badge>
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </Card>
            </section>

            <aside>
              <Card className="shadow-none">
                <CardHeader className="flex-row items-center justify-between space-y-0 pb-3">
                  <h2 className="text-base font-semibold text-foreground">
                    我的监控
                  </h2>
                  <Link
                    href="/dashboard/settings"
                    className="text-xs text-muted-foreground no-underline hover:text-primary"
                  >
                    管理
                  </Link>
                </CardHeader>
                <CardContent>
                  <div className="divide-y border-y">
                    {monitors.length ? (
                      monitors.map((monitor) => (
                        <Link
                          key={monitor.id}
                          href="/dashboard/settings"
                          className="flex items-center gap-3 py-4 text-sm text-foreground no-underline hover:text-primary"
                        >
                          <span className="h-2 w-2 rounded-full bg-emerald-500" />
                          <span className="min-w-0 flex-1 truncate">
                            {monitor.name || `监控 #${monitor.id}`}
                          </span>
                          <ChevronRight className="h-4 w-4 text-muted-foreground" />
                        </Link>
                      ))
                    ) : (
                      <p className="py-5 text-sm text-muted-foreground">
                        还没有可用监控
                      </p>
                    )}
                  </div>
                  <Link
                    href="/dashboard/alerts"
                    className="mt-5 flex items-center justify-between rounded-md bg-muted px-4 py-3 text-sm text-foreground no-underline hover:text-primary"
                  >
                    <span className="inline-flex items-center gap-2">
                      <BellRing className="h-4 w-4" />
                      待处理告警
                    </span>
                    <span className="font-semibold">{openAlerts}</span>
                  </Link>
                  <Button
                    asChild
                    variant="ghost"
                    className="mt-2 w-full justify-between px-4"
                  >
                    <Link href="/dashboard/settings">
                      查看全部监控
                      <ArrowRight className="h-4 w-4" />
                    </Link>
                  </Button>
                </CardContent>
              </Card>
            </aside>
          </div>
        </>
      )}
    </div>
  );
}
