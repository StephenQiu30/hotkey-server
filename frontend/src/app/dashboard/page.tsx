"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import Link from "next/link";
import { useGSAP } from "@gsap/react";
import gsap from "gsap";
import {
  Activity,
  ArrowRight,
  BellRing,
  CalendarDays,
  ChevronRight,
  CircleAlert,
  Loader2,
  Plus,
  Radar,
  RefreshCw,
  ScanLine,
} from "lucide-react";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import { OverviewMetric } from "@/components/dashboard/OverviewMetric";
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty";
import { HotspotCard } from "@/components/dashboard/HotspotCard";
import { getHotspots } from "@/services/hotkey/hotkey-server/hotspots";
import { getMonitors } from "@/services/hotkey/hotkey-server/monitors";
import { UserRole } from "@/lib/domainEnums";
import { useAuthStore } from "@/stores/authStore";
import { PageShell } from "@/layouts/PageShell";

gsap.registerPlugin(useGSAP);

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

export default function DashboardPage() {
  const pageRef = useRef<HTMLDivElement>(null);
  const role = useAuthStore((state) => state.user?.role);
  const canManage =
    role === UserRole.Analyst ||
    role === UserRole.Editor ||
    role === UserRole.Admin;
  const [hotspots, setHotspots] = useState<HotKeyAPI.HotspotCardResponse[]>([]);
  const [summary, setSummary] = useState<HotKeyAPI.HotspotSummaryResponse>({});
  const [monitors, setMonitors] = useState<HotKeyAPI.MonitorResponse[]>([]);
  const [timeContext, setTimeContext] = useState({ greeting: "你好", today: "" });
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string>();
  const [monitorError, setMonitorError] = useState<string>();

  const load = useCallback(async () => {
    setLoading(true);
    setError(undefined);
    setMonitorError(undefined);
    const [hotspotResult, monitorResult] = await Promise.allSettled([
      getHotspots({ limit: 3, sort: "heat" }),
      getMonitors({ limit: 4 }),
    ]);
    if (hotspotResult.status === "rejected") {
      setError(
        hotspotResult.reason instanceof Error
          ? hotspotResult.reason.message
          : "信号加载失败",
      );
      setHotspots([]);
      setSummary({});
    } else {
      setHotspots(hotspotResult.value.data?.items ?? []);
      setSummary(hotspotResult.value.data?.summary ?? {});
    }
    if (monitorResult.status === "fulfilled") {
      setMonitors(monitorResult.value.data?.items ?? []);
    } else {
      setMonitors([]);
      setMonitorError(
        monitorResult.reason instanceof Error
          ? monitorResult.reason.message
          : "监控状态暂时不可用",
      );
    }
    setLoading(false);
  }, []);

  useEffect(() => {
    void load();
    setTimeContext(currentTimeContext());
  }, [load]);

  useGSAP(
    () => {
      if (loading) return;
      const media = gsap.matchMedia();
      media.add("(prefers-reduced-motion: no-preference)", () => {
        const intro = gsap.timeline({
          defaults: { duration: 0.55, ease: "power3.out" },
        });
        intro
          .from(".dashboard-intro", { autoAlpha: 0, y: 18 })
          .from(
            ".overview-metric",
            { autoAlpha: 0, y: 16, stagger: 0.07, duration: 0.42 },
            "<0.15",
          )
          .from(
            ".hotspot-card",
            { autoAlpha: 0, y: 18, stagger: 0.08, duration: 0.48 },
            "<0.12",
          )
          .from(".work-queue", { autoAlpha: 0, x: 14, duration: 0.45 }, "<");
      });
      return () => media.revert();
    },
    {
      dependencies: [loading, hotspots.length],
      revertOnUpdate: true,
      scope: pageRef,
    },
  );

  if (loading) {
    return (
      <PageShell ref={pageRef} align="center" aria-live="polite" role="status">
        <Loader2 aria-hidden="true" className="h-5 w-5 animate-spin text-primary" />
        <span className="sr-only">正在加载今日态势</span>
      </PageShell>
    );
  }

  const activeMonitors = monitors.filter((monitor) => monitor.status === "active").length;
  const observedSignals = summary.total ?? 0;
  const urgentSignals = summary.urgent ?? 0;

  const metrics = [
    {
      label: "已观察信号",
      description: "当前进入观察范围",
      value: observedSignals,
      icon: <ScanLine />,
      tone: "signal" as const,
    },
    {
      label: "今日新增",
      description: "本日首次发现",
      value: summary.today ?? 0,
      icon: <Activity />,
      tone: "blue" as const,
    },
    {
      label: "紧急待复核",
      description: "需要优先形成判断",
      value: urgentSignals,
      icon: <CircleAlert />,
      tone: "heat" as const,
    },
    {
      label: "运行中监控",
      description: "持续接收公开信号",
      value: activeMonitors,
      icon: <Radar />,
      tone: "success" as const,
    },
  ];

  return (
    <PageShell ref={pageRef} data-testid="dashboard-overview" className="pt-8 sm:pt-10">
      <section className="dashboard-intro relative isolate overflow-hidden rounded-2xl bg-secondary/75 px-6 py-7 text-foreground [box-shadow:var(--shadow-card)] sm:px-8 sm:py-9 lg:grid lg:grid-cols-[minmax(0,1fr)_340px] lg:items-end lg:gap-10">
        <div aria-hidden="true" className="absolute -right-20 -top-28 -z-10 h-72 w-72 rounded-full bg-foreground/[.025] blur-3xl" />
        <div>
          <p className="mono inline-flex items-center gap-2 text-[11px] font-semibold uppercase tracking-[.12em] text-muted-foreground">
            <span aria-hidden="true" className="h-1.5 w-1.5 rounded-full bg-foreground/55" />
            Live situation
          </p>
          <h1 className="mt-4 text-balance text-3xl font-semibold leading-[1.05] tracking-[-0.045em] sm:text-5xl">
            {timeContext.greeting}，今日信号态势
          </h1>
          <p className="mt-4 inline-flex items-center gap-2 text-sm text-muted-foreground">
            <CalendarDays aria-hidden="true" className="h-4 w-4" />
            {timeContext.today || "今日"} · 先看变化，再回到证据
          </p>
          <div className="mt-6 flex flex-wrap gap-2 text-xs">
            <span className="rounded-full bg-background px-3 py-1.5 text-foreground">{observedSignals} 条信号进入观察</span>
            <span className="rounded-full bg-muted px-3 py-1.5 text-muted-foreground">{urgentSignals} 条紧急信号待复核</span>
          </div>
        </div>
        <div className="mt-8 rounded-xl bg-background/82 p-4 backdrop-blur-sm [box-shadow:var(--shadow-card)] lg:mt-0">
          <div className="flex items-center justify-between gap-4">
            <div>
              <p className="text-xs text-muted-foreground">监控运行状态</p>
              <p className="mt-1 text-lg font-semibold">
                {monitorError ? "监控状态暂时不可用" : `${activeMonitors} 个任务在线`}
              </p>
            </div>
            <span className="inline-flex h-10 w-10 items-center justify-center rounded-lg bg-muted text-foreground">
              <Radar aria-hidden="true" className="h-5 w-5" />
            </span>
          </div>
          {!monitorError ? (
            <div className="mt-4 h-1.5 overflow-hidden rounded-full bg-muted">
              <div className="h-full w-3/4 rounded-full bg-foreground/55" />
            </div>
          ) : (
            <p className="mt-3 text-xs text-muted-foreground">{monitorError}</p>
          )}
          {canManage && hotspots.length > 0 ? (
            <Button asChild className="mt-5 w-full justify-between">
              <Link href="/dashboard/settings">
                创建监控 <Plus />
              </Link>
            </Button>
          ) : null}
        </div>
      </section>

      {error ? (
        <Alert className="mt-6" variant="destructive">
          <BellRing className="h-4 w-4" />
          <AlertTitle>信号加载失败</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
          <Button className="mt-4 gap-2" onClick={() => void load()} size="sm" variant="outline">
            <RefreshCw className="h-4 w-4" />
            重新加载
          </Button>
        </Alert>
      ) : null}

      {!error ? (
        <section aria-label="今日态势统计" className="mt-5 grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
          {metrics.map((metric) => (
            <OverviewMetric key={metric.label} {...metric} />
          ))}
        </section>
      ) : null}

      {!error && hotspots.length === 0 ? (
        <Card className="mt-6" variant="subtle">
          <Empty className="min-h-80">
            <EmptyHeader>
              <EmptyMedia variant="icon"><Radar /></EmptyMedia>
              <EmptyTitle>还没有进入观察的信号</EmptyTitle>
              <EmptyDescription>
                {canManage
                  ? "建立监控并完成首次扫描后，值得关注的变化会出现在这里。"
                  : "工作区建立监控并完成首次扫描后，值得关注的变化会出现在这里。"}
              </EmptyDescription>
            </EmptyHeader>
            <EmptyContent>
              <Button asChild>
                <Link href="/dashboard/settings">{canManage ? "创建监控" : "查看监控"}</Link>
              </Button>
            </EmptyContent>
          </Empty>
        </Card>
      ) : null}

      {hotspots.length ? (
        <div className="mt-8 grid gap-8 xl:grid-cols-[minmax(0,1fr)_340px]">
          <section className="min-w-0">
            <div className="mb-4 flex items-end justify-between gap-4">
              <div>
                <p className="mono text-[10px] font-semibold uppercase tracking-[.12em] text-signal">Priority signals</p>
                <h2 className="mt-1.5 text-xl font-semibold text-foreground">正在升温</h2>
              </div>
              <Link aria-label="查看全部信号" className="inline-flex items-center gap-1 text-sm font-semibold text-primary no-underline hover:opacity-75" href="/dashboard/contents">
                查看全部 <ChevronRight className="h-4 w-4" />
              </Link>
            </div>
            <div className="space-y-4">
              {hotspots.map((hotspot, index) => (
                <HotspotCard
                  card={hotspot}
                  headingLevel="h3"
                  key={hotspot.id ?? index}
                />
              ))}
            </div>
          </section>

          <aside className="work-queue">
            <Card className="sticky top-4 overflow-hidden">
              <CardHeader className="bg-secondary/35 pb-4">
                <div className="flex items-center justify-between gap-3">
                  <div>
                    <p className="mono text-[10px] font-semibold uppercase tracking-[.12em] text-signal">Today</p>
                    <h2 className="mt-1.5 text-base font-semibold text-foreground">今日待处理</h2>
                  </div>
                  <span className="inline-flex h-9 w-9 items-center justify-center rounded-lg bg-muted text-foreground">
                    <CircleAlert aria-hidden="true" className="h-4 w-4" />
                  </span>
                </div>
              </CardHeader>
              <CardContent className="p-5">
                <div className="grid grid-cols-2 gap-2">
                  <div className="rounded-xl bg-muted p-3">
                    <p className="text-[10px] text-muted-foreground">紧急待复核</p>
                    <p className="mono mt-2 text-2xl font-semibold text-foreground">{urgentSignals}</p>
                  </div>
                  <div className="rounded-xl bg-muted p-3">
                    <p className="text-[10px] text-muted-foreground">运行中监控</p>
                    <p className="mono mt-2 text-2xl font-semibold text-foreground">
                      {monitorError ? "—" : activeMonitors}
                    </p>
                  </div>
                </div>

                <div className="mt-6 flex items-center justify-between">
                  <h3 className="text-sm font-semibold">监控任务</h3>
                  <Link className="text-xs text-muted-foreground no-underline hover:text-primary" href="/dashboard/settings">管理</Link>
                </div>
                <div className="mt-2 space-y-1">
                  {monitorError ? (
                    <p className="py-5 text-sm text-muted-foreground">监控状态暂时不可用</p>
                  ) : monitors.length ? (
                    monitors.map((monitor) => (
                      <Link className="-mx-2 flex items-center gap-3 rounded-lg px-2 py-3.5 text-sm text-foreground no-underline hover:bg-secondary/70 hover:text-primary" href="/dashboard/settings" key={monitor.id}>
                        <span aria-hidden="true" className={`h-2 w-2 rounded-full ${monitor.status === "active" ? "bg-foreground/70" : "bg-muted-foreground"}`} />
                        <span className="sr-only">{monitor.status === "active" ? "启用" : "未启用"}</span>
                        <span className="min-w-0 flex-1 truncate">{monitor.name || `监控 #${monitor.id}`}</span>
                        <ChevronRight className="h-4 w-4 text-muted-foreground" />
                      </Link>
                    ))
                  ) : (
                    <p className="py-5 text-sm text-muted-foreground">还没有可用监控</p>
                  )}
                </div>

                <Link className="mt-4 flex items-center justify-between rounded-xl bg-muted px-4 py-3 text-sm text-foreground no-underline hover:bg-accent" href="/dashboard/notifications">
                  <span className="inline-flex items-center gap-2"><BellRing className="h-4 w-4 text-muted-foreground" />查看通知</span>
                  <ChevronRight className="h-4 w-4 text-muted-foreground" />
                </Link>
                <Button asChild className="mt-2 w-full justify-between px-4" variant="ghost">
                  <Link href="/dashboard/events">
                    进入事件雷达 <ArrowRight className="h-4 w-4" />
                  </Link>
                </Button>
              </CardContent>
            </Card>
          </aside>
        </div>
      ) : null}
    </PageShell>
  );
}
