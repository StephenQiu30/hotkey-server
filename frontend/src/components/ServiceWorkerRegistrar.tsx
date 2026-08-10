"use client";

import { useEffect } from "react";
import { HOTKEY_SERVICE_WORKER_PATH } from "@/lib/webPush";

export function ServiceWorkerRegistrar() {
  useEffect(() => {
    if (!("serviceWorker" in navigator) || !window.isSecureContext) return;

    const register = () => {
      void navigator.serviceWorker
        .register(HOTKEY_SERVICE_WORKER_PATH, {
          scope: "/",
          updateViaCache: "none",
        })
        .catch(() => {
          // Registration failure must not break the authenticated app. The
          // notification settings surface reports capability errors explicitly.
        });
    };

    if (document.readyState === "complete") {
      register();
      return;
    }
    window.addEventListener("load", register, { once: true });
    return () => window.removeEventListener("load", register);
  }, []);

  return null;
}
