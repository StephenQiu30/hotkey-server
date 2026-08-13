"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import {
  ArrowRight,
  BellRing,
  CalendarDays,
  ChevronRight,
  Loader2,
  Plus,
  RefreshCw,
} from "lucide-react";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
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
import { HotspotCard } from "@/components/dashboard/HotspotCard";
import { getHotspots } from "@/services/hotkey/hotkey-server/hotspots";
import { getMonitors } from "@/services/hotkey/hotkey-server/monitors";
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

export default function DashboardPage() {
  const role = useAuthStore((state) => state.user?.role);
  const canManage = role === UserRole.Admin || role === UserRole.Editor;
  const [hotspots, setHotspots] = useState<HotKeyAPI.HotspotCardResponse[]>([]);
  const [summary, setSummary] = useState<HotKeyAPI.HotspotSummaryResponse>({});
  const [monitors, setMonitors] = useState<HotKeyAPI.MonitorResponse[]>([]);
  const [timeContext, setTimeContext] = useState({
    greeting: "你好",
    today: "",
  });
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string>();

  const load = useCallback(async () => {
    setLoading(true);
    setError(undefined);
    const [hotspotResult, monitorResult] = await Promise.allSettled([
      getHotspots({ limit: 3, sort: "heat" }),
      getMonitors({ limit: 4 }),
    ]);
    if (hotspotResult.status === "rejected") {
      setError(
        hotspotResult.reason instanceof Error
          ? hotspotResult.reason.message
          : "热点加载失败"
      );
      setHotspots([]);
      setSummary({});
    } else {
      setHotspots(hotspotResult.value.data?.items ?? []);
      setSummary(hotspotResult.value.data?.summary ?? {});
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
    setTimeContext(currentTimeContext());
  }, [load]);

  if (loading) {
    return (
      <div className="flex min-h-[calc(100vh-64px)] items-center justify-center">
        <Loader2 className="h-5 w-5 animate-spin text-primary" />
      </div>
    );
  }

  const activeMonitors = monitors.filter(
    (monitor) => monitor.status === "active"
  ).length;

  return (
    <main className="app-page" data-testid="dashboard-overview">
      <header className="flex flex-col gap-6 border-b pb-8 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-3xl font-semibold tracking-[-0.045em] text-foreground sm:text-4xl">
            {timeContext.greeting}，这是今日值得关注的热点
          </h1>
          <p className="mt-3 inline-flex items-center gap-2 text-sm text-muted-foreground">
            <CalendarDays className="h-4 w-4" />
            {timeContext.today || "今日"}
          </p>
        </div>
        {canManage && hotspots.length > 0 ? (
          <Button asChild className="self-start gap-2 px-5">
            <Link href="/dashboard/settings">
              <Plus className="h-4 w-4" />
              创建监控
            </Link>
          </Button>
        ) : null}
      </header>

      {error ? (
        <Alert className="mt-8" variant="destructive">
          <BellRing className="h-4 w-4" />
          <AlertTitle>热点加载失败</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
          <Button
            className="mt-4 gap-2"
            onClick={() => void load()}
            size="sm"
            variant="outline"
          >
            <RefreshCw className="h-4 w-4" />
            重新加载
          </Button>
        </Alert>
      ) : null}

      {!error ? (
        <section
          aria-label="热点概览统计"
          className="mt-8 grid gap-3 sm:grid-cols-2 xl:grid-cols-4"
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
      ) : null}

      {!error && hotspots.length === 0 ? (
        <Card className="mt-8 border-dashed">
          <Empty className="min-h-80 border-0">
            <EmptyHeader>
              <EmptyMedia variant="icon">
                <BellRing />
              </EmptyMedia>
              <EmptyTitle>暂时没有热点</EmptyTitle>
              <EmptyDescription>
                {canManage
                  ? "创建监控并立即扫描，第一批内容会出现在这里。"
                  : "工作区创建监控后，第一批内容会出现在这里。"}
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
      ) : null}

      {hotspots.length ? (
        <div className="mt-8 grid gap-8 xl:grid-cols-[minmax(0,1fr)_320px]">
          <section className="min-w-0">
            <div className="mb-3 flex items-center justify-between">
              <h2 className="text-base font-semibold text-foreground">
                热门热点
              </h2>
              <Link
                aria-label="查看全部热点"
                className="inline-flex items-center gap-1 text-sm text-primary no-underline hover:underline"
                href="/dashboard/contents"
              >
                查看全部
                <ChevronRight className="h-4 w-4" />
              </Link>
            </div>
            <div className="space-y-4">
              {hotspots.map((hotspot, index) => (
                <HotspotCard card={hotspot} key={hotspot.id ?? index} />
              ))}
            </div>
          </section>

          <aside>
            <Card>
              <CardHeader className="flex-row items-center justify-between space-y-0 pb-3">
                <h2 className="text-base font-semibold text-foreground">
                  我的监控
                </h2>
                <Link
                  className="text-xs text-muted-foreground no-underline hover:text-primary"
                  href="/dashboard/settings"
                >
                  管理
                </Link>
              </CardHeader>
              <CardContent>
                <div className="divide-y border-y">
                  {monitors.length ? (
                    monitors.map((monitor) => (
                      <Link
                        className="flex items-center gap-3 py-4 text-sm text-foreground no-underline hover:text-primary"
                        href="/dashboard/settings"
                        key={monitor.id}
                      >
                        <span
                          className={`h-2 w-2 rounded-full ${
                            monitor.status === "active"
                              ? "bg-emerald-500"
                              : "bg-muted-foreground"
                          }`}
                        />
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
                  className="mt-5 flex items-center justify-between rounded-md bg-muted px-4 py-3 text-sm text-foreground no-underline hover:text-primary"
                  href="/dashboard/notifications"
                >
                  <span className="inline-flex items-center gap-2">
                    <BellRing className="h-4 w-4" />
                    查看通知
                  </span>
                  <ChevronRight className="h-4 w-4 text-muted-foreground" />
                </Link>
                <Button
                  asChild
                  className="mt-2 w-full justify-between px-4"
                  variant="ghost"
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
      ) : null}
    </main>
  );
}
