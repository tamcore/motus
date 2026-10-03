import type { Page } from '@playwright/test';
import { test, expect } from '../fixtures/auth-fixture';
import { mockFetch, recordedUrls } from '../helpers/mock-fetch';

const HEAT_DEVICE = { id: 4242, name: 'Heat Device', uniqueId: 'heat-1', status: 'online' };
const HEAT_POINTS = [
  { lat: 49.79, lon: 9.95, speed: 10, fixTime: '2026-10-01T10:00:00Z' },
  { lat: 49.8, lon: 9.96, speed: 20, fixTime: '2026-10-01T11:00:00Z' },
];

interface HeatmapMocks {
  /** Response for /api/devices. */
  devices?: unknown[];
  /** Response for /api/admin/devices (only mocked when set). */
  adminDevices?: unknown[];
  /** Response for /api/positions/*. */
  points?: unknown[];
}

/**
 * Mocks the heatmap's API calls via a window.fetch monkeypatch (page.route()
 * does not reliably intercept SvelteKit client fetches) and records the
 * requested position URLs in window.__positionRequests.
 */
function mockHeatmapApi(page: Page, mocks: HeatmapMocks = {}) {
  return mockFetch(page, [
    ...(mocks.adminDevices ? [{ match: '/api/admin/devices', body: mocks.adminDevices }] : []),
    { match: '/api/devices', body: mocks.devices ?? [HEAT_DEVICE] },
    { match: '/api/positions', body: mocks.points ?? HEAT_POINTS, record: '__positionRequests' },
  ]);
}

function positionRequests(page: Page): Promise<string[]> {
  return recordedUrls(page, '__positionRequests');
}

test.describe('Heatmap', () => {
  test('loads compact points instead of full positions', async ({ authedPage }) => {
    await mockHeatmapApi(authedPage);

    await authedPage.goto('/heatmap');

    await expect(authedPage.locator('.data-count')).toHaveText('2 points');
    const requests = await positionRequests(authedPage);
    expect(requests.length).toBeGreaterThan(0);
    for (const url of requests) {
      const parsed = new URL(url, 'http://x');
      expect(parsed.pathname).toBe('/api/positions/points');
      expect(parsed.searchParams.get('deviceId')).toBe('4242');
      expect(parsed.searchParams.get('limit')).toBe('10000');
    }
  });

  test('renders heat points, also after navigating away and back', async ({ authedPage }) => {
    await mockHeatmapApi(authedPage);

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
    await authedPage.addInitScript(() => localStorage.removeItem('motus_settings'));
    // The admin owns no devices; other users' devices only appear with "All users".
    await mockHeatmapApi(authedPage, {
      devices: [],
      adminDevices: [{ id: 4242, name: 'Other User Device', uniqueId: 'heat-1', status: 'online' }],
    });

    await authedPage.goto('/heatmap');
    await expect(authedPage.locator('.empty-message')).toBeVisible();
    await expect(authedPage.locator('.data-count')).toHaveText('0 points');

    await authedPage.locator('.admin-toggle input[type="checkbox"]').check();

    await expect(authedPage.locator('.data-count')).toHaveText('2 points');
    await expect(authedPage.locator('canvas.leaflet-heatmap-layer')).toBeAttached();
  });
});

test.describe('Heatmap custom range', () => {
  // A zone east of UTC, so UTC midnight and local midnight differ.
  test.use({ timezoneId: 'Europe/Berlin' });

  test('queries from local midnight of the start day to the end of the end day', async ({ authedPage }) => {
    await mockHeatmapApi(authedPage, { points: [] });

    await authedPage.goto('/heatmap');
    await authedPage.locator('#date-range').selectOption('custom');

    // Native date inputs are filled with yyyy-mm-dd regardless of the locale display format.
    await authedPage.locator('#date-from').fill('2026-01-13');
    await authedPage.locator('#date-to').fill('2026-01-15');

    await authedPage.evaluate(() => {
      (window as unknown as { __positionRequests: string[] }).__positionRequests = [];
    });
    await authedPage.locator('.custom-range').getByRole('button', { name: 'Apply' }).click();

    await expect.poll(async () => (await positionRequests(authedPage)).length).toBeGreaterThan(0);
    const requests = await positionRequests(authedPage);
    const parsed = new URL(requests[requests.length - 1], 'http://x');
    // Local (Europe/Berlin, UTC+1 in January) start of 13 Jan / end of 15 Jan.
    expect(parsed.searchParams.get('from')).toBe('2026-01-12T23:00:00.000Z');
    expect(parsed.searchParams.get('to')).toBe('2026-01-15T22:59:59.999Z');
  });
});
