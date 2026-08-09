"use client";

import Link from "next/link";
import { Menu } from "lucide-react";
import { BrandLogo } from "@/components/BrandLogo";
import ThemeToggle from "@/components/ThemeToggle";
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
  { href: "#workflow", label: "解决方案" },
  { href: "#sources", label: "资源" },
  { href: "#pricing", label: "定价" },
];

export function PublicHeader() {
  return (
    <header className="sticky top-0 z-50 bg-background/95 backdrop-blur-sm">
      <div className="mx-auto flex h-[72px] max-w-[1440px] items-center px-5 sm:px-8">
        <Link
          href="/"
          aria-label="HotKey 首页"
          className="text-lg font-semibold tracking-[-0.03em] text-foreground no-underline"
        >
          <BrandLogo markClassName="h-[18px] w-[18px]" />
        </Link>
        <span className="ml-3 hidden rounded-full bg-muted px-2.5 py-1 text-[10px] font-medium uppercase tracking-[0.13em] text-muted-foreground sm:inline-flex">
          Intelligence
        </span>

        <nav aria-label="首页导航" className="ml-auto hidden items-center gap-7 md:flex">
          {navigation.map((item) => (
            <a
              key={item.href}
              href={item.href}
              className="text-sm text-foreground no-underline transition-opacity hover:opacity-55 focus-visible:rounded-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              {item.label}
            </a>
          ))}
          <Link
            href="/login"
            className="text-sm text-foreground no-underline transition-opacity hover:opacity-55 focus-visible:rounded-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            登录
          </Link>
          <Button asChild size="sm" className="h-10 rounded-lg px-4">
            <Link href="/register">开始使用</Link>
          </Button>
        </nav>

        <ThemeToggle className="ml-auto md:ml-2" />

        <Sheet>
          <SheetTrigger asChild>
            <Button
              variant="ghost"
              size="icon"
              className="md:hidden"
              aria-label="打开首页导航"
            >
              <Menu />
            </Button>
          </SheetTrigger>
          <SheetContent side="right" className="w-[min(88vw,360px)] p-0">
            <SheetHeader className="p-5 text-left">
              <SheetTitle>HotKey</SheetTitle>
              <SheetDescription>重要变化，先形成判断。</SheetDescription>
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
                  <Link href="/register">开始使用</Link>
                </Button>
              </SheetClose>
            </nav>
          </SheetContent>
        </Sheet>
      </div>
    </header>
  );
}
