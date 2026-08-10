const SHELL_CACHE = "hotkey-public-shell-v1";
const OFFLINE_PATH = "/offline.html";
const PUBLIC_SHELL_ASSETS = [
  OFFLINE_PATH,
  "/icon.svg",
  "/icons/hotkey-192.png",
  "/icons/hotkey-512.png",
];
const EVENT_DEEP_LINK = /^\/dashboard\/events\?event=[1-9][0-9]{0,18}$/;

self.addEventListener("install", (event) => {
  event.waitUntil(
    caches
      .open(SHELL_CACHE)
      .then((cache) => cache.addAll(PUBLIC_SHELL_ASSETS))
      .then(() => self.skipWaiting()),
  );
});

self.addEventListener("activate", (event) => {
  event.waitUntil(
    Promise.all([
      caches
        .keys()
        .then((keys) =>
          Promise.all(keys.filter((key) => key !== SHELL_CACHE).map((key) => caches.delete(key))),
        ),
      self.clients.claim(),
    ]),
  );
});

self.addEventListener("fetch", (event) => {
  const request = event.request;
  if (request.method !== "GET") return;
  const url = new URL(request.url);
  if (url.origin !== self.location.origin) return;

  // API, authenticated pages, and document bodies are always network-only and
  // are never passed to Cache Storage. A Service Worker must not create a
  // second local source of truth for private HotKey data.
  if (url.pathname.startsWith("/api/") || request.headers.has("Authorization")) return;

  if (request.mode === "navigate") {
    event.respondWith(
      fetch(request, { cache: "no-store" }).catch(async () => {
        const fallback = await caches.match(OFFLINE_PATH);
        return fallback || Response.error();
      }),
    );
    return;
  }

  if (PUBLIC_SHELL_ASSETS.includes(url.pathname)) {
    event.respondWith(caches.match(request).then((cached) => cached || fetch(request)));
  }
});

function safePushPayload(event) {
  if (!event.data) return null;
  try {
    const payload = event.data.json();
    if (!payload || typeof payload !== "object" || Array.isArray(payload)) return null;
    const keys = Object.keys(payload).sort();
    if (keys.join(",") !== "deep_link,event_id,priority,title") return null;
    if (
      !Number.isSafeInteger(payload.event_id) ||
      payload.event_id <= 0 ||
      typeof payload.title !== "string" ||
      payload.title.length === 0 ||
      payload.title.length > 160 ||
      payload.priority !== "normal" ||
      !EVENT_DEEP_LINK.test(payload.deep_link) ||
      payload.deep_link !== `/dashboard/events?event=${payload.event_id}`
    ) {
      return null;
    }
    return payload;
  } catch {
    return null;
  }
}

self.addEventListener("push", (event) => {
  const payload = safePushPayload(event);
  if (!payload) return;
  event.waitUntil(
    self.registration.showNotification(payload.title, {
      body: "HotKey 发现一条新的事件更新，点击后在站内查看来源与证据。",
      icon: "/icons/hotkey-192.png",
      badge: "/icons/hotkey-192.png",
      tag: `event-${payload.event_id}`,
      renotify: false,
      data: { deep_link: payload.deep_link },
    }),
  );
});

self.addEventListener("notificationclick", (event) => {
  event.notification.close();
  const deepLink = event.notification?.data?.deep_link;
  if (typeof deepLink !== "string" || !EVENT_DEEP_LINK.test(deepLink)) return;
  const destination = new URL(deepLink, self.location.origin);
  if (destination.origin !== self.location.origin) return;

  event.waitUntil(
    self.clients.matchAll({ type: "window", includeUncontrolled: true }).then((clients) => {
      for (const client of clients) {
        if ("navigate" in client && "focus" in client) {
          return client.navigate(destination.href).then(() => client.focus());
        }
      }
      return self.clients.openWindow(destination.href);
    }),
  );
});
