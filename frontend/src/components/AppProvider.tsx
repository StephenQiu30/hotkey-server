"use client";

import { useEffect, useRef } from "react";
import { Toaster } from "sonner";
import { useAuthStore } from "@/stores/authStore";
import { useTheme } from "@/components/ThemeProvider";

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
      {children}
      <Toaster
        position="top-center"
        richColors
        theme={theme}
        closeButton
        toastOptions={{
          style: {
            fontFamily:
              '-apple-system, BlinkMacSystemFont, "SF Pro Display", "SF Pro Text", "Helvetica Neue", Arial, sans-serif',
          },
        }}
      />
    </>
  );
}
