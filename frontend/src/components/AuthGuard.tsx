"use client";

import { useEffect } from "react";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useAuthStore } from "@/stores/authStore";
import { Loader2, ShieldAlert } from "lucide-react";
import { AuthStatus } from "@/lib/domainEnums";
import { createLoginRedirect } from "@/lib/safeRedirect";
import { canAccessDashboardRoute } from "@/lib/dashboardAccess";
import {
  Alert,
  AlertDescription,
  AlertTitle,
} from "@/components/ui/alert";

export default function AuthGuard({ children }: { children: React.ReactNode }) {
  const status = useAuthStore((s) => s.status);
  const user = useAuthStore((s) => s.user);
  const pathname = usePathname();
  const router = useRouter();
  const accessDenied =
    status === AuthStatus.Authenticated &&
    !canAccessDashboardRoute(pathname, user?.role);

  useEffect(() => {
    if (status === AuthStatus.Unauthenticated) {
      router.replace(createLoginRedirect(pathname, window.location.search));
      return;
    }
  }, [pathname, router, status]);

  if (status === AuthStatus.Initializing) {
    return (
      <div
        role="status"
        aria-label="正在验证访问权限"
        className="flex min-h-screen items-center justify-center bg-background"
      >
        <Loader2 className="h-8 w-8 animate-spin text-primary" />
        <span className="sr-only">正在验证访问权限</span>
      </div>
    );
  }

  if (accessDenied) {
    return (
      <main className="app-page" tabIndex={-1}>
        <Alert aria-label="权限不足" className="mx-auto max-w-2xl">
          <ShieldAlert />
          <AlertTitle>权限不足</AlertTitle>
          <AlertDescription>
            当前账号没有访问此页面的权限。
            <Link className="ml-2 font-medium underline underline-offset-4" href="/dashboard">
              返回工作台
            </Link>
          </AlertDescription>
        </Alert>
      </main>
    );
  }

  if (status === AuthStatus.Unauthenticated) {
    return null;
  }

  return <>{children}</>;
}
