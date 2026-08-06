import Link from "next/link";
import { ArrowLeft, LogIn } from "lucide-react";
import { BrandLogo } from "@/components/BrandLogo";
import { Button } from "@/components/ui/button";

export default function NotFound() {
  return (
    <main className="flex min-h-screen items-center justify-center bg-background px-5 py-12">
      <div className="w-full max-w-lg text-center">
        <Link
          href="/"
          aria-label="HotKey 首页"
          className="inline-flex text-foreground no-underline"
        >
          <BrandLogo />
        </Link>
        <p className="mono mt-12 text-sm text-blue-700">404</p>
        <h1 className="mt-3 text-3xl font-semibold">页面不存在</h1>
        <p className="mt-4 text-sm leading-6 text-slate-600">
          你访问的地址可能已被移动或删除。可以返回首页，或登录后继续使用工作台。
        </p>
        <div className="mt-8 flex flex-col justify-center gap-3 sm:flex-row">
          <Button asChild variant="outline">
            <Link href="/">
              <ArrowLeft />返回首页
            </Link>
          </Button>
          <Button asChild>
            <Link href="/login">
              <LogIn />登录工作台
            </Link>
          </Button>
        </div>
      </div>
    </main>
  );
}
