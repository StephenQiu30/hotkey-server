import Link from "next/link";
import { ArrowRightIcon } from "lucide-react";

import { Button } from "@/components/ui/button";

export function SiteHeader() {
  return (
    <header className="relative z-20 mx-auto flex h-16 max-w-7xl items-center justify-between px-5 sm:px-8 xl:px-0">
      <Link
        href="/"
        className="flex min-h-11 items-center gap-2.5 font-medium md:min-h-8"
        aria-label="HotKey 首页"
      >
        <span className="bg-primary text-primary-foreground flex size-8 items-center justify-center rounded-lg font-mono text-xs">
          HK
        </span>
        <span>HotKey</span>
      </Link>

      <Button asChild size="navigation">
        <Link href="/login">
          登录
          <ArrowRightIcon data-icon="inline-end" />
        </Link>
      </Button>
    </header>
  );
}
