"use client";

import { useRef } from "react";
import Link from "next/link";
import { useGSAP } from "@gsap/react";
import gsap from "gsap";
import { ArrowLeft, FileCheck2, Network, Radar } from "lucide-react";
import { BrandLogo } from "@/components/BrandLogo";
import { SignalOrbitScene } from "@/components/home/SignalOrbitScene";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";

gsap.registerPlugin(useGSAP);

const authCapabilities = [
  { icon: Network, label: "多源聚合" },
  { icon: Radar, label: "趋势识别" },
  { icon: FileCheck2, label: "证据复核" },
];

export default function AuthShell({
  title,
  subtitle,
  children,
}: {
  title: string;
  subtitle: string;
  children: React.ReactNode;
}) {
  const shellRef = useRef<HTMLDivElement>(null);

  useGSAP(
    () => {
      const media = gsap.matchMedia();
      media.add(
        {
          reduceMotion: "(prefers-reduced-motion: reduce)",
          allowMotion: "(prefers-reduced-motion: no-preference)",
        },
        (context) => {
          if (context.conditions?.reduceMotion) {
            gsap.set(".auth-animate", { x: 0, y: 0 });
            return;
          }

          gsap
            .timeline({ defaults: { duration: 0.46, ease: "power3.out" } })
            .from(".auth-brand", { y: -10 })
            .from(".auth-kicker", { y: 12 }, "<0.06")
            .from(".auth-title", { y: 24 }, "<0.06")
            .from(".auth-copy", { y: 18 }, "<0.12")
            .from(
              ".auth-capability",
              { y: 12, stagger: 0.05, duration: 0.36 },
              "<0.1",
            )
            .from(".auth-form-surface", { x: 22, duration: 0.48 }, 0.08);
        },
      );

      return () => media.revert();
    },
    { scope: shellRef },
  );

  return (
    <div
      ref={shellRef}
      className="relative min-h-svh overflow-x-hidden bg-background text-foreground"
    >
      <header className="absolute inset-x-0 top-0 z-30 bg-background">
        <div className="mx-auto flex h-[72px] max-w-[1440px] items-center px-5 sm:px-8">
          <Link
            href="/"
            aria-label="HotKey 首页"
            className="auth-brand auth-animate text-lg font-semibold tracking-[-0.03em] text-foreground no-underline"
          >
            <BrandLogo markClassName="h-[18px] w-[18px]" />
          </Link>
          <Link
            href="/"
            className="ml-auto inline-flex items-center gap-2 rounded-lg px-2 py-2 text-xs font-medium text-muted-foreground no-underline transition-colors hover:text-foreground focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
          >
            <ArrowLeft aria-hidden="true" className="h-3.5 w-3.5" />
            返回产品首页
          </Link>
        </div>
      </header>

      <div className="relative z-10 mx-auto grid min-h-svh w-full max-w-[1440px] lg:grid-cols-[minmax(0,1.12fr)_minmax(420px,.88fr)]">
        <aside
          aria-label="HotKey 品牌介绍"
          className="relative hidden min-h-svh overflow-hidden px-8 pb-8 pt-24 lg:flex lg:flex-col xl:px-12 xl:pb-10"
        >
          <div className="relative z-10 max-w-[660px]">
            <p className="auth-kicker auth-animate inline-flex items-center gap-2 text-xs font-medium tracking-[0.02em] text-muted-foreground">
              <span
                aria-hidden="true"
                className="h-1.5 w-1.5 rounded-full bg-foreground/60"
              />
              Editorial intelligence
            </p>
            <h2 className="auth-title auth-animate mt-5 text-[clamp(3rem,3.4vw,3.5rem)] font-semibold leading-[1.05] tracking-normal">
              <span className="block whitespace-nowrap">重要变化，</span>
              <span className="block whitespace-nowrap">先形成判断</span>
            </h2>
            <p className="auth-copy auth-animate mt-5 max-w-[420px] text-sm leading-7 text-muted-foreground xl:text-base xl:leading-8">
              持续监测公开信号，识别真正重要的变化，让每一次判断都建立在证据之上。
            </p>
            <dl className="mt-6 flex max-w-[440px] flex-wrap items-center gap-x-6 gap-y-3">
              {authCapabilities.map((item) => {
                const Icon = item.icon;
                return (
                  <div
                    key={item.label}
                    className="auth-capability auth-animate text-xs font-medium text-muted-foreground"
                  >
                    <dt className="flex items-center gap-2">
                      <Icon aria-hidden="true" className="h-3.5 w-3.5" />
                      {item.label}
                    </dt>
                  </div>
                );
              })}
            </dl>
          </div>

          <div className="pointer-events-none relative z-10 mt-6 h-[190px] w-[54%] self-end [&_.signal-orbit-label]:hidden [&>figure]:min-h-0 xl:h-[230px]">
            <SignalOrbitScene variant="ambient" className="h-full" />
          </div>

          <p className="relative z-10 mt-auto text-xs text-muted-foreground">© 2026 HotKey</p>
        </aside>

        <main className="flex min-h-svh items-center justify-center bg-background px-5 pb-10 pt-28 sm:px-8 lg:px-12 lg:py-16 xl:py-20">
          <div className="auth-form-surface auth-animate w-full max-w-[400px] bg-transparent shadow-none [&_[data-slot=input]]:bg-card">
            <Card className="rounded-none border-0 bg-transparent shadow-none">
              <CardHeader className="space-y-0 px-0 pb-8 pt-0">
                <p className="inline-flex w-fit items-center gap-2 text-xs font-medium tracking-[0.02em] text-muted-foreground">
                  <span
                    aria-hidden="true"
                    className="h-1.5 w-1.5 rounded-full bg-foreground/60"
                  />
                  HotKey Intelligence
                </p>
                <CardTitle className="mt-5">
                  <h1 className="text-[2rem] leading-10 tracking-normal">{title}</h1>
                </CardTitle>
                <CardDescription className="mt-2 leading-6">{subtitle}</CardDescription>
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
