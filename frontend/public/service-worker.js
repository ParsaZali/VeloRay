/* Cache public build assets only. Authenticated responses and subscriptions are
 * deliberately outside this worker's fetch handling, including navigations. */
const CACHE = "veloray-static-v1";
const OFFLINE = "/offline.html";
const LIMIT = 64;
self.addEventListener("install", event => {
  event.waitUntil(caches.open(CACHE).then(cache => cache.add(new Request(OFFLINE, {cache: "reload"}))));
});
self.addEventListener("activate", event => {
  event.waitUntil((async () => {
    for (const key of await caches.keys()) {
      if (key.startsWith("veloray-static-") && key !== CACHE) await caches.delete(key);
    }
    await self.clients.claim();
  })());
});
self.addEventListener("message", event => {
  if (event.data?.type === "VELO_UPDATE") self.skipWaiting();
});
function publicAsset(url) {
  return /^\/assets\/[A-Za-z0-9_-]+\.(js|css)$/.test(url.pathname) ||
    /^\/fonts\/[A-Za-z0-9_.-]+\.woff2$/.test(url.pathname) ||
    ["/veloray-logo.png", "/app-icon.svg"].includes(url.pathname);
}
self.addEventListener("fetch", event => {
  const request = event.request, url = new URL(request.url);
  if (request.method !== "GET" || url.origin !== self.location.origin || url.search ||
      url.pathname.startsWith("/api/") || url.pathname.startsWith("/sub/")) return;
  if (request.mode === "navigate") {
    event.respondWith(fetch(new Request(request, {cache: "no-store"})).catch(async () => (await caches.match(OFFLINE)) || Response.error()));
    return;
  }
  if (!publicAsset(url)) return;
  event.respondWith((async () => {
    const cache = await caches.open(CACHE), cached = await cache.match(request);
    if (cached) return cached;
    const response = await fetch(request);
    const type = response.headers.get("Content-Type") || "";
    if (response.ok && response.type === "basic" && !type.includes("text/html")) {
      await cache.put(request, response.clone());
      const keys = await cache.keys();
      let excess = keys.length - LIMIT;
      for (const key of keys) {
        if (excess > 0 && new URL(key.url).pathname !== OFFLINE) {await cache.delete(key);excess--;}
      }
    }
    return response;
  })());
});
