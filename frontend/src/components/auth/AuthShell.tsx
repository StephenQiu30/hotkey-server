"use client";

import Image from "next/image";
import Link from "next/link";
import { BrandLogo } from "@/components/BrandLogo";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";

export default function AuthShell({
  title,
  subtitle,
  children,
}: {
  title: string;
  subtitle: string;
  children: React.ReactNode;
}) {
  return (
    <div className="min-h-screen bg-muted/30 px-5 sm:px-8">
      <div className="mx-auto grid min-h-screen w-full max-w-[1440px] lg:grid-cols-[minmax(0,1fr)_480px]">
        <aside
          aria-label="HotKey 品牌介绍"
          className="relative hidden min-h-screen overflow-hidden p-8 lg:flex lg:flex-col xl:p-10"
        >
          <Link
            href="/"
            className="relative z-10 text-base font-semibold text-foreground no-underline"
          >
            <BrandLogo markClassName="h-[18px] w-[18px]" />
          </Link>

          <div className="relative z-10 my-auto max-w-[min(54%,620px)]">
            <p className="text-xs text-muted-foreground">Editorial intelligence</p>
            <h2 className="mt-6 text-[clamp(3rem,5vw,5.75rem)] font-normal leading-[0.98] tracking-[-0.065em]">
              重要变化，
              <br />
              先形成判断
            </h2>
            <p className="mt-7 text-sm leading-7 text-muted-foreground">
              持续监测公开信号，识别真正重要的变化，让每一次判断都建立在证据之上。
            </p>
          </div>

          <div className="pointer-events-none absolute bottom-[8%] right-[-3%] h-[42%] w-[54%] opacity-70 xl:right-[2%]">
            <Image
              src="/images/hotkey-signal-radar.png"
              alt=""
              fill
              priority
              sizes="50vw"
              className="object-contain object-right-bottom dark:invert"
            />
          </div>

          <p className="relative z-10 text-xs text-muted-foreground">© 2026 HotKey</p>
        </aside>

        <main className="flex min-h-screen items-center justify-center py-10 sm:px-2 sm:py-12 lg:px-10">
          <div className="w-full max-w-[400px]">
            <Link
              href="/"
              className="mb-10 inline-flex text-base font-semibold text-foreground no-underline sm:mb-12 lg:hidden"
            >
              <BrandLogo />
            </Link>
            <Card className="rounded-none border-0 bg-transparent shadow-none">
              <CardHeader className="space-y-0 px-0 pb-6 pt-0">
                <p className="text-xs text-muted-foreground">HotKey Intelligence</p>
                <CardTitle className="mt-3">
                  <h1 className="text-2xl leading-8 tracking-[-0.035em]">{title}</h1>
                </CardTitle>
                <CardDescription className="mt-1.5 leading-6">{subtitle}</CardDescription>
              </CardHeader>
              <CardContent className="px-0 pb-0">
                {children}
              </CardContent>
            </Card>
          </div>
        </main>
      </div>
    </div>
  );
}
