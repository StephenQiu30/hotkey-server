"use client";

import Link from "next/link";
import { useState } from "react";
import { Menu } from "lucide-react";
import { BrandLogo } from "@/components/BrandLogo";
import { SkipLink } from "@/components/SkipLink";
import { Button } from "@/components/ui/button";
import {
  Sheet,
  SheetClose,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from "@/components/ui/sheet";

const navigation = [
  { href: "#product", label: "产品" },
  { href: "#workflow", label: "工作方式" },
  { href: "#sources", label: "来源与边界" },
  { href: "#start", label: "开始使用" },
];

export function PublicHeader() {
  const [mobileOpen, setMobileOpen] = useState(false);

  return (
    <header className="sticky top-0 z-50 bg-background/88 backdrop-blur-xl shadow-[0_18px_42px_-38px_rgba(0,0,0,.08)]">
      <SkipLink />
      <div className="mx-auto flex h-[72px] max-w-[1440px] items-center px-5 sm:px-8">
        <Link
          href="/"
          aria-label="HotKey 首页"
          className="text-lg font-semibold tracking-[-0.03em] text-foreground no-underline"
        >
          <BrandLogo markClassName="h-[18px] w-[18px]" />
        </Link>
        <nav aria-label="首页导航" className="ml-auto hidden items-center gap-6 md:flex">
          {navigation.map((item) => (
            <a
              key={item.href}
              href={item.href}
              className="text-sm text-muted-foreground no-underline transition-colors hover:text-foreground focus-visible:rounded-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              {item.label}
            </a>
          ))}
          <Link
            href="/login"
            className="text-sm font-medium text-foreground no-underline hover:text-primary"
          >
            登录
          </Link>
          <Button asChild size="sm" className="h-10 rounded-lg px-4">
            <Link href="/register">创建监控</Link>
          </Button>
        </nav>

        <Sheet open={mobileOpen} onOpenChange={setMobileOpen}>
          <SheetTrigger asChild>
            <Button
              variant="ghost"
              size="icon"
              className="ml-auto md:hidden"
              aria-label={mobileOpen ? "关闭首页导航" : "打开首页导航"}
            >
              <Menu />
            </Button>
          </SheetTrigger>
          <SheetContent side="right" className="w-[min(88vw,360px)] p-0">
            <SheetHeader className="p-5 text-left">
              <SheetTitle>HotKey</SheetTitle>
              <SheetDescription>把零散信号，变成可追溯的事件。</SheetDescription>
            </SheetHeader>
            <nav aria-label="移动端首页导航" className="grid gap-1 p-4">
              {navigation.map((item) => (
                <SheetClose asChild key={item.href}>
                  <a
                    href={item.href}
                    className="rounded-md px-3 py-3 text-sm text-foreground no-underline hover:bg-muted"
                  >
                    {item.label}
                  </a>
                </SheetClose>
              ))}
              <SheetClose asChild>
                <Link
                  href="/login"
                  className="rounded-md px-3 py-3 text-sm text-foreground no-underline hover:bg-muted"
                >
                  登录
                </Link>
              </SheetClose>
              <SheetClose asChild>
                <Button asChild className="mt-3 w-full">
                  <Link href="/register">创建监控</Link>
                </Button>
              </SheetClose>
            </nav>
          </SheetContent>
        </Sheet>
      </div>
    </header>
  );
}
