"use client";

import { useEffect, useState } from "react";
import { Activity, Database, FileText, Loader2, Radar, User } from "lucide-react";
import { useAuthStore } from "@/stores/authStore";
import { getMonitors } from "@/services/hotkey/hotkey-server/monitors";
import { getReports } from "@/services/hotkey/hotkey-server/reports";
import { getSourceConnections } from "@/services/hotkey/hotkey-server/sources";
import { getOperationsOverview } from "@/services/hotkey/hotkey-server/operations";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import { UserRole } from "@/lib/domainEnums";

const emptyStats = { monitors: 0, reports: 0, sources: 0, running: 0 };

export default function ProfilePage() {
  const user = useAuthStore((state) => state.user);
  const canViewOperations = user?.role === UserRole.Editor || user?.role === UserRole.Admin;
  const [loading, setLoading] = useState(true);
  const [stats, setStats] = useState(emptyStats);
  const summaryRows = [
    { label: "热点监控", value: stats.monitors, icon: Radar, href: "/dashboard/settings" },
    { label: "来源连接", value: stats.sources, icon: Database, href: "/dashboard/sources" },
    { label: "报告记录", value: stats.reports, icon: FileText, href: "/dashboard/reports" },
    { label: "运行任务", value: stats.running, icon: Activity, href: "/dashboard" },
  ];

  useEffect(() => {
    let cancelled = false;
    const load = async () => {
      try {
        const overviewRequest = canViewOperations
          ? getOperationsOverview().catch(() => undefined)
          : undefined;
        const [monitors, reports, sources] = await Promise.all([
          getMonitors({ limit: 100 }),
          getReports({ limit: 100 }),
          getSourceConnections({ limit: 100 }),
        ]);
        const overview = await overviewRequest;
        if (!cancelled) {
          setStats({
            monitors: monitors.data?.items?.length ?? 0,
            reports: reports.data?.items?.length ?? 0,
            sources: sources.data?.items?.length ?? 0,
            running: overview?.data?.running_jobs ?? 0,
          });
        }
      } finally {
        if (!cancelled) setLoading(false);
      }
    };
    void load();
    return () => { cancelled = true; };
  }, [canViewOperations]);

  return (
    <div className="app-page">
      <div className="border-b border-border pb-6">
        <p className="eyebrow">Account</p>
        <h1 className="mt-2 text-2xl font-semibold">账户信息</h1>
      </div>
      <div className="mt-6 grid gap-5 lg:grid-cols-[320px_minmax(0,1fr)]">
        <aside className="panel h-fit p-6">
          <Avatar className="h-12 w-12">
            <AvatarFallback className="bg-foreground text-background">
              {user?.display_name?.slice(0, 1) || <User />}
            </AvatarFallback>
          </Avatar>
          <h2 className="mt-4 text-lg font-semibold">{user?.display_name || "HotKey 用户"}</h2>
          <p className="mt-1 text-sm text-muted-foreground">{user?.email}</p>
          <div className="mt-6 space-y-3 border-t border-border pt-5 text-xs">
            <div className="flex justify-between"><span className="text-muted-foreground">角色</span><span>{user?.role || "—"}</span></div>
            <div className="flex justify-between"><span className="text-muted-foreground">状态</span><span>{user?.status || "—"}</span></div>
            <div className="flex justify-between"><span className="text-muted-foreground">最近登录</span><span className="mono">{user?.last_login_at ? new Date(user.last_login_at).toLocaleDateString("zh-CN") : "—"}</span></div>
          </div>
        </aside>
        <section>
          {loading ? (
            <div className="panel flex h-64 items-center justify-center"><Loader2 className="animate-spin text-muted-foreground" /></div>
          ) : (
            <>
              <div className="panel divide-y divide-border">
                {summaryRows.map((row) => {
                  const Icon = row.icon;
                  return (
                    <a key={row.label} href={row.href} className="flex items-center gap-4 px-5 py-5 text-foreground no-underline hover:bg-blue-50/60">
                      <Icon className="h-4 w-4 text-muted-foreground" />
                      <span className="text-sm">{row.label}</span>
                      <span className="mono ml-auto text-lg">{row.value}</span>
                    </a>
                  );
                })}
              </div>
              <div className="mt-5 flex flex-wrap gap-2">
                <a href="/dashboard/settings"><Button>管理监控</Button></a>
                <a href="/dashboard/reports"><Button variant="outline">查看报告</Button></a>
                <a href="/dashboard/sources"><Button variant="outline">检查来源</Button></a>
              </div>
            </>
          )}
        </section>
      </div>
    </div>
  );
}
