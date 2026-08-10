"use client";

import { useEffect, useRef } from "react";
import { Toaster } from "sonner";
import { useAuthStore } from "@/stores/authStore";
import { useTheme } from "@/components/ThemeProvider";
import { ServiceWorkerRegistrar } from "@/components/ServiceWorkerRegistrar";

export default function AppProvider({ children }: { children: React.ReactNode }) {
  const initialized = useRef(false);
  const initialize = useAuthStore((s) => s.initialize);
  const { theme } = useTheme();

  useEffect(() => {
    if (initialized.current) return;
    initialized.current = true;
    initialize();
  }, [initialize]);

  return (
    <>
      <ServiceWorkerRegistrar />
      {children}
      <Toaster
        position="top-center"
        richColors
        theme={theme}
        closeButton
        toastOptions={{
          style: {
            fontFamily:
              '"Geist", "PingFang SC", "Microsoft YaHei", ui-sans-serif, system-ui, sans-serif',
          },
        }}
      />
    </>
  );
}
