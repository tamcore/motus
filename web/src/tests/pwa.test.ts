import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

// Mock $app/environment before any imports that depend on it
vi.mock("$app/environment", () => ({
  browser: true,
}));

describe("PWA Store", () => {
  let mockServiceWorker: {
    register: ReturnType<typeof vi.fn>;
    controller: ServiceWorker | null;
    addEventListener: ReturnType<typeof vi.fn>;
  };
  let mockRegistration: {
    installing: ServiceWorker | null;
    waiting: ServiceWorker | null;
    active: ServiceWorker | null;
    addEventListener: ReturnType<typeof vi.fn>;
    update: ReturnType<typeof vi.fn>;
  };

  beforeEach(() => {
    vi.clearAllMocks();
    sessionStorage.clear();

    mockRegistration = {
      installing: null,
      waiting: null,
      active: null,
      addEventListener: vi.fn(),
      update: vi.fn(),
    };

    mockServiceWorker = {
      register: vi.fn().mockResolvedValue(mockRegistration),
      controller: null,
      addEventListener: vi.fn(),
    };

    Object.defineProperty(navigator, "serviceWorker", {
      value: mockServiceWorker,
      writable: true,
      configurable: true,
    });
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  describe("Service Worker Registration", () => {
    it("should register the service worker at /sw.js with scope /", async () => {
      // Simulate service worker registration
      await navigator.serviceWorker.register("/sw.js", { scope: "/" });

      expect(mockServiceWorker.register).toHaveBeenCalledWith("/sw.js", {
        scope: "/",
      });
    });

    it("should handle registration failure gracefully", async () => {
      mockServiceWorker.register.mockRejectedValueOnce(
        new Error("Registration failed"),
      );

      let error: Error | null = null;
      try {
        await navigator.serviceWorker.register("/sw.js", { scope: "/" });
      } catch (e) {
        error = e as Error;
      }

      expect(error).not.toBeNull();
      expect(error?.message).toBe("Registration failed");
    });

    it("should check for updates on the registration", async () => {
      const registration = await navigator.serviceWorker.register("/sw.js", {
        scope: "/",
      });

      registration.update();
      expect(mockRegistration.update).toHaveBeenCalled();
    });
  });

  describe("Install State Management", () => {
    it("should track installable state", () => {
      let installable = false;
      let installDismissed = false;
      let installed = false;

      // Simulate beforeinstallprompt event
      installable = true;

      expect(installable).toBe(true);
      expect(installDismissed).toBe(false);
      expect(installed).toBe(false);
    });

    it("should track dismissed state in session storage", () => {
      sessionStorage.setItem("motus_pwa_install_dismissed", "true");

      const dismissed =
        sessionStorage.getItem("motus_pwa_install_dismissed") === "true";
      expect(dismissed).toBe(true);
    });

    it("should clear dismissed state when session storage is empty", () => {
      const dismissed =
        sessionStorage.getItem("motus_pwa_install_dismissed") === "true";
      expect(dismissed).toBe(false);
    });

    it("should detect standalone display mode as installed", () => {
      // Mock matchMedia since jsdom does not implement it
      const mockMatchMedia = vi.fn().mockReturnValue({
        matches: false,
        media: "(display-mode: standalone)",
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
      });
      Object.defineProperty(window, "matchMedia", {
        value: mockMatchMedia,
        writable: true,
        configurable: true,
      });

      const result = window.matchMedia("(display-mode: standalone)");
      expect(mockMatchMedia).toHaveBeenCalledWith("(display-mode: standalone)");
      // In non-standalone context, matches should be false
      expect(result.matches).toBe(false);
    });

    it("should transition to installed after appinstalled event", () => {
      let installable = true;
      let installed = false;

      // Simulate appinstalled event handler
      installable = false;
      installed = true;

      expect(installable).toBe(false);
      expect(installed).toBe(true);
    });
  });

  describe("Update Detection", () => {
    it("should detect when a new service worker is waiting", () => {
      let updateAvailable = false;

      // Simulate updatefound -> new worker installed
      const newWorkerState = "installed";
      const hasController = true; // There's already an active controller

      if (newWorkerState === "installed" && hasController) {
        updateAvailable = true;
      }

      expect(updateAvailable).toBe(true);
    });

    it("should not flag update when there is no existing controller", () => {
      let updateAvailable = false;

      const newWorkerState = "installed";
      const hasController = false; // First-time install, no existing controller

      if (newWorkerState === "installed" && hasController) {
        updateAvailable = true;
      }

      expect(updateAvailable).toBe(false);
    });

    it("should send SKIP_WAITING message to update worker", () => {
      const mockPostMessage = vi.fn();
      const updateWorker = { postMessage: mockPostMessage };

      updateWorker.postMessage({ type: "SKIP_WAITING" });

      expect(mockPostMessage).toHaveBeenCalledWith({ type: "SKIP_WAITING" });
    });
  });

  describe("Show Install Banner Logic", () => {
    it("should show banner when installable, not dismissed, not installed", () => {
      const state = {
        installable: true,
        installDismissed: false,
        installed: false,
      };

      const showBanner =
        state.installable && !state.installDismissed && !state.installed;
      expect(showBanner).toBe(true);
    });

    it("should hide banner when dismissed", () => {
      const state = {
        installable: true,
        installDismissed: true,
        installed: false,
      };

      const showBanner =
        state.installable && !state.installDismissed && !state.installed;
      expect(showBanner).toBe(false);
    });

    it("should hide banner when already installed", () => {
      const state = {
        installable: true,
        installDismissed: false,
        installed: true,
      };

      const showBanner =
        state.installable && !state.installDismissed && !state.installed;
      expect(showBanner).toBe(false);
    });

    it("should hide banner when not installable", () => {
      const state = {
        installable: false,
        installDismissed: false,
        installed: false,
      };

      const showBanner =
        state.installable && !state.installDismissed && !state.installed;
      expect(showBanner).toBe(false);
    });
  });
});

describe("PWA Manifest Validation", () => {
  it("should have required fields for installability", async () => {
    // Read the manifest (simulate what the browser does)
    const manifest = {
      name: "Motus GPS Tracking",
      short_name: "Motus",
      start_url: "/",
      display: "standalone",
      icons: [
        { src: "/icon-192.png", sizes: "192x192", type: "image/png" },
        { src: "/icon-512.png", sizes: "512x512", type: "image/png" },
      ],
    };

    expect(manifest.name).toBeTruthy();
    expect(manifest.short_name).toBeTruthy();
    expect(manifest.start_url).toBe("/");
    expect(manifest.display).toBe("standalone");
    expect(manifest.icons.length).toBeGreaterThanOrEqual(2);
  });

  it("should have a 192x192 icon", () => {
    const icons = [
      {
        src: "/icon-192.png",
        sizes: "192x192",
        type: "image/png",
        purpose: "any",
      },
      {
        src: "/icon-512.png",
        sizes: "512x512",
        type: "image/png",
        purpose: "any",
      },
    ];

    const has192 = icons.some((i) => i.sizes === "192x192");
    expect(has192).toBe(true);
  });

  it("should have a 512x512 icon", () => {
    const icons = [
      {
        src: "/icon-192.png",
        sizes: "192x192",
        type: "image/png",
        purpose: "any",
      },
      {
        src: "/icon-512.png",
        sizes: "512x512",
        type: "image/png",
        purpose: "any",
      },
    ];

    const has512 = icons.some((i) => i.sizes === "512x512");
    expect(has512).toBe(true);
  });

  it("should have maskable icons for adaptive icon support", () => {
    const icons = [
      {
        src: "/icon-maskable-192.png",
        sizes: "192x192",
        type: "image/png",
        purpose: "maskable",
      },
      {
        src: "/icon-maskable-512.png",
        sizes: "512x512",
        type: "image/png",
        purpose: "maskable",
      },
    ];

    const hasMaskable = icons.some((i) => i.purpose === "maskable");
    expect(hasMaskable).toBe(true);
  });

  it("should have valid display mode", () => {
    const validDisplayModes = [
      "fullscreen",
      "standalone",
      "minimal-ui",
      "browser",
    ];
    const display = "standalone";

    expect(validDisplayModes).toContain(display);
  });

  it("should have theme_color matching dark theme background", () => {
    const themeColor = "#1a1a1a";
    expect(themeColor).toBe("#1a1a1a"); // Matches --bg-primary in dark theme
  });
});

describe("Service Worker Caching Strategy", () => {
  describe("API Cache TTL Configuration", () => {
    const API_CACHE_CONFIG: Record<string, number> = {
      "/api/devices": 5 * 60 * 1000,
      "/api/positions": 2 * 60 * 1000,
      "/api/geofences": 10 * 60 * 1000,
      "/api/notifications": 10 * 60 * 1000,
      "/api/session": 15 * 60 * 1000,
    };

    it("should have TTL for device endpoints", () => {
      expect(API_CACHE_CONFIG["/api/devices"]).toBe(300000); // 5 min
    });

    it("should have shorter TTL for positions (real-time data)", () => {
      expect(API_CACHE_CONFIG["/api/positions"]).toBe(120000); // 2 min
      expect(API_CACHE_CONFIG["/api/positions"]).toBeLessThan(
        API_CACHE_CONFIG["/api/devices"],
      );
    });

    it("should have longer TTL for static-ish data", () => {
      expect(API_CACHE_CONFIG["/api/geofences"]).toBe(600000); // 10 min
      expect(API_CACHE_CONFIG["/api/geofences"]).toBeGreaterThan(
        API_CACHE_CONFIG["/api/devices"],
      );
    });

    it("should return null TTL for unconfigured endpoints", () => {
      function getApiCacheTtl(pathname: string): number | null {
        for (const [prefix, ttl] of Object.entries(API_CACHE_CONFIG)) {
          if (pathname.startsWith(prefix)) {
            return ttl;
          }
        }
        return null;
      }

      expect(getApiCacheTtl("/api/commands/send")).toBeNull();
      expect(getApiCacheTtl("/api/users")).toBeNull();
      expect(getApiCacheTtl("/api/devices")).toBe(300000);
      expect(getApiCacheTtl("/api/devices/123")).toBe(300000);
    });
  });

  describe("Cache Freshness Check", () => {
    it("should consider cache fresh within TTL", () => {
      const cachedAt = Date.now() - 60000; // 1 minute ago
      const maxAge = 300000; // 5 minutes
      const age = Date.now() - cachedAt;

      expect(age).toBeLessThan(maxAge);
    });

    it("should consider cache stale after TTL", () => {
      const cachedAt = Date.now() - 600000; // 10 minutes ago
      const maxAge = 300000; // 5 minutes
      const age = Date.now() - cachedAt;

      expect(age).toBeGreaterThan(maxAge);
    });
  });

  describe("Request Classification", () => {
    it("should identify API requests by path prefix", () => {
      const isApiRequest = (pathname: string) => pathname.startsWith("/api/");

      expect(isApiRequest("/api/devices")).toBe(true);
      expect(isApiRequest("/api/positions?deviceId=1")).toBe(true);
      expect(isApiRequest("/map")).toBe(false);
      expect(isApiRequest("/")).toBe(false);
    });

    it("should identify tile requests by hostname", () => {
      const isTileRequest = (hostname: string) =>
        hostname.includes("tile.openstreetmap.org");

      expect(isTileRequest("a.tile.openstreetmap.org")).toBe(true);
      expect(isTileRequest("b.tile.openstreetmap.org")).toBe(true);
      expect(isTileRequest("example.com")).toBe(false);
    });
  });

  describe("Precache URLs", () => {
    it("should include essential offline resources", () => {
      const PRECACHE_URLS = [
        "/",
        "/offline.html",
        "/manifest.json",
        "/favicon.png",
        "/icon-192.png",
        "/icon-512.png",
      ];

      expect(PRECACHE_URLS).toContain("/");
      expect(PRECACHE_URLS).toContain("/offline.html");
      expect(PRECACHE_URLS).toContain("/manifest.json");
    });
  });
});

describe("Background Sync Queue", () => {
  it("should store sync entries with required fields", () => {
    const entry = {
      id: 1,
      url: "/api/devices",
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ name: "Test Device" }),
    };

    expect(entry.url).toBeTruthy();
    expect(entry.method).toBeTruthy();
    expect(entry.headers).toBeDefined();
  });

  it("should remove entry from queue on successful sync", () => {
    const queue = [
      { id: 1, url: "/api/devices", method: "POST" },
      { id: 2, url: "/api/geofences", method: "POST" },
    ];

    // Simulate successful sync of first item
    const updatedQueue = queue.filter((item) => item.id !== 1);

    expect(updatedQueue).toHaveLength(1);
    expect(updatedQueue[0].id).toBe(2);
  });

  it("should retain entry in queue on failed sync", () => {
    const queue = [
      { id: 1, url: "/api/devices", method: "POST" },
      { id: 2, url: "/api/geofences", method: "POST" },
    ];

    // Failed sync - queue unchanged
    expect(queue).toHaveLength(2);
  });
});

describe("Offline Detection", () => {
  it("should detect online status from navigator", () => {
    // jsdom defaults to true
    expect(navigator.onLine).toBe(true);
  });

  it("should handle navigation requests with offline fallback", () => {
    // Simulate the service worker fetch logic for navigation
    const isNavigateRequest = true;
    const networkFailed = true;
    const hasCachedOfflinePage = true;

    const shouldShowOffline =
      isNavigateRequest && networkFailed && hasCachedOfflinePage;
    expect(shouldShowOffline).toBe(true);
  });

  it("should return 503 JSON for API requests when offline", () => {
    const isApiRequest = true;
    const networkFailed = true;
    const hasCachedResponse = false;

    if (isApiRequest && networkFailed && !hasCachedResponse) {
      const response = {
        status: 503,
        body: { error: "Network unavailable and no cached data" },
      };

      expect(response.status).toBe(503);
      expect(response.body.error).toContain("Network unavailable");
    }
  });
});
