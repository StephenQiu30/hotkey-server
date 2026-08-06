"use client";

import Image from "next/image";
import Link from "next/link";
import { BrandLogo } from "@/components/BrandLogo";

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
    <div className="grid min-h-screen bg-background lg:grid-cols-[minmax(0,1fr)_480px]">
      <aside
        aria-label="HotKey 品牌介绍"
        className="relative hidden min-h-screen overflow-hidden border-r p-8 lg:flex lg:flex-col xl:p-10"
      >
        <Link
          href="/"
          className="relative z-10 text-base font-semibold text-foreground no-underline"
        >
          <BrandLogo markClassName="h-[18px] w-[18px]" />
        </Link>

        <div className="relative z-10 my-auto max-w-[620px]">
          <p className="text-xs text-muted-foreground">Editorial intelligence</p>
          <h2 className="mt-6 text-[clamp(3rem,5vw,5.75rem)] font-normal leading-[0.98] tracking-[-0.065em]">
            重要变化，
            <br />
            先形成判断
          </h2>
          <p className="mt-7 max-w-md text-sm leading-7 text-muted-foreground">
            持续监测公开信号，识别真正重要的变化，让每一次判断都建立在证据之上。
          </p>
        </div>

        <div className="pointer-events-none absolute bottom-[8%] right-[-3%] h-[46%] w-[58%] opacity-70 xl:right-[2%]">
          <Image
            src="/images/hotkey-signal-radar.png"
            alt=""
            fill
            priority
            sizes="50vw"
            className="object-contain object-right-bottom"
          />
        </div>

        <p className="relative z-10 text-xs text-muted-foreground">© 2026 HotKey</p>
      </aside>

      <main className="flex min-h-screen items-center justify-center px-5 py-12 sm:px-10">
        <div className="w-full max-w-sm">
          <Link
            href="/"
            className="mb-14 inline-flex text-base font-semibold text-foreground no-underline lg:hidden"
          >
            <BrandLogo />
          </Link>
          <p className="text-xs text-muted-foreground">HotKey Intelligence</p>
          <h1 className="mt-4 text-3xl font-semibold tracking-[-0.045em]">{title}</h1>
          <p className="mt-3 text-sm leading-6 text-muted-foreground">{subtitle}</p>
          <div className="mt-10">{children}</div>
        </div>
      </main>
    </div>
  );
}
