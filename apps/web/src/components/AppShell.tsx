"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useEffect, useMemo, useState, type ReactNode } from "react";
import {
  Bell,
  FileText,
  Gauge,
  ListFilter,
  Radar,
  Search,
  Settings,
  Sparkles,
  TrendingUp,
  ChevronRight,
  Tags,
  Workflow,
  LogOut,
  User2,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { cn } from "@/lib/utils";
import { clearAuthToken, getCurrentUser, type AuthUser } from "@/lib/api";

const navItems = [
  { href: "/app", label: "总览", icon: Gauge },
  { href: "/app/hotspots", label: "热点", icon: Radar },
  { href: "/app/search", label: "搜索", icon: Search },
  { href: "/app/keywords", label: "关键词", icon: Tags },
  { href: "/app/sources", label: "来源", icon: ListFilter },
  { href: "/app/runs", label: "任务", icon: Workflow },
  { href: "/app/reports", label: "报告", icon: FileText },
  { href: "/app/notifications", label: "通知", icon: Bell },
  { href: "/app/analytics", label: "趋势", icon: TrendingUp },
  { href: "/app/settings", label: "设置", icon: Settings },
];

export function AppShell({ title, description, actions, children }: { title: string; description?: string; actions?: ReactNode; children: ReactNode }) {
  const pathname = usePathname();
  const router = useRouter();
  const [me, setMe] = useState<AuthUser | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let isActive = true;
    getCurrentUser()
      .then((user) => {
        if (!isActive) return;
        setMe(user);
      })
      .catch(() => {
        if (!isActive) return;
        clearAuthToken();
        router.replace("/login");
      })
      .finally(() => {
        if (!isActive) return;
        setLoading(false);
      });
    return () => {
      isActive = false;
    };
  }, [router]);

  const displayName = useMemo(() => {
    if (!me?.github_name) {
      return me?.github_login || "用户";
    }
    return me.github_name;
  }, [me]);

  if (loading) {
    return (
      <main className="min-h-screen bg-background text-foreground">
        <section className="mx-auto w-full max-w-7xl px-4 py-6 md:px-6 lg:px-8">
          <div className="flex min-h-screen flex-col gap-4">
            <Skeleton className="h-16 rounded-xl" />
            <Skeleton className="h-12 rounded-lg" />
            <Skeleton className="h-64 flex-1 rounded-xl" />
          </div>
        </section>
      </main>
    );
  }

  return (
    <main className="min-h-screen bg-background text-foreground">
      <header className="sticky top-0 z-30 border-b border-border/70 bg-white/85 backdrop-blur-xl">
        <div className="mx-auto flex w-full max-w-7xl flex-col gap-3 px-4 py-3 md:px-6 lg:px-8">
          <div className="flex min-w-0 items-center justify-between gap-3">
            <Link
              className="ios-reveal ios-shell-card flex min-h-11 min-w-0 items-center gap-3 rounded-2xl border border-border/80 bg-white px-2 py-2 transition hover:bg-muted/60 focus-visible:outline-none"
              href="/app"
            >
              <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-primary text-primary-foreground shadow-sm">
                <Sparkles className="h-5 w-5" />
              </span>
              <span className="min-w-0">
                <span className="block truncate text-sm font-extrabold">AI Hotspot Radar</span>
                <span className="block truncate text-xs font-medium text-muted-foreground">Private SaaS Console</span>
              </span>
            </Link>
            <div className="flex items-center gap-2">
              <span className="hidden items-center gap-2 rounded-full border border-border bg-slate-50 px-3 py-2 text-sm text-muted-foreground md:flex">
                <User2 className="h-4 w-4" />
                <span className="max-w-32 truncate">{displayName}</span>
              </span>
              <Button
                className="shrink-0"
                size="sm"
                variant="secondary"
                onClick={() => {
                  clearAuthToken();
                  router.replace("/login");
                }}
              >
                <LogOut className="mr-1 h-4 w-4" />
                退出
              </Button>
              <Button asChild className="shrink-0" size="sm" variant="secondary">
                <Link href="/">官网</Link>
              </Button>
            </div>
          </div>
          <nav aria-label="主导航" className="flex gap-2 overflow-x-auto pb-1">
            {navItems.map((item) => {
              const Icon = item.icon;
              const active = pathname === item.href || (item.href !== "/app" && pathname.startsWith(`${item.href}/`));
              return (
                <Link
                  className={cn(
                    "ios-reveal ios-shell-card flex min-h-10 shrink-0 items-center gap-2 rounded-full border border-transparent bg-white/70 px-3 py-2 text-sm font-semibold text-muted-foreground transition-colors hover:bg-white hover:text-foreground focus-visible:outline-none",
                    active && "border-border bg-primary/12 text-primary"
                  )}
                  href={item.href}
                  key={item.href}
                >
                  <Icon className="h-4 w-4 shrink-0" />
                  <span>{item.label}</span>
                  {active ? <ChevronRight className="h-3.5 w-3.5 text-primary" /> : null}
                </Link>
              );
            })}
          </nav>
        </div>
      </header>
      <section className="mx-auto w-full max-w-7xl px-4 py-6 md:px-6 lg:px-8">
        <header className="mb-6 flex min-w-0 flex-col gap-4 border-b border-border pb-5 md:flex-row md:items-center md:justify-between">
          <div>
            <p className="text-xs font-medium uppercase tracking-[0.26em] text-muted-foreground">控制面板</p>
            <h1 className="mt-1 text-2xl font-extrabold tracking-tight text-foreground md:text-3xl">{title}</h1>
            {description ? <p className="mt-2 max-w-3xl text-sm leading-6 text-muted-foreground">{description}</p> : null}
          </div>
          {actions ? <div className="flex flex-wrap items-center gap-2">{actions}</div> : null}
        </header>
        <div className="ios-reveal">
          {children}
        </div>
      </section>
    </main>
  );
}
