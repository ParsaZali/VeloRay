# Installable panel and safe static caching

VeloRay provides a scoped web-app manifest, the existing brand in a square SVG
icon, and a service worker. On HTTPS (or loopback during development), a supporting
browser can offer installation. The panel shows **Install app** when the browser
supplies an installation prompt. Installation support and promotion depend on the
browser; iOS/Safari users may use the browser's Add to Home Screen action.

The worker caches only same-origin fingerprinted JS/CSS, local WOFF2 fonts, public
brand icons and a small bilingual offline page. It never handles API/subscription
requests, non-GET requests, other origins or URLs with query strings. It does not
store HTML panel navigations, account data, subscription tokens or credentials.
Navigations use the network with no-store; if the server cannot be reached, the
static offline page asks the user to reconnect. Offline account management is not
provided.

The cache is bounded to 64 entries, preserving the offline page. Activation deletes
obsolete caches with the VeloRay static-cache prefix and preserves unrelated caches.
A new worker waits while the running application is open. **Update app** explicitly
activates it and reloads the application; the panel does not interrupt an active
edit by automatically reloading a new version. Closing all old app tabs also allows
the browser's normal service-worker lifecycle to activate a waiting worker.

Production browser tests build the real frontend, serve it over a local HTTP
server, close that server to verify the offline fallback, inspect storage for API
and subscription exclusion, check cache limits/cleanup, and exercise explicit
update acceptance. Development Vite mode does not register the worker automatically.

The existing server sends no-store for the service-worker script. Upstream reverse
proxies must preserve that behavior, serve `/service-worker.js` from the panel
origin and permit the root scope. No third-party CDN or offline-data store is used.
