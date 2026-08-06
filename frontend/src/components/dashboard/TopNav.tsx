"use client";

import type { FormEvent } from "react";
import { useState } from "react";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import {
  Bell,
  ChevronDown,
  LogOut,
  Menu,
  Search,
  Settings2,
  User,
} from "lucide-react";
import { BrandLogo } from "@/components/BrandLogo";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  NavigationMenu,
  NavigationMenuItem,
  NavigationMenuLink,
  NavigationMenuList,
} from "@/components/ui/navigation-menu";
import {
  Sheet,
  SheetClose,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from "@/components/ui/sheet";
import { cn } from "@/lib/utils";
import { useAuthStore } from "@/stores/authStore";
import { UserRole } from "@/lib/domainEnums";

interface MenuItem {
  path: string;
  name: string;
  icon: React.ReactNode;
}

function isActivePath(pathname: string, path: string) {
  return path === "/dashboard"
    ? pathname === path
    : pathname === path || pathname.startsWith(`${path}/`);
}

export default function TopNav({
  menuItems,
  adminMenuItems = [],
  title = "HotKey",
  alertCount = 0,
}: {
  menuItems: MenuItem[];
  adminMenuItems?: MenuItem[];
  title?: string;
  alertCount?: number;
}) {
  const pathname = usePathname();
  const router = useRouter();
  const [mobileOpen, setMobileOpen] = useState(false);
  const [query, setQuery] = useState("");
  const user = useAuthStore((state) => state.user);
  const logout = useAuthStore((state) => state.logout);
  const canManage =
    user?.role === UserRole.Admin || user?.role === UserRole.Editor;

  const handleSearch = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const normalized = query.trim();
    router.push(
      normalized
        ? `/dashboard/events?q=${encodeURIComponent(normalized)}`
        : "/dashboard/events",
    );
    setMobileOpen(false);
  };

  const handleLogout = async () => {
    await logout();
    window.location.href = "/login";
  };

  return (
    <header
      data-top-nav
      className="sticky top-0 z-50 border-b bg-background/95 backdrop-blur-xl"
    >
      <div className="mx-auto flex h-[72px] max-w-[1536px] min-w-0 items-center gap-3 px-4 sm:px-6 lg:gap-6 lg:px-8">
        <Link
          href="/dashboard"
          className="flex shrink-0 items-center text-base font-bold tracking-tight text-foreground no-underline"
        >
          <BrandLogo title={title} markClassName="h-5 w-5" />
        </Link>

        <NavigationMenu
          aria-label="主导航"
          className="hidden h-full max-w-none flex-none justify-start lg:flex"
        >
          <NavigationMenuList className="h-full gap-1">
            {menuItems.map((item) => {
              const active = isActivePath(pathname, item.path);
              return (
                <NavigationMenuItem key={item.path}>
                  <NavigationMenuLink asChild active={active}>
                    <Link
                      href={item.path}
                      aria-current={active ? "page" : undefined}
                      className={cn(
                        "relative inline-flex h-[72px] items-center px-3 text-sm font-medium text-muted-foreground no-underline transition-colors hover:text-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring focus-visible:ring-inset",
                        active && "text-primary",
                      )}
                    >
                      <span className="hidden">{item.icon}</span>
                      {item.name}
                      {active ? (
                        <span className="absolute inset-x-3 bottom-0 h-0.5 rounded-full bg-primary" />
                      ) : null}
                    </Link>
                  </NavigationMenuLink>
                </NavigationMenuItem>
              );
            })}
          </NavigationMenuList>
        </NavigationMenu>

        <form
          role="search"
          onSubmit={handleSearch}
          className="relative ml-auto hidden min-w-0 flex-1 md:block md:max-w-[320px]"
        >
          <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            type="search"
            aria-label="搜索事件或监控"
            placeholder="搜索事件或监控"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            className="h-9 bg-muted/40 pl-9 shadow-none"
          />
        </form>

        <Button
          asChild
          variant="ghost"
          size="icon"
          className="relative shrink-0"
        >
          <Link
            href="/dashboard/alerts"
            aria-label={
              alertCount > 0 ? `告警，${alertCount} 条待处理` : "告警"
            }
          >
            <Bell />
            {alertCount > 0 ? (
              <span className="absolute right-1.5 top-1.5 h-2 w-2 rounded-full border-2 border-background bg-primary" />
            ) : null}
          </Link>
        </Button>

        <Sheet open={mobileOpen} onOpenChange={setMobileOpen}>
          <SheetTrigger asChild>
            <Button
              variant="ghost"
              size="icon"
              className="h-9 w-9 shrink-0 lg:hidden"
              aria-label="打开导航"
            >
              <Menu />
            </Button>
          </SheetTrigger>
          <SheetContent side="left" className="w-[min(88vw,360px)] p-0">
            <SheetHeader className="border-b p-5 text-left">
              <SheetTitle>工作区导航</SheetTitle>
              <SheetDescription>浏览热点监控与分析功能。</SheetDescription>
            </SheetHeader>
            <div className="p-4">
              <form
                role="search"
                onSubmit={handleSearch}
                className="mb-4 flex gap-2"
              >
                <Input
                  type="search"
                  aria-label="移动端搜索事件或监控"
                  placeholder="搜索事件或监控"
                  value={query}
                  onChange={(event) => setQuery(event.target.value)}
                />
                <Button type="submit" size="icon" aria-label="搜索">
                  <Search />
                </Button>
              </form>
              <nav aria-label="移动导航" className="space-y-1">
                {menuItems.map((item) => {
                  const active = isActivePath(pathname, item.path);
                  return (
                    <SheetClose asChild key={item.path}>
                      <Link
                        href={item.path}
                        aria-current={active ? "page" : undefined}
                        className={cn(
                          "flex items-center gap-3 rounded-md px-3 py-2.5 text-sm text-muted-foreground no-underline hover:bg-accent hover:text-accent-foreground",
                          active && "bg-accent font-medium text-foreground",
                        )}
                      >
                        {item.icon}
                        {item.name}
                      </Link>
                    </SheetClose>
                  );
                })}
                {canManage ? (
                  <SheetClose asChild>
                    <Link
                      href="/dashboard/sources"
                      className="mt-3 flex items-center gap-3 border-t px-3 pt-4 text-sm text-muted-foreground no-underline"
                    >
                      <Settings2 className="h-4 w-4" />
                      工作区管理
                    </Link>
                  </SheetClose>
                ) : null}
              </nav>
            </div>
          </SheetContent>
        </Sheet>

        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              variant="ghost"
              aria-label="账户菜单"
              data-nav-menu-trigger="account"
              className="h-9 gap-1.5 rounded-full p-0.5 text-xs text-muted-foreground"
            >
              <Avatar className="h-8 w-8">
                <AvatarFallback className="bg-primary text-[11px] font-semibold text-primary-foreground">
                  {user?.display_name?.slice(0, 1)?.toUpperCase() || (
                    <User className="h-3.5 w-3.5" />
                  )}
                </AvatarFallback>
              </Avatar>
              <ChevronDown className="hidden h-3.5 w-3.5 sm:block" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-56">
            <DropdownMenuLabel className="font-normal">
              <p className="truncate text-sm font-medium text-foreground">
                {user?.display_name || "账户"}
              </p>
              <p className="mt-0.5 truncate text-xs text-muted-foreground">
                {user?.email}
              </p>
            </DropdownMenuLabel>
            <DropdownMenuSeparator />
            <DropdownMenuItem asChild>
              <Link href="/dashboard/profile">
                <User className="mr-2 h-4 w-4" />
                账户信息
              </Link>
            </DropdownMenuItem>
            {canManage && adminMenuItems.length > 0 ? (
              <>
                <DropdownMenuSeparator />
                <DropdownMenuLabel className="text-[11px] font-medium text-muted-foreground">
                  工作区管理
                </DropdownMenuLabel>
                {adminMenuItems.map((item) => (
                  <DropdownMenuItem key={item.path} asChild>
                    <Link href={item.path}>
                      <span className="mr-2">{item.icon}</span>
                      {item.name}
                    </Link>
                  </DropdownMenuItem>
                ))}
              </>
            ) : null}
            <DropdownMenuSeparator />
            <DropdownMenuItem
              onClick={handleLogout}
              className="text-destructive"
            >
              <LogOut className="mr-2 h-4 w-4" />
              退出登录
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </header>
  );
}
