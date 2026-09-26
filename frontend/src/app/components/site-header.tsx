import Link from "next/link";
import { ArrowRightIcon } from "lucide-react";

import { BrandLockup } from "@/components/brand/brand-lockup";
import { Button } from "@/components/ui/button";

export function SiteHeader() {
  return (
    <header className="relative z-20 mx-auto flex h-20 max-w-7xl items-center justify-between px-5 sm:px-8 xl:px-16 2xl:px-0">
      <BrandLockup href="/" showEnglish />

      <Button asChild size="lg" className="min-h-11">
        <Link href="/login">
          登录
          <ArrowRightIcon data-icon="inline-end" />
        </Link>
      </Button>
    </header>
  );
}
