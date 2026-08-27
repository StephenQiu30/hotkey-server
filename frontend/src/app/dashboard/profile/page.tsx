"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import {
  Activity,
  AlertCircle,
  Database,
  Loader2,
  Radar,
  User,
} from "lucide-react";
import { useAuthStore } from "@/stores/authStore";
import { getMonitors } from "@/services/hotkey/hotkey-server/monitors";
import { getSourceConnections } from "@/services/hotkey/hotkey-server/sources";
import { getOperationsOverview } from "@/services/hotkey/hotkey-server/operations";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { PageHeader } from "@/components/dashboard/PageHeader";
import { UserRole } from "@/lib/domainEnums";

const emptyStats = { monitors: 0, sources: 0, running: 0 };

export default function ProfilePage() {
  const user = useAuthStore((state) => state.user);
  const canViewOperations =
    user?.role === UserRole.Editor || user?.role === UserRole.Admin;
  const canManageSources = user?.role === UserRole.Admin;
  const canManage =
    user?.role === UserRole.Analyst || canViewOperations;
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string>();
  const [stats, setStats] = useState(emptyStats);
  const summaryRows = [
    {
      label: "热点监控",
      value: stats.monitors,
      icon: Radar,
      href: "/dashboard/settings",
    },
    ...(canManageSources
      ? [
          {
            label: "来源连接",
            value: stats.sources,
            icon: Database,
            href: "/dashboard/sources",
          },
        ]
      : []),
    {
      label: "运行任务",
      value: stats.running,
      icon: Activity,
      href: "/dashboard",
    },
  ];

  const load = useCallback(async () => {
    setLoading(true);
    setError(undefined);
    try {
      const [monitors, sources, overview] = await Promise.all([
        getMonitors({ limit: 100 }),
        canManageSources
          ? getSourceConnections({ limit: 100 })
          : Promise.resolve(undefined),
        canViewOperations
          ? getOperationsOverview()
          : Promise.resolve(undefined),
      ]);
      setStats({
        monitors: monitors.data?.items?.length ?? 0,
        sources: sources?.data?.items?.length ?? 0,
        running: overview?.data?.running_jobs ?? 0,
      });
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "工作区统计加载失败");
    } finally {
      setLoading(false);
    }
  }, [canManageSources, canViewOperations]);

  useEffect(() => {
    void load();
  }, [load]);

  return (
    <div className="app-page">
      <PageHeader
        eyebrow="Account"
        title="账户信息"
        description="查看当前账户、权限角色与工作区资源概览。"
      />
      <div className="mt-6 grid gap-5 lg:grid-cols-[320px_minmax(0,1fr)]">
        <Card className="h-fit">
          <CardContent>
            <Avatar className="h-12 w-12">
              <AvatarFallback className="bg-foreground text-background">
                {user?.display_name?.slice(0, 1) || <User />}
              </AvatarFallback>
            </Avatar>
            <h2 className="mt-4 text-lg font-semibold">
              {user?.display_name || "HotKey 用户"}
            </h2>
            <p className="mt-1 text-sm text-muted-foreground">{user?.email}</p>
            <div className="mt-6 space-y-3 border-t border-border pt-5 text-xs">
              <div className="flex justify-between">
                <span className="text-muted-foreground">角色</span>
                <span>{user?.role || "—"}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">状态</span>
                <span>{user?.status || "—"}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">最近登录</span>
                <span className="mono">
                  {user?.last_login_at
                    ? new Date(user.last_login_at).toLocaleDateString("zh-CN")
                    : "—"}
                </span>
              </div>
            </div>
          </CardContent>
        </Card>
        <section>
          {loading ? (
            <Card>
              <CardContent className="flex h-64 items-center justify-center">
                <Loader2 className="animate-spin text-muted-foreground" />
              </CardContent>
            </Card>
          ) : error ? (
            <Alert variant="destructive">
              <AlertCircle />
              <AlertTitle>无法加载工作区统计</AlertTitle>
              <AlertDescription className="flex flex-wrap items-center justify-between gap-3">
                <span>{error}</span>
                <Button size="sm" variant="outline" onClick={() => void load()}>
                  重试
                </Button>
              </AlertDescription>
            </Alert>
          ) : (
            <>
              <Card className="gap-0 divide-y divide-border py-0">
                {summaryRows.map((row) => {
                  const Icon = row.icon;
                  return (
                    <Link
                      key={row.label}
                      href={row.href}
                      className="flex items-center gap-4 px-5 py-5 text-foreground no-underline hover:bg-muted/60"
                    >
                      <Icon className="h-4 w-4 text-muted-foreground" />
                      <span className="text-sm">{row.label}</span>
                      <span className="mono ml-auto text-lg">{row.value}</span>
                    </Link>
                  );
                })}
              </Card>
              <div className="mt-5 flex flex-wrap gap-2">
                <Button asChild>
                  <Link href="/dashboard/settings">
                    {canManage ? "管理监控" : "查看监控"}
                  </Link>
                </Button>
                {canManageSources ? (
                  <Button asChild variant="outline">
                    <Link href="/dashboard/sources">检查来源</Link>
                  </Button>
                ) : null}
              </div>
            </>
          )}
        </section>
      </div>
    </div>
  );
}
