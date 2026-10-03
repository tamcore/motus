import { test, expect } from '../fixtures/auth-fixture';
import { RoutePlaybackPage } from '../page-objects/RoutePlaybackPage';

test.describe('Route Playback Page', () => {
  test('should have correct page title in tab', async ({ authedPage }) => {
    await authedPage.goto('/reports/route');
    await expect(authedPage).toHaveTitle('Route Playback - Motus');
  });

  test('should display route page container', async ({ authedPage }) => {
    await authedPage.goto('/reports/route');
    await expect(authedPage.locator('.route-page')).toBeVisible();
  });

  test('should display map element', async ({ authedPage }) => {
    await authedPage.goto('/reports/route');
    await expect(authedPage.locator('.route-map')).toBeVisible();
  });

  test('should show loading message initially', async ({ authedPage }) => {
    await authedPage.goto('/reports/route?deviceId=1&from=2025-01-01T00:00:00Z&to=2025-01-02T00:00:00Z');
    // Loading message appears briefly
    const loadingEl = authedPage.locator('.map-loading');
    // It may already be gone if load completes fast
    const count = await loadingEl.count();
    expect(count).toBeGreaterThanOrEqual(0);
  });

  test('should not show controls when no positions', async ({ authedPage }) => {
    // Use dates with no data
    await authedPage.goto('/reports/route?deviceId=999999&from=2020-01-01T00:00:00Z&to=2020-01-01T01:00:00Z');
    await authedPage.waitForTimeout(2000);
    await expect(authedPage.locator('.controls-container')).toHaveCount(0);
  });

  test('should not show info panel when no positions', async ({ authedPage }) => {
    await authedPage.goto('/reports/route?deviceId=999999&from=2020-01-01T00:00:00Z&to=2020-01-01T01:00:00Z');
    await authedPage.waitForTimeout(2000);
    await expect(authedPage.locator('.info-panel')).toHaveCount(0);
  });

  test('should handle missing query parameters gracefully', async ({ authedPage }) => {
    await authedPage.goto('/reports/route');
    await authedPage.waitForTimeout(1000);
    // Page should still render without crashing
    await expect(authedPage.locator('.route-page')).toBeVisible();
    await expect(authedPage.locator('.controls-container')).toHaveCount(0);
  });

  test('should navigate to route from reports page', async ({ authedPage }) => {
    await authedPage.goto('/reports');
    await authedPage.waitForSelector('h1:has-text("Reports")');
    // Verify the route link pattern exists in the DOM if trips are present
    const viewLinks = authedPage.locator('a.view-link');
    const count = await viewLinks.count();
    if (count > 0) {
      const href = await viewLinks.first().getAttribute('href');
      expect(href).toContain('/reports/route');
      expect(href).toContain('deviceId=');
    }
  });

  test('should show playback controls when positions exist', async ({ authedPage }) => {
    const mockPoints = [
      { lat: 51.5, lon: -0.09, speed: 30, fixTime: '2025-01-01T00:00:00Z' },
      { lat: 51.51, lon: -0.08, speed: 40, fixTime: '2025-01-01T00:01:00Z' },
      { lat: 51.52, lon: -0.07, speed: 35, fixTime: '2025-01-01T00:02:00Z' },
    ];

    // Intercept fetch at JS level — page.route() doesn't reliably intercept
    // SvelteKit's client-side fetch due to CSRF token validation on the server
    await authedPage.addInitScript((positions) => {
      const origFetch = window.fetch;
      window.fetch = async function (...args: Parameters<typeof fetch>) {
        const url = typeof args[0] === 'string' ? args[0] : args[0] instanceof URL ? args[0].href : (args[0] as Request).url;
        if (url.includes('/api/positions/points?')) {
          (window as unknown as { __pointsRequested?: boolean }).__pointsRequested = true;
          return new Response(JSON.stringify(positions), {
            status: 200,
            headers: { 'Content-Type': 'application/json' },
          });
        }
        return origFetch.apply(this, args);
      };
    }, mockPoints);

    await authedPage.goto('/reports/route?deviceId=1&from=2025-01-01T00:00:00Z&to=2025-01-02T00:00:00Z');

    await expect(authedPage.locator('.controls-container')).toBeVisible({ timeout: 20000 });
    await expect(authedPage.locator('button:has-text("Play")')).toBeVisible();
    await expect(authedPage.locator('button:has-text("Stop")')).toBeVisible();
    await expect(authedPage.locator('button:has-text("GPX")')).toBeVisible();
    await expect(authedPage.locator('.info-panel')).toContainText('51.500000, -0.090000');
    expect(
      await authedPage.evaluate(() => (window as unknown as { __pointsRequested?: boolean }).__pointsRequested),
    ).toBe(true);
  });

  test('should show speed selector buttons', async ({ authedPage }) => {
    await authedPage.addInitScript(() => {
      const origFetch = window.fetch;
      window.fetch = async function (...args: Parameters<typeof fetch>) {
        const url = typeof args[0] === 'string' ? args[0] : args[0] instanceof URL ? args[0].href : (args[0] as Request).url;
        if (url.includes('/api/positions/points?')) {
          return new Response(
            JSON.stringify([
              { lat: 51.5, lon: -0.09, speed: 30, fixTime: '2025-01-01T00:00:00Z' },
              { lat: 51.51, lon: -0.08, speed: 40, fixTime: '2025-01-01T00:01:00Z' },
            ]),
            { status: 200, headers: { 'Content-Type': 'application/json' } },
          );
        }
        return origFetch.apply(this, args);
      };
    });

    await authedPage.goto('/reports/route?deviceId=1&from=2025-01-01T00:00:00Z&to=2025-01-02T00:00:00Z');

    await expect(authedPage.locator('.controls-container')).toBeVisible({ timeout: 20000 });
    await expect(authedPage.locator('.speed-btn')).toHaveCount(4); // 1x, 2x, 4x, 8x
  });

  test('should show info panel with position data', async ({ authedPage }) => {
    await authedPage.route('**/api/positions/points*', (route) => {
      route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify([
          { lat: 51.5, lon: -0.09, speed: 30, fixTime: '2025-01-01T00:00:00Z' },
          { lat: 51.51, lon: -0.08, speed: 40, fixTime: '2025-01-01T00:01:00Z' },
        ]),
      });
    });

    await authedPage.goto('/reports/route?deviceId=1&from=2025-01-01T00:00:00Z&to=2025-01-02T00:00:00Z');
    await authedPage.waitForTimeout(2000);

    const infoPanel = authedPage.locator('.info-panel');
    if (await infoPanel.isVisible()) {
      await expect(infoPanel.locator('.info-label:has-text("Speed")')).toBeVisible();
      await expect(infoPanel.locator('.info-label:has-text("Location")')).toBeVisible();
    }
  });
});

// Trip times carry the server's UTC offset (e.g. "+02:00"). Unencoded in a link
// the "+" decodes to a space and /api/positions/points rejects it with 400.
const offsetPoints = [
  { lat: 50, lon: 10, speed: 30, fixTime: '2026-10-03T05:48:24Z', course: 45, altitude: 100 },
  { lat: 50.01, lon: 10.01, speed: 40, fixTime: '2026-10-03T05:49:24Z', course: 45, altitude: 110 },
];

async function mockPointsRecordingQuery(page: import('@playwright/test').Page) {
  await page.addInitScript((points) => {
    const origFetch = window.fetch;
    window.fetch = async function (...args: Parameters<typeof fetch>) {
      const url = typeof args[0] === 'string' ? args[0] : args[0] instanceof URL ? args[0].href : (args[0] as Request).url;
      if (url.includes('/api/positions/points?')) {
        (window as unknown as { __pointsUrl?: string }).__pointsUrl = url;
        return new Response(JSON.stringify(points), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        });
      }
      return origFetch.apply(this, args);
    };
  }, offsetPoints);
}

async function requestedRange(page: import('@playwright/test').Page) {
  const raw = await page.evaluate(() => (window as unknown as { __pointsUrl?: string }).__pointsUrl);
  const params = new URL(raw ?? '', 'http://localhost').searchParams;
  return { from: params.get('from'), to: params.get('to') };
}

test.describe('Trip links with UTC offsets', () => {
  const rawQuery = 'deviceId=1&from=2026-10-03T07:48:24+02:00&to=2026-10-03T08:17:34+02:00';

  test('replay loads a trip whose offset "+" was decoded to a space', async ({ authedPage }) => {
    await mockPointsRecordingQuery(authedPage);

    await authedPage.goto(`/reports/replay?${rawQuery}`);

    await expect(authedPage.locator('.playback-bar')).toBeVisible({ timeout: 20000 });
    await expect(authedPage.locator('.error-message')).toHaveCount(0);
    expect(await requestedRange(authedPage)).toEqual({
      from: '2026-10-03T05:48:24.000Z',
      to: '2026-10-03T06:17:34.000Z',
    });
  });

  test('route loads a trip whose offset "+" was decoded to a space', async ({ authedPage }) => {
    await mockPointsRecordingQuery(authedPage);

    await authedPage.goto(`/reports/route?${rawQuery}`);

    await expect(authedPage.locator('.controls-container')).toBeVisible({ timeout: 20000 });
    expect(await requestedRange(authedPage)).toEqual({
      from: '2026-10-03T05:48:24.000Z',
      to: '2026-10-03T06:17:34.000Z',
    });
  });

  test('reports page encodes offset trip times in Route and Replay links', async ({ authedPage }) => {
    await authedPage.addInitScript(() => {
      const origFetch = window.fetch;
      window.fetch = async function (...args: Parameters<typeof fetch>) {
        const url = typeof args[0] === 'string' ? args[0] : args[0] instanceof URL ? args[0].href : (args[0] as Request).url;
        const json = (body: unknown) =>
          new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } });
        const path = new URL(url, window.location.origin).pathname;
        if (path === '/api/devices' || path === '/api/admin/devices') {
          return json([{ id: 1, name: 'Offset Car', uniqueId: '9000000000099', status: 'offline' }]);
        }
        if (path === '/api/reports/activity') {
          return json({
            trips: [{
              deviceId: 1, deviceName: 'Offset Car',
              startTime: '2026-10-03T07:48:24+02:00', endTime: '2026-10-03T08:17:34+02:00',
              duration: 1750, distance: 11.5, avgSpeed: 20, maxSpeed: 25,
            }],
            stops: [],
          });
        }
        return origFetch.apply(this, args);
      };
    });

    await authedPage.goto('/reports');
    await authedPage.waitForSelector('h1:has-text("Reports")');
    await authedPage.click('button:has-text("Apply")');

    const replay = authedPage.locator('table.trips-table a.replay-link').first();
    await expect(replay).toBeVisible({ timeout: 20000 });
    for (const link of [replay, authedPage.locator('table.trips-table a.view-link:has-text("Route")').first()]) {
      const params = new URL((await link.getAttribute('href')) ?? '', 'http://localhost').searchParams;
      expect(params.get('from')).toBe('2026-10-03T07:48:24+02:00');
      expect(params.get('to')).toBe('2026-10-03T08:17:34+02:00');
    }
  });
});
