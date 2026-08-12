"use client";

import type { FormEvent } from "react";
import { useEffect, useState } from "react";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { Bell, ChevronDown, LogOut, Menu, Search, User } from "lucide-react";
import { BrandLogo } from "@/components/BrandLogo";
import ThemeToggle from "@/components/ThemeToggle";
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
import { useNotificationStore } from "@/stores/notificationStore";
import { UserRole } from "@/lib/domainEnums";
import { canAccessRoles } from "@/lib/dashboardAccess";

interface MenuItem {
  path: string;
  name: string;
  icon: React.ReactNode;
  roles?: readonly UserRole[];
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
}: {
  menuItems: MenuItem[];
  adminMenuItems?: MenuItem[];
  title?: string;
}) {
  const pathname = usePathname();
  const router = useRouter();
  const [mobileOpen, setMobileOpen] = useState(false);
  const [query, setQuery] = useState("");
  const user = useAuthStore((state) => state.user);
  const logout = useAuthStore((state) => state.logout);
  const unreadCount = useNotificationStore((state) => state.unreadCount);
  const visibleMenuItems = menuItems.filter((item) =>
    canAccessRoles(item.roles, user?.role),
  );
  const visibleAdminMenuItems = adminMenuItems.filter((item) =>
    canAccessRoles(item.roles, user?.role),
  );

  useEffect(() => {
    setQuery(new URLSearchParams(window.location.search).get("q") ?? "");
  }, [pathname]);

  const handleSearch = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const normalized = query.trim();
    router.push(
      normalized
        ? `/dashboard/events?q=${encodeURIComponent(normalized)}`
        : "/dashboard/events"
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
      className="sticky top-0 z-50 bg-background/90 backdrop-blur-xl"
    >
      <div className="mx-auto flex h-16 max-w-[1440px] min-w-0 items-center gap-3 px-4 sm:px-6 lg:gap-7 lg:px-8">
        <Link
          href="/dashboard"
          className="flex shrink-0 items-center text-base font-bold tracking-tight text-foreground no-underline"
        >
          <BrandLogo title={title} markClassName="h-5 w-5" />
        </Link>
        <NavigationMenu
          aria-label="主导航"
          className="hidden h-full max-w-none flex-none justify-start xl:flex"
        >
          <NavigationMenuList className="h-full gap-1">
            {visibleMenuItems.map((item) => {
              const active = isActivePath(pathname, item.path);
              return (
                <NavigationMenuItem key={item.path}>
                  <NavigationMenuLink asChild active={active}>
                    <Link
                      href={item.path}
                      aria-current={active ? "page" : undefined}
                      className={cn(
                        "relative inline-flex h-16 items-center px-2.5 text-sm font-normal text-muted-foreground no-underline transition-colors hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset",
                        active && "font-medium text-foreground"
                      )}
                    >
                      <span className="hidden">{item.icon}</span>
                      {item.name}
                      {active ? (
                        <span className="absolute inset-x-2.5 bottom-0 h-px bg-foreground" />
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
            aria-label="搜索事件"
            placeholder="搜索事件"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            className="h-9 bg-muted/70 pl-9 shadow-none"
          />
        </form>

        <Button
          asChild
          variant="ghost"
          size="icon"
          className="relative shrink-0"
        >
          <Link
            href="/dashboard/notifications"
            aria-label={
              unreadCount > 0 ? `通知，${unreadCount} 条未读` : "通知"
            }
          >
            <Bell />
            {unreadCount > 0 ? (
              <span className="absolute right-1.5 top-1.5 h-2 w-2 rounded-full border-2 border-background bg-destructive" />
            ) : null}
          </Link>
        </Button>

        <ThemeToggle />

        <Sheet open={mobileOpen} onOpenChange={setMobileOpen}>
          <SheetTrigger asChild>
            <Button
              variant="ghost"
              size="icon"
              className="h-9 w-9 shrink-0 xl:hidden"
              aria-label="打开导航"
            >
              <Menu />
            </Button>
          </SheetTrigger>
          <SheetContent
            side="left"
            className="w-[min(88vw,360px)] overflow-y-auto p-0"
          >
            <SheetHeader className="p-5 text-left">
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
                  aria-label="移动端搜索事件"
                  placeholder="搜索事件"
                  value={query}
                  onChange={(event) => setQuery(event.target.value)}
                />
                <Button type="submit" size="icon" aria-label="搜索">
                  <Search />
                </Button>
              </form>
              <nav aria-label="移动导航" className="space-y-1">
                {visibleMenuItems.map((item) => {
                  const active = isActivePath(pathname, item.path);
                  return (
                    <SheetClose asChild key={item.path}>
                      <Link
                        href={item.path}
                        aria-current={active ? "page" : undefined}
                        className={cn(
                          "flex items-center gap-3 rounded-md px-3 py-2.5 text-sm text-muted-foreground no-underline hover:bg-accent hover:text-accent-foreground",
                          active && "bg-accent font-medium text-foreground"
                        )}
                      >
                        {item.icon}
                        {item.name}
                      </Link>
                    </SheetClose>
                  );
                })}
                {visibleAdminMenuItems.length > 0 ? (
                  <div className="mt-5 space-y-1 pt-3">
                    <p className="px-3 py-1 text-xs font-medium text-muted-foreground">
                      管理
                    </p>
                    {visibleAdminMenuItems.map((item) => {
                      const active = isActivePath(pathname, item.path);
                      return (
                        <SheetClose asChild key={item.path}>
                          <Link
                            href={item.path}
                            aria-current={active ? "page" : undefined}
                            className={cn(
                              "flex items-center gap-3 rounded-md px-3 py-2.5 text-sm text-muted-foreground no-underline hover:bg-accent hover:text-accent-foreground",
                              active && "bg-accent font-medium text-foreground"
                            )}
                          >
                            {item.icon}
                            {item.name}
                          </Link>
                        </SheetClose>
                      );
                    })}
                  </div>
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
            {visibleAdminMenuItems.length > 0 ? (
              <>
                <DropdownMenuSeparator />
                <DropdownMenuLabel className="text-[11px] font-medium text-muted-foreground">
                  工作区管理
                </DropdownMenuLabel>
                {visibleAdminMenuItems.map((item) => (
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
