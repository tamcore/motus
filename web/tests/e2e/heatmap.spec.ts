import { test, expect } from '../fixtures/auth-fixture';

test.describe('Heatmap', () => {
  test('loads compact points instead of full positions', async ({ authedPage }) => {
    await authedPage.addInitScript(() => {
      const w = window as unknown as { __positionRequests: string[] };
      w.__positionRequests = [];
      const origFetch = window.fetch;
      const json = (body: unknown) =>
        new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } });
      window.fetch = async function (input: RequestInfo | URL, init?: RequestInit) {
        const url = typeof input === 'string' ? input : input instanceof URL ? input.href : input.url;
        if (url.includes('/api/devices')) {
          return json([{ id: 4242, name: 'Heat Device', uniqueId: 'heat-1', status: 'online' }]);
        }
        if (url.includes('/api/positions')) {
          w.__positionRequests.push(url);
          return json([
            { lat: 49.79, lon: 9.95, speed: 10, fixTime: '2026-10-01T10:00:00Z' },
            { lat: 49.8, lon: 9.96, speed: 20, fixTime: '2026-10-01T11:00:00Z' },
          ]);
        }
        return origFetch.apply(globalThis, [input, init] as Parameters<typeof fetch>);
      } as typeof fetch;
    });

    await authedPage.goto('/heatmap');

    await expect(authedPage.locator('.data-count')).toHaveText('2 points');
    const requests = await authedPage.evaluate(
      () => (window as unknown as { __positionRequests: string[] }).__positionRequests,
    );
    expect(requests.length).toBeGreaterThan(0);
    for (const url of requests) {
      const parsed = new URL(url, 'http://x');
      expect(parsed.pathname).toBe('/api/positions/points');
      expect(parsed.searchParams.get('deviceId')).toBe('4242');
      expect(parsed.searchParams.get('limit')).toBe('10000');
    }
  });

  test('renders heat points, also after navigating away and back', async ({ authedPage }) => {
    await authedPage.addInitScript(() => {
      const origFetch = window.fetch;
      const json = (body: unknown) =>
        new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } });
      window.fetch = async function (input: RequestInfo | URL, init?: RequestInit) {
        const url = typeof input === 'string' ? input : input instanceof URL ? input.href : input.url;
        if (url.includes('/api/devices')) {
          return json([{ id: 4242, name: 'Heat Device', uniqueId: 'heat-1', status: 'online' }]);
        }
        if (url.includes('/api/positions/points')) {
          return json([
            { lat: 49.79, lon: 9.95, speed: 10, fixTime: '2026-10-01T10:00:00Z' },
            { lat: 49.8, lon: 9.96, speed: 20, fixTime: '2026-10-01T11:00:00Z' },
          ]);
        }
        return origFetch.apply(globalThis, [input, init] as Parameters<typeof fetch>);
      } as typeof fetch;
    });

    // Number of painted (non-transparent) pixels on the heat layer canvas.
    const paintedPixels = () =>
      authedPage.evaluate(() => {
        const canvas = document.querySelector<HTMLCanvasElement>('canvas.leaflet-heatmap-layer');
        if (!canvas || canvas.width === 0 || canvas.height === 0) return 0;
        const data = canvas.getContext('2d')!.getImageData(0, 0, canvas.width, canvas.height).data;
        let painted = 0;
        for (let i = 3; i < data.length; i += 4) if (data[i] > 0) painted++;
        return painted;
      });

    await authedPage.goto('/heatmap');
    await expect(authedPage.locator('.data-count')).toHaveText('2 points');
    await expect.poll(paintedPixels).toBeGreaterThan(0);

    // Client-side navigation away and back: the leaflet.heat module is cached
    // and does not re-run, so the heat layer must still be available.
    await authedPage.locator('a.nav-link[href="/devices"]').click();
    await expect(authedPage).toHaveURL(/\/devices$/);
    await authedPage.locator('a.nav-link[href="/heatmap"]').click();
    await expect(authedPage).toHaveURL(/\/heatmap$/);
    await expect(authedPage.locator('.data-count')).toHaveText('2 points');
    await expect.poll(paintedPixels).toBeGreaterThan(0);
  });

  test('reloads the heatmap when an admin toggles All users', async ({ authedPage }) => {
    await authedPage.addInitScript(() => {
      localStorage.removeItem('motus_settings');
      const origFetch = window.fetch;
      const json = (body: unknown) =>
        new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } });
      window.fetch = async function (input: RequestInfo | URL, init?: RequestInit) {
        const url = typeof input === 'string' ? input : input instanceof URL ? input.href : input.url;
        // The admin owns no devices; other users' devices only appear with "All users".
        if (url.includes('/api/admin/devices')) {
          return json([{ id: 4242, name: 'Other User Device', uniqueId: 'heat-1', status: 'online' }]);
        }
        if (url.includes('/api/devices')) {
          return json([]);
        }
        if (url.includes('/api/positions/points')) {
          return json([
            { lat: 49.79, lon: 9.95, speed: 10, fixTime: '2026-10-01T10:00:00Z' },
            { lat: 49.8, lon: 9.96, speed: 20, fixTime: '2026-10-01T11:00:00Z' },
          ]);
        }
        return origFetch.apply(globalThis, [input, init] as Parameters<typeof fetch>);
      } as typeof fetch;
    });

    await authedPage.goto('/heatmap');
    await expect(authedPage.locator('.empty-message')).toBeVisible();
    await expect(authedPage.locator('.data-count')).toHaveText('0 points');

    await authedPage.locator('.admin-toggle input[type="checkbox"]').check();

    await expect(authedPage.locator('.data-count')).toHaveText('2 points');
    await expect(authedPage.locator('canvas.leaflet-heatmap-layer')).toBeAttached();
  });
});
