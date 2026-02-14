// Motus Service Worker
// Provides offline caching, API response caching with TTL, and background sync.

const CACHE_VERSION = "v4";
const STATIC_CACHE = `motus-static-${CACHE_VERSION}`;
const API_CACHE = `motus-api-${CACHE_VERSION}`;
const RUNTIME_CACHE = `motus-runtime-${CACHE_VERSION}`;

// Static assets to pre-cache during install
const PRECACHE_URLS = [
  "/",
  "/offline.html",
  "/manifest.json",
  "/favicon.png",
  "/icon-192.png",
  "/icon-512.png",
];

// API endpoints to cache with their TTL (in milliseconds)
const API_CACHE_CONFIG = {
  "/api/devices": 5 * 60 * 1000, // 5 minutes
  "/api/positions": 2 * 60 * 1000, // 2 minutes
  "/api/geofences": 10 * 60 * 1000, // 10 minutes
  "/api/notifications": 10 * 60 * 1000, // 10 minutes
  // NOTE: /api/session is intentionally NOT cached. Caching auth responses
  // causes stale CSRF tokens and stripped Set-Cookie headers, breaking
  // session persistence across PWA restarts and redeployments.
};

// Background sync queue name
const SYNC_QUEUE = "motus-sync-queue";

// ─── Install ────────────────────────────────────────────────────────────────

self.addEventListener("install", (event) => {
  event.waitUntil(
    caches
      .open(STATIC_CACHE)
      .then((cache) => cache.addAll(PRECACHE_URLS))
      .then(() => self.skipWaiting()),
  );
});

// ─── Activate ───────────────────────────────────────────────────────────────

self.addEventListener("activate", (event) => {
  event.waitUntil(
    caches
      .keys()
      .then((cacheNames) => {
        return Promise.all(
          cacheNames
            .filter((name) => {
              // Delete old versioned caches
              return (
                name.startsWith("motus-") &&
                name !== STATIC_CACHE &&
                name !== API_CACHE &&
                name !== RUNTIME_CACHE
              );
            })
            .map((name) => caches.delete(name)),
        );
      })
      .then(() => self.clients.claim()),
  );
});

// ─── Fetch ──────────────────────────────────────────────────────────────────

self.addEventListener("fetch", (event) => {
  const { request } = event;
  const url = new URL(request.url);

  // Skip non-GET requests (let them pass through)
  if (request.method !== "GET") {
    return;
  }

  // Skip WebSocket upgrade requests
  if (request.headers.get("Upgrade") === "websocket") {
    return;
  }

  // Skip all cross-origin requests (let the browser fetch them directly).
  // Intercepting tile requests caused OSM's CDN to return 404 on first load
  // because the SW added an Origin header that the browser's native <img>
  // requests don't include, resulting in different CDN cache behaviour.
  if (url.origin !== self.location.origin) {
    return;
  }

  // API requests: network-first with cache fallback and TTL
  if (url.pathname.startsWith("/api/")) {
    event.respondWith(handleApiRequest(request, url));
    return;
  }

  // Static assets and navigation: cache-first with network fallback
  event.respondWith(handleStaticRequest(request));
});

// ─── Strategy: API requests (network-first with TTL cache) ──────────────────

async function handleApiRequest(request, url) {
  const cacheTtl = getApiCacheTtl(url.pathname);

  // If this API path is not in our cache config, just fetch it
  if (cacheTtl === null) {
    try {
      return await fetch(request);
    } catch {
      return new Response(JSON.stringify({ error: "Network unavailable" }), {
        status: 503,
        headers: { "Content-Type": "application/json" },
      });
    }
  }

  try {
    // Try network first
    const networkResponse = await fetch(request);

    // Only cache successful responses
    if (networkResponse.ok) {
      const cache = await caches.open(API_CACHE);
      const responseToCache = networkResponse.clone();

      // Store with timestamp header for TTL checking
      const headers = new Headers(responseToCache.headers);
      headers.set("X-SW-Cached-At", Date.now().toString());

      const body = await responseToCache.blob();
      const cachedResponse = new Response(body, {
        status: responseToCache.status,
        statusText: responseToCache.statusText,
        headers: headers,
      });

      await cache.put(request, cachedResponse);
    }

    return networkResponse;
  } catch {
    // Network failed, try cache
    const cachedResponse = await getCachedResponse(request, cacheTtl);
    if (cachedResponse) {
      return cachedResponse;
    }

    return new Response(
      JSON.stringify({ error: "Network unavailable and no cached data" }),
      { status: 503, headers: { "Content-Type": "application/json" } },
    );
  }
}

// ─── Strategy: Static assets (cache-first) ──────────────────────────────────

async function handleStaticRequest(request) {
  // Navigation requests (HTML page loads) must use network-first.
  // Cache-first + revalidateInBackground with mode:'navigate' requests
  // causes Firefox to treat the background fetch as a page navigation,
  // creating an infinite reload loop.
  if (request.mode === "navigate") {
    try {
      const networkResponse = await fetch(request);
      if (networkResponse.ok) {
        const cache = await caches.open(RUNTIME_CACHE);
        cache.put(request, networkResponse.clone());
      }
      return networkResponse;
    } catch {
      // Network failed — try cache fallback, then offline page
      const cachedResponse = await caches.match(request);
      if (cachedResponse) return cachedResponse;
      const offlineResponse = await caches.match("/offline.html");
      if (offlineResponse) return offlineResponse;
      return new Response("Offline", {
        status: 503,
        headers: { "Content-Type": "text/plain" },
      });
    }
  }

  // Check static cache first
  const cachedResponse = await caches.match(request);
  if (cachedResponse) {
    // Revalidate in background for non-immutable assets
    const url = new URL(request.url);
    if (!url.pathname.includes("/_app/immutable/")) {
      revalidateInBackground(request);
    }
    return cachedResponse;
  }

  try {
    const networkResponse = await fetch(request);

    // Cache successful responses
    if (networkResponse.ok) {
      const cache = await caches.open(RUNTIME_CACHE);
      cache.put(request, networkResponse.clone());
    }

    return networkResponse;
  } catch {
    return new Response("Offline", {
      status: 503,
      headers: { "Content-Type": "text/plain" },
    });
  }
}

// ─── Cache TTL helpers ──────────────────────────────────────────────────────

function getApiCacheTtl(pathname) {
  for (const [prefix, ttl] of Object.entries(API_CACHE_CONFIG)) {
    if (pathname.startsWith(prefix)) {
      return ttl;
    }
  }
  return null;
}

async function getCachedResponse(request, maxAge) {
  const cache = await caches.open(API_CACHE);
  const response = await cache.match(request);

  if (!response) {
    return null;
  }

  const cachedAt = response.headers.get("X-SW-Cached-At");
  if (cachedAt) {
    const age = Date.now() - parseInt(cachedAt, 10);
    if (age > maxAge) {
      // Cache expired, delete it
      await cache.delete(request);
      return null;
    }
  }

  return response;
}

// ─── Background revalidation ────────────────────────────────────────────────

function revalidateInBackground(request) {
  fetch(request)
    .then((response) => {
      if (response.ok) {
        caches.open(RUNTIME_CACHE).then((cache) => {
          cache.put(request, response);
        });
      }
    })
    .catch(() => {
      // Silently fail - we already served from cache
    });
}

// ─── Background Sync ────────────────────────────────────────────────────────

self.addEventListener("sync", (event) => {
  if (event.tag === SYNC_QUEUE) {
    event.waitUntil(processSyncQueue());
  }
});

async function processSyncQueue() {
  // Open IndexedDB to get queued requests
  const db = await openSyncDB();
  const tx = db.transaction("requests", "readonly");
  const store = tx.objectStore("requests");
  const requests = await getAllFromStore(store);

  for (const entry of requests) {
    try {
      await fetch(entry.url, {
        method: entry.method,
        headers: entry.headers,
        body: entry.body,
        credentials: "include",
      });

      // Remove from queue on success
      const deleteTx = db.transaction("requests", "readwrite");
      deleteTx.objectStore("requests").delete(entry.id);
    } catch {
      // Leave in queue for next sync attempt
      break;
    }
  }
}

function openSyncDB() {
  return new Promise((resolve, reject) => {
    const request = indexedDB.open("motus-sync", 1);
    request.onupgradeneeded = () => {
      const db = request.result;
      if (!db.objectStoreNames.contains("requests")) {
        db.createObjectStore("requests", {
          keyPath: "id",
          autoIncrement: true,
        });
      }
    };
    request.onsuccess = () => resolve(request.result);
    request.onerror = () => reject(request.error);
  });
}

function getAllFromStore(store) {
  return new Promise((resolve, reject) => {
    const request = store.getAll();
    request.onsuccess = () => resolve(request.result);
    request.onerror = () => reject(request.error);
  });
}

// ─── Message handling (for update notification) ─────────────────────────────

self.addEventListener("message", (event) => {
  if (event.data && event.data.type === "SKIP_WAITING") {
    self.skipWaiting();
  }

  if (event.data && event.data.type === "GET_CACHE_STATS") {
    getCacheStats().then((stats) => {
      event.ports[0].postMessage(stats);
    });
  }
});

async function getCacheStats() {
  const cacheNames = await caches.keys();
  const stats = {};

  for (const name of cacheNames) {
    if (name.startsWith("motus-")) {
      const cache = await caches.open(name);
      const keys = await cache.keys();
      stats[name] = keys.length;
    }
  }

  return stats;
}
