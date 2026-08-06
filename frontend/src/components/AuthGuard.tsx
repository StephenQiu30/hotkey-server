"use client";

import { useEffect } from "react";
import { usePathname, useRouter } from "next/navigation";
import { useAuthStore } from "@/stores/authStore";
import { Loader2 } from "lucide-react";
import { AuthStatus } from "@/lib/domainEnums";
import { createLoginRedirect } from "@/lib/safeRedirect";

export default function AuthGuard({ children }: { children: React.ReactNode }) {
  const status = useAuthStore((s) => s.status);
  const pathname = usePathname();
  const router = useRouter();

  useEffect(() => {
    if (status !== AuthStatus.Unauthenticated) return;
    router.replace(createLoginRedirect(pathname, window.location.search));
  }, [pathname, router, status]);

  if (status === AuthStatus.Initializing) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-background">
        <Loader2 className="h-8 w-8 animate-spin text-primary" />
      </div>
    );
  }

  if (status === AuthStatus.Unauthenticated) {
    return null;
  }

  return <>{children}</>;
}
