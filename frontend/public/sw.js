const SHELL_CACHE = "hotkey-public-shell-v1";
const OFFLINE_PATH = "/offline.html";
const PUBLIC_SHELL_ASSETS = [
  OFFLINE_PATH,
  "/icon.svg",
  "/icons/hotkey-192.png",
  "/icons/hotkey-512.png",
];

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
