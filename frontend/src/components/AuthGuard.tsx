"use client";

import { useAuthStore } from "@/stores/authStore";
import { Loader2 } from "lucide-react";
import { AuthStatus } from "@/lib/domainEnums";

export default function AuthGuard({ children }: { children: React.ReactNode }) {
  const status = useAuthStore((s) => s.status);

  if (status === AuthStatus.Initializing) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-background">
        <Loader2 className="h-8 w-8 animate-spin text-primary" />
      </div>
    );
  }

  if (status === AuthStatus.Unauthenticated) {
    if (typeof window !== "undefined") {
      window.location.href = "/login";
    }
    return null;
  }

  return <>{children}</>;
}
