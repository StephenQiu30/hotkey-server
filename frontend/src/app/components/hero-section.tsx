import Link from "next/link";
import { ArrowRightIcon } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";

export function HeroSection() {
  return (
    <section className="relative isolate px-5 pt-20 pb-24 sm:px-8 sm:pt-28 sm:pb-32 xl:px-0 2xl:pt-32">
      <div aria-hidden="true" className="hero-grid absolute inset-0 -z-20" />
      <div
        aria-hidden="true"
        className="hero-mesh absolute inset-x-0 inset-y-0 -z-10 mx-auto max-w-7xl rounded-full opacity-70 blur-3xl"
      />
      <div className="mx-auto flex max-w-4xl flex-col items-center text-center">
        <Badge variant="secondary" className="font-mono font-normal">
          HOTKEY
        </Badge>
        <h1 className="mt-7 max-w-4xl text-4xl leading-tight font-semibold tracking-tighter text-balance sm:text-6xl lg:text-7xl">
          让热点信息变得清晰、可追溯。
        </h1>
        <p className="text-muted-foreground mt-6 max-w-2xl text-base leading-7 text-balance sm:text-lg sm:leading-8">
          汇总公开信息，整理事件脉络、讨论观点与证据来源。
        </p>
        <div className="mt-9">
          <Button asChild size="lg">
            <Link href="#capabilities">
              查看核心能力
              <ArrowRightIcon data-icon="inline-end" />
            </Link>
          </Button>
        </div>
      </div>
    </section>
  );
}
