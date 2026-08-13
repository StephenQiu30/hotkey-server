"use client";

import { useEffect } from "react";

const HOTKEY_SERVICE_WORKER_PATH = "/sw.js";

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
          // Offline-shell registration failure must not break the authenticated app.
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
