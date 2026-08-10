import { readFileSync } from "node:fs";
import path from "node:path";
import vm from "node:vm";
import { describe, expect, it, vi } from "vitest";
import manifest from "@/app/manifest";

type Listener = (event: any) => void;

function serviceWorkerHarness() {
  const listeners = new Map<string, Listener>();
  const showNotification = vi.fn<(title: string, options: NotificationOptions) => Promise<void>>(
    () => Promise.resolve(),
  );
  const cacheMatch = vi.fn(() => Promise.resolve(new Response("offline")));
  const fetchMock = vi.fn(() => Promise.reject(new Error("offline")));
  const context = {
    URL,
    Response,
    Promise,
    Number,
    Object,
    Array,
    caches: {
      open: vi.fn(() => Promise.resolve({ addAll: vi.fn(() => Promise.resolve()) })),
      keys: vi.fn(() => Promise.resolve([])),
      delete: vi.fn(() => Promise.resolve(true)),
      match: cacheMatch,
    },
    fetch: fetchMock,
    self: {
      location: { origin: "https://hotkey.example" },
      addEventListener: (name: string, listener: Listener) => listeners.set(name, listener),
      skipWaiting: vi.fn(() => Promise.resolve()),
      registration: { showNotification },
      clients: {
        claim: vi.fn(() => Promise.resolve()),
        matchAll: vi.fn(() => Promise.resolve([])),
        openWindow: vi.fn(() => Promise.resolve()),
      },
    },
  };
  const source = readFileSync(path.join(process.cwd(), "public/sw.js"), "utf8");
  vm.runInNewContext(source, context, { filename: "public/sw.js" });
  return { listeners, showNotification, cacheMatch, fetchMock };
}

describe("PWA contract", () => {
  it("publishes a standalone manifest with local icons", () => {
    const value = manifest();
    expect(value.start_url).toBe("/dashboard/notifications");
    expect(value.display).toBe("standalone");
    expect(value.icons).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ src: "/icons/hotkey-192.png", sizes: "192x192" }),
        expect.objectContaining({ src: "/icons/hotkey-512.png", sizes: "512x512" }),
      ]),
    );
  });

  it("never intercepts API requests and uses only a public offline shell", async () => {
    const { listeners, cacheMatch, fetchMock } = serviceWorkerHarness();
    const respondWith = vi.fn();
    listeners.get("fetch")?.({
      request: {
        method: "GET",
        url: "https://hotkey.example/api/v1/notifications",
        mode: "cors",
        headers: { has: () => true },
      },
      respondWith,
    });
    expect(respondWith).not.toHaveBeenCalled();
    expect(cacheMatch).not.toHaveBeenCalled();

    const navigationRespondWith = vi.fn();
    listeners.get("fetch")?.({
      request: {
        method: "GET",
        url: "https://hotkey.example/dashboard/events",
        mode: "navigate",
        headers: { has: () => false },
      },
      respondWith: navigationRespondWith,
    });
    await expect(navigationRespondWith.mock.calls[0][0]).resolves.toBeInstanceOf(Response);
    expect(fetchMock).toHaveBeenCalledWith(expect.anything(), { cache: "no-store" });
    expect(cacheMatch).toHaveBeenCalledWith("/offline.html");
  });

  it("rejects enriched or unsafe push payloads and stores only a safe event deep link", async () => {
    const { listeners, showNotification } = serviceWorkerHarness();
    const invalidWait = vi.fn();
    listeners.get("push")?.({
      data: {
        json: () => ({
          title: "更新",
          event_id: 8,
          deep_link: "/dashboard/events?event=8",
          priority: "normal",
          summary: "不应进入推送载荷",
        }),
      },
      waitUntil: invalidWait,
    });
    expect(invalidWait).not.toHaveBeenCalled();
    expect(showNotification).not.toHaveBeenCalled();

    const validWait = vi.fn();
    listeners.get("push")?.({
      data: {
        json: () => ({
          title: "微事件更新",
          event_id: 8,
          deep_link: "/dashboard/events?event=8",
          priority: "normal",
        }),
      },
      waitUntil: validWait,
    });
    await validWait.mock.calls[0][0];
    expect(showNotification).toHaveBeenCalledWith(
      "微事件更新",
      expect.objectContaining({
        tag: "event-8",
        data: { deep_link: "/dashboard/events?event=8" },
      }),
    );
    const options = showNotification.mock.calls[0][1];
    expect(JSON.stringify(options)).not.toContain("summary");
    expect(JSON.stringify(options)).not.toContain("token");
  });
});
