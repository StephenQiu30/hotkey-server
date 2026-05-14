"use client";

import Link from "next/link";
import { useEffect, useMemo, useState } from "react";
import { ArrowRight, Bell, FileText, Radar, Workflow } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { api, CheckRun, formatDate, Hotspot, Notification, Page, Report, statusTone } from "@/lib/api";

export function DashboardClient() {
  const [hotspots, setHotspots] = useState<Hotspot[]>([]);
  const [runs, setRuns] = useState<CheckRun[]>([]);
  const [notifications, setNotifications] = useState<Notification[]>([]);
  const [reports, setReports] = useState<Report[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    Promise.all([
      api<Page<Hotspot>>("/api/hotspots?limit=8&sort=fetched_at_desc"),
      api<Page<CheckRun>>("/api/check-runs?limit=6"),
      api<Page<Notification>>("/api/notifications?limit=6"),
      api<Page<Report>>("/api/reports?limit=5"),
    ])
      .then(([hotspotPage, runPage, notificationPage, reportPage]) => {
        setHotspots(hotspotPage.items);
        setRuns(runPage.items);
        setNotifications(notificationPage.items);
        setReports(reportPage.items);
      })
      .catch((err) => setError(err instanceof Error ? err.message : "加载工作台失败"))
      .finally(() => setLoading(false));
  }, []);

  const activeCount = useMemo(() => hotspots.filter((item) => item.status === "active").length, [hotspots]);
  const filteredCount = useMemo(() => hotspots.filter((item) => item.status === "filtered").length, [hotspots]);
  const lastRun = runs[0];

  if (loading) {
    return (
      <div className="grid gap-4">
        <div className="grid gap-4 md:grid-cols-4">
          {Array.from({ length: 4 }).map((_, index) => <Skeleton className="h-32" key={index} />)}
        </div>
        <Skeleton className="h-96" />
      </div>
    );
  }

  return (
    <div className="grid gap-5">
      {error ? <p className="rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-700" role="alert">{error}</p> : null}
      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
        <MetricCard icon={Radar} label="最近热点" value={hotspots.length.toString()} helper={`${activeCount} active / ${filteredCount} filtered`} />
        <MetricCard icon={Workflow} label="最近任务" value={lastRun?.status || "无记录"} helper={lastRun ? `${lastRun.success_count} 成功 / ${lastRun.failure_count} 失败` : "尚未触发检测"} />
        <MetricCard icon={FileText} label="报告" value={reports.length.toString()} helper={reports[0]?.subject || "暂无报告"} />
        <MetricCard icon={Bell} label="通知" value={notifications.length.toString()} helper={notifications[0]?.status || "暂无通知"} />
      </div>

      <div className="grid gap-5 xl:grid-cols-[1.25fr_.75fr]">
        <Card className="ios-shell-card">
          <CardHeader className="flex flex-row items-center justify-between gap-4">
            <div>
              <CardTitle>最新热点</CardTitle>
              <CardDescription>快速浏览最近抓取和分析出的候选内容。</CardDescription>
            </div>
            <Button asChild size="sm" variant="secondary">
              <Link href="/app/hotspots">查看全部</Link>
            </Button>
          </CardHeader>
          <CardContent className="grid gap-3">
            {hotspots.length === 0 ? <EmptyState text="暂无热点。先配置关键词和来源，然后触发一次检测。" /> : null}
            {hotspots.slice(0, 5).map((item) => (
              <Link
                className="ios-card-muted group rounded-2xl border border-border/70 p-3 transition-colors hover:bg-blue-50/70 focus:outline-none ios-focus-ring"
                href={`/app/hotspots/${item.id}`}
                key={item.id}
              >
                <div className="flex flex-col gap-2 md:flex-row md:items-start md:justify-between">
                  <div className="min-w-0">
                    <p className="truncate font-bold text-slate-950 group-hover:text-primary">{item.title}</p>
                    <p className="mt-1 line-clamp-2 text-sm leading-6 text-muted-foreground">{item.ai_analysis?.summary || item.snippet || item.url}</p>
                  </div>
                  <Badge variant={statusTone(item.status)}>{item.status}</Badge>
                </div>
              </Link>
            ))}
          </CardContent>
        </Card>

        <Card className="ios-shell-card">
          <CardHeader>
            <CardTitle>下一步</CardTitle>
            <CardDescription>常用工作流入口。</CardDescription>
          </CardHeader>
          <CardContent className="grid gap-3">
            {[
              { href: "/app/search", label: "即时搜索一个主题" },
              { href: "/app/runs", label: "手动触发热点检测" },
              { href: "/app/reports", label: "生成日报或周报" },
              { href: "/app/keywords", label: "维护监控关键词" },
              { href: "/app/analytics", label: "查看趋势图" },
            ].map((item) => (
                <Button asChild className="justify-between" key={item.href} variant="secondary">
                <Link href={item.href}>
                  {item.label}
                  <ArrowRight className="h-4 w-4" />
                </Link>
              </Button>
            ))}
          </CardContent>
        </Card>
      </div>
    </div>
  );
}

function MetricCard({ icon: Icon, label, value, helper }: { icon: typeof Radar; label: string; value: string; helper: string }) {
  return (
    <Card className="ios-shell-card">
      <CardHeader>
        <div className="flex items-center justify-between gap-3">
          <CardDescription>{label}</CardDescription>
          <Icon className="h-5 w-5 text-primary" />
        </div>
        <CardTitle className="truncate text-2xl">{value}</CardTitle>
      </CardHeader>
      <CardContent>
        <p className="truncate text-sm text-muted-foreground">{helper}</p>
      </CardContent>
    </Card>
  );
}

function EmptyState({ text }: { text: string }) {
  return <p className="rounded-lg border border-dashed border-border bg-muted/40 p-4 text-sm text-muted-foreground">{text}</p>;
}
