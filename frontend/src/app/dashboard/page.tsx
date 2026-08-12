"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import Link from "next/link";
import {
  ArrowRight,
  BellRing,
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
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { getMicroEvents } from "@/services/hotkey/hotkey-server/microEvents";
import { getMonitors } from "@/services/hotkey/hotkey-server/monitors";
import { cn } from "@/lib/utils";
import { UserRole } from "@/lib/domainEnums";
import { useAuthStore } from "@/stores/authStore";

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

function formatTime(value?: string) {
  if (!value) return "—";
  const date = new Date(value);
  if (Number.isNaN(date.valueOf())) return "—";
  return new Intl.DateTimeFormat("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  }).format(date);
}

function microEventTitle(event: HotKeyAPI.MicroEventResponseDTO) {
  const subject = event.primary_subject_key?.trim() || "未命名主体";
  const action = event.primary_action_key?.trim() || "未命名动作";
  return `${subject} · ${action}`;
}

function microEventSummary(event: HotKeyAPI.MicroEventResponseDTO) {
  return (
    event.evidence_summary?.sentences?.[0]?.text?.trim() ||
    event.storyline?.summary?.trim() ||
    event.storyline?.title?.trim() ||
    "事件仍在持续跟踪中。"
  );
}

function eventStateLabel(value?: string) {
  return (
    ({
      active: "活跃",
      review_pending: "待复核",
      closed: "已关闭",
      merged: "已合并",
    } as Record<string, string>)[value ?? ""] || "状态未知"
  );
}

function heatReason(event: HotKeyAPI.MicroEventResponseDTO) {
  const code = event.latest_heat?.reason_codes?.[0];
  return (
    ({
      velocity_rising: "报道速度正在上升",
      acceleration_rising: "报道增速正在上升",
      coverage_growing: "内容起源覆盖正在扩大",
      recency_high: "近期出现新的报道",
      metrics_unavailable: "部分互动指标不可用，热度已重归一化",
    } as Record<string, string>)[code ?? ""] || "事件证据集合出现新的变化"
  );
}

function EventMark({ acceleration }: { acceleration?: number }) {
  const tone =
    acceleration == null || acceleration === 0
      ? "success"
      : acceleration > 0
        ? "danger"
        : "muted";
  return (
    <span
      className={cn(
        "flex h-9 w-9 shrink-0 items-center justify-center rounded-full",
        tone === "danger" && "bg-red-50 text-red-600",
        tone === "success" && "bg-emerald-50 text-emerald-600",
        tone === "muted" && "bg-muted text-muted-foreground"
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
  const role = useAuthStore((state) => state.user?.role);
  const canManage = role === UserRole.Admin || role === UserRole.Editor;
  const [events, setEvents] = useState<HotKeyAPI.MicroEventResponseDTO[]>([]);
  const [monitors, setMonitors] = useState<HotKeyAPI.MonitorResponse[]>([]);
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
    const [eventResult, monitorResult] = await Promise.allSettled([
      getMicroEvents({ limit: 12 }),
      getMonitors({ limit: 4 }),
    ]);

    if (eventResult.status === "rejected") {
      setError(
        eventResult.reason instanceof Error
          ? eventResult.reason.message
          : "热点事件加载失败"
      );
      setEvents([]);
    } else {
      const items = eventResult.value.data?.items ?? [];
      setEvents(items);
      setAsOf(items.find((item) => item.latest_heat?.window_ended_at)?.latest_heat?.window_ended_at);
    }
    setMonitors(
      monitorResult.status === "fulfilled"
        ? monitorResult.value.data?.items ?? []
        : []
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
      <div className="flex min-h-[calc(100vh-64px)] items-center justify-center">
        <Loader2 className="h-5 w-5 animate-spin text-primary" />
      </div>
    );
  }

  return (
    <div className="app-page radar-page" data-testid="dashboard-overview">
      <header className="flex flex-col gap-6 border-b pb-8 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-3xl font-semibold tracking-[-0.045em] text-foreground sm:text-4xl">
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
                更新于 {formatTime(asOf)}
              </span>
            ) : null}
          </div>
        </div>
        {events.length > 0 && canManage ? (
          <Button asChild className="self-start gap-2 px-5">
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
          <AlertTitle>热点事件加载失败</AlertTitle>
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
        <Card className="mt-8 border-dashed">
          <Empty className="min-h-80 border-0">
            <EmptyHeader>
              <EmptyMedia variant="icon">
                <BellRing />
              </EmptyMedia>
              <EmptyTitle>当前窗口内还没有热点事件</EmptyTitle>
              <EmptyDescription>
                {canManage
                  ? "创建并发布监控后，HotKey 会持续聚合来源、识别事件变化并在这里给出解释。"
                  : "工作区发布监控后，HotKey 会持续聚合来源、识别事件变化并在这里给出解释。"}
              </EmptyDescription>
            </EmptyHeader>
            <EmptyContent>
              <Button asChild>
                <Link href="/dashboard/settings">
                  {canManage ? "创建监控" : "查看监控"}
                </Link>
              </Button>
            </EmptyContent>
          </Empty>
        </Card>
      ) : (
        <>
          <Card className="mt-8 overflow-hidden">
            <CardHeader className="flex-row flex-wrap items-center gap-2 space-y-0 border-b px-5 py-4 sm:px-6">
              <CircleDot className="h-4 w-4 text-primary" />
              <h2 className="text-sm font-semibold text-foreground">
                今日事件摘要
              </h2>
              <Badge variant="secondary" className="text-[11px] font-medium">
                Heat v2
              </Badge>
              <p className="ml-auto text-xs text-muted-foreground">
                基于可追溯正文、独立内容家族与事件信号整理
              </p>
            </CardHeader>
            <CardContent className="px-5 py-0 sm:px-6">
              <ol className="divide-y">
                {summaryEvents.map((event, index) => (
                  <li
                    key={event.id ?? index}
                    className="grid gap-3 py-5 sm:grid-cols-[36px_minmax(0,1fr)_auto] sm:items-center"
                  >
                    <span className="text-2xl font-light text-primary">
                      {index + 1}
                    </span>
                    <div className="min-w-0">
                      <p className="font-medium leading-6 text-foreground">
                        <span className="sr-only">摘要：</span>
                        {microEventTitle(event)}
                      </p>
                      <p className="mt-1 line-clamp-2 text-sm leading-6 text-muted-foreground">
                        {microEventSummary(event)}
                      </p>
                    </div>
                    <div className="flex items-center gap-3 text-xs text-muted-foreground sm:justify-end">
					  <span className="text-primary">
                        {event.latest_heat?.independent_lineage_root_count ?? 0} 个独立内容起源
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
              <Card className="overflow-hidden">
                <Table aria-label="重点事件列表" scrollAreaLabel="重点事件列表">
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
                      <TableRow key={event.id ?? index}>
                        <TableCell className="py-3">
                          <Link
                            href={`/dashboard/events?event=${
                              event.id ?? ""
                            }`}
                            className="flex min-w-0 items-center gap-3 text-foreground no-underline"
                          >
                            <EventMark acceleration={event.latest_heat?.acceleration} />
                            <span className="min-w-0">
                              <span className="block truncate text-sm font-medium">
                                {microEventTitle(event)}
                              </span>
                              <span className="mt-1 block text-xs text-muted-foreground">
								{event.latest_heat?.independent_lineage_root_count ?? 0} 个独立内容起源
                              </span>
                            </span>
                          </Link>
                        </TableCell>
                        <TableCell className="text-xs leading-5 text-muted-foreground">
                          {heatReason(event)}
                        </TableCell>
                        <TableCell className="text-xs leading-5 text-muted-foreground">
                          {microEventSummary(event)}
                        </TableCell>
                        <TableCell>
                          <Badge
                            variant="outline"
                            className="gap-1.5 font-normal"
                          >
                            <span className="h-1.5 w-1.5 rounded-full bg-current" />
                            {eventStateLabel(event.status)}
                          </Badge>
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </Card>
            </section>

            <aside>
              <Card>
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
                    href="/dashboard/notifications"
                    className="mt-5 flex items-center justify-between rounded-md bg-muted px-4 py-3 text-sm text-foreground no-underline hover:text-primary"
                  >
                    <span className="inline-flex items-center gap-2">
                      <BellRing className="h-4 w-4" />
                      查看通知
                    </span>
                    <ChevronRight className="h-4 w-4 text-muted-foreground" />
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
