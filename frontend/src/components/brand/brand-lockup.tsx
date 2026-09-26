import Image from "next/image";
import Link from "next/link";

import { cn } from "@/lib/utils";

export function BrandMark() {
  return (
    <Image
      src="/icon.png?v=2"
      alt=""
      aria-hidden="true"
      width={44}
      height={44}
      loading="eager"
      unoptimized
      className="size-11 shrink-0 mix-blend-multiply"
    />
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
