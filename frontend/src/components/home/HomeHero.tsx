"use client";

import { useRef } from "react";
import Link from "next/link";
import { useGSAP } from "@gsap/react";
import gsap from "gsap";
import { ArrowRight, FileCheck2, Flame, Network } from "lucide-react";
import { Button } from "@/components/ui/button";
import { SignalOrbitScene } from "@/components/home/SignalOrbitScene";

gsap.registerPlugin(useGSAP);

const capabilities = [
  { icon: Network, label: "跨来源聚合", detail: "授权公开来源" },
  { icon: Flame, label: "升温趋势识别", detail: "变化而非榜单" },
  { icon: FileCheck2, label: "证据链与复核", detail: "回到原始出处" },
];

export function HomeHero() {
  const heroRef = useRef<HTMLElement>(null);

  useGSAP(
    () => {
      const media = gsap.matchMedia();
      media.add(
        {
          reduceMotion: "(prefers-reduced-motion: reduce)",
          allowMotion: "(prefers-reduced-motion: no-preference)",
          desktop: "(min-width: 1024px)",
        },
        (context) => {
          const { reduceMotion, desktop } = context.conditions ?? {};
          if (reduceMotion) {
            gsap.set(".home-animate", { autoAlpha: 1, x: 0, y: 0 });
            return;
          }

          const intro = gsap.timeline({
            defaults: { duration: 0.72, ease: "power3.out" },
          });
          intro
            .from(".hero-kicker", { autoAlpha: 0, y: 14 })
            .from(".hero-title", { autoAlpha: 0, y: 30 }, "<0.08")
            .from(".hero-copy", { autoAlpha: 0, y: 22 }, "<0.1")
            .from(".hero-actions", { autoAlpha: 0, y: 18 }, "<0.08")
            .from(
              ".hero-capability",
              {
                autoAlpha: 0,
                x: desktop ? -16 : 0,
                y: desktop ? 0 : 12,
                stagger: 0.09,
                duration: 0.5,
              },
              "<0.08",
            );
        },
      );

      return () => media.revert();
    },
    { scope: heroRef },
  );

  return (
    <section ref={heroRef} className="relative isolate overflow-hidden">
      <div
        aria-hidden="true"
        className="intelligence-grid pointer-events-none absolute inset-0 -z-10"
      />
      <div className="mx-auto grid min-h-[calc(100svh-72px)] max-w-[1440px] items-center gap-12 px-5 py-16 sm:px-8 lg:grid-cols-[minmax(0,.92fr)_minmax(520px,1.08fr)] lg:gap-16 lg:py-14">
        <div className="relative z-10 max-w-[670px]">
          <div className="hero-kicker home-animate inline-flex items-center gap-2 rounded-full bg-muted px-3 py-1.5 text-xs font-semibold text-muted-foreground">
            <span
              aria-hidden="true"
              className="h-1.5 w-1.5 rounded-full bg-foreground/60"
            />
            Live signal intelligence
          </div>
          <h1
            aria-label="在热点成为热点前，看见它的升温轨迹"
            className="hero-title home-animate mt-7 text-balance text-[clamp(3.2rem,6.1vw,6.4rem)] font-semibold leading-[.94] tracking-[-0.045em]"
          >
            在热点成为热点前，
            <span className="block text-foreground">
              看见它的升温轨迹
            </span>
          </h1>
          <p className="hero-copy home-animate mt-7 max-w-[600px] text-pretty text-base leading-8 text-muted-foreground sm:text-lg">
            HotKey
            持续聚合授权公开来源，把零散内容聚合为可解释的事件、证据链与提醒，让研究、内容与舆情团队更早形成判断。
          </p>
          <div className="hero-actions home-animate mt-9 flex flex-col gap-3 sm:flex-row">
            <Button
              asChild
              size="lg"
              className="h-12 rounded-lg px-6"
            >
              <Link href="/register">
                建立第一条监控 <ArrowRight />
              </Link>
            </Button>
            <Button
              asChild
              size="lg"
              variant="outline"
              className="h-12 rounded-lg bg-secondary px-6"
            >
              <a href="#briefing">查看事件样例</a>
            </Button>
          </div>

          <dl className="mt-12 grid gap-3 sm:grid-cols-3">
            {capabilities.map((item) => {
              const Icon = item.icon;
              return (
                <div
                  key={item.label}
                  className="hero-capability home-animate rounded-lg bg-muted/55 p-4 backdrop-blur-sm"
                >
                  <dt className="flex items-center gap-2 text-sm font-semibold">
                    <Icon aria-hidden="true" className="h-4 w-4 text-muted-foreground" />
                    {item.label}
                  </dt>
                  <dd className="mt-2 text-xs text-muted-foreground">
                    {item.detail}
                  </dd>
                </div>
              );
            })}
          </dl>
        </div>

        <div className="relative h-[440px] lg:h-[650px]">
          <SignalOrbitScene className="h-full" />
        </div>
      </div>
    </section>
  );
}
