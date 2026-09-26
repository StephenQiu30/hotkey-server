import Link from "next/link";
import { ArrowRightIcon } from "lucide-react";

import { Button } from "@/components/ui/button";

export function HeroSection() {
  return (
    <section className="relative isolate overflow-hidden px-5 pt-24 pb-16 sm:px-8 sm:pt-32 sm:pb-24 lg:pt-36 xl:px-16 2xl:px-0">
      <div aria-hidden="true" className="hero-wash absolute inset-0 -z-20" />
      <div
        aria-hidden="true"
        className="absolute top-12 right-0 -z-10 aspect-square w-full max-w-2xl opacity-60 sm:top-0 sm:opacity-100"
      >
        <div className="hero-ripple absolute inset-0 rounded-full" />
        <div className="hero-ripple absolute inset-12 rounded-full" />
        <div className="hero-ripple absolute inset-24 rounded-full" />
        <div className="hero-ripple absolute inset-40 rounded-full" />
        <div className="bg-background absolute top-1/2 left-1/2 size-3 -translate-x-1/2 -translate-y-1/2 rounded-full shadow-lg shadow-white" />
      </div>

      <div className="mx-auto max-w-7xl">
        <div className="max-w-4xl">
          <h1 className="text-4xl leading-tight font-semibold tracking-tighter text-balance sm:text-6xl lg:text-7xl">
            <span className="block">从一个关键词，</span>
            <span className="block">看见正在发生的变化</span>
          </h1>
          <p className="text-muted-foreground mt-8 max-w-2xl text-lg leading-8 text-pretty sm:text-xl sm:leading-9">
            设定你关心的品牌、产品或话题，持续汇集相关讨论；沿着来源和时间，看清变化如何发生。
          </p>
          <div className="mt-10 flex flex-wrap items-center gap-6">
            <Button asChild size="xl">
              <Link href="/monitors/new">
                设置监控主题
                <ArrowRightIcon data-icon="inline-end" />
              </Link>
            </Button>
            <Link
              href="#how-it-works"
              className="text-muted-foreground hover:text-foreground focus-visible:ring-ring rounded-sm text-sm underline underline-offset-4 transition-colors focus-visible:ring-2 focus-visible:outline-none"
            >
              了解如何追踪
            </Link>
          </div>
        </div>
      </div>
    </section>
  );
}
