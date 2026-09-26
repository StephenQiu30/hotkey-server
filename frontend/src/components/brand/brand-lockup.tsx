import Link from "next/link";

import { cn } from "@/lib/utils";

export function BrandMark() {
  return (
    <span
      aria-hidden="true"
      className="bg-primary text-primary-foreground flex size-9 shrink-0 items-center justify-center rounded-lg text-xl font-bold"
    >
      见
    </span>
  );
}

type BrandLockupProps = {
  href: string;
  compactOnMobile?: boolean;
  showEnglish?: boolean;
};

export function BrandLockup({
  href,
  compactOnMobile = false,
  showEnglish = false,
}: BrandLockupProps) {
  return (
    <Link
      href={href}
      aria-label={href === "/" ? "知微见澜首页" : "知微见澜工作台"}
      className="focus-visible:ring-ring flex min-h-11 items-center gap-3 rounded-sm focus-visible:ring-2 focus-visible:outline-none"
    >
      <BrandMark />
      <span
        className={cn(
          "text-base font-semibold tracking-tight sm:text-lg",
          compactOnMobile && "hidden sm:inline",
        )}
      >
        知微见澜
      </span>
      {showEnglish ? (
        <span className="text-muted-foreground hidden text-sm sm:inline">
          / Ripplesight
        </span>
      ) : null}
    </Link>
  );
}
