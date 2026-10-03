import type { Page } from '@playwright/test';

export interface MockRoute {
  /** Substring of the request URL. */
  match?: string;
  /** Exact pathname, e.g. '/api/devices' (not '/api/devices/1'). */
  path?: string;
  /** Only this HTTP method (default: any). */
  method?: string;
  status?: number;
  /** JSON response body. */
  body?: unknown;
  /** Raw response text (e.g. an SSE stream) instead of `body`. */
  text?: string;
  contentType?: string;
  /** Collect the requested URLs in `window[record]` (read with `recordedUrls`). */
  record?: string;
}

/**
 * Mocks API responses by patching window.fetch before the app loads:
 * page.route() does not reliably intercept SvelteKit client-side fetches.
 * The first matching route wins; unmatched requests go to the network.
 */
export async function mockFetch(page: Page, routes: MockRoute[]): Promise<void> {
  await page.addInitScript((routes: MockRoute[]) => {
    const w = window as unknown as Record<string, string[]>;
    for (const r of routes) if (r.record) w[r.record] ??= [];
    const origFetch = window.fetch;
    window.fetch = async function (input: RequestInfo | URL, init?: RequestInit) {
      const url = typeof input === 'string' ? input : input instanceof URL ? input.href : input.url;
      const method = (init?.method ?? 'GET').toUpperCase();
      const path = new URL(url, window.location.origin).pathname;
      const route = routes.find(
        (r) =>
          (r.match === undefined || url.includes(r.match)) &&
          (r.path === undefined || path === r.path) &&
          (r.method === undefined || r.method === method),
      );
      if (!route) return origFetch.apply(globalThis, [input, init] as Parameters<typeof fetch>);
      if (route.record) w[route.record].push(url);
      const status = route.status ?? 200;
      if (status === 204) return new Response(null, { status });
      const text = route.text ?? JSON.stringify(route.body ?? null);
      return new Response(text, {
        status,
        headers: { 'Content-Type': route.contentType ?? 'application/json' },
      });
    } as typeof fetch;
  }, routes);
}

/** URLs collected by a route with `record: key`. */
export function recordedUrls(page: Page, key: string): Promise<string[]> {
  return page.evaluate((k) => (window as unknown as Record<string, string[]>)[k] ?? [], key);
}
