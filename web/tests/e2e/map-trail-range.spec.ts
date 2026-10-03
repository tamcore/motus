import { test, expect } from '../fixtures/auth-fixture';
import type { Page } from '@playwright/test';
import { MapPage } from '../page-objects/MapPage';

const HOUR = 60 * 60 * 1000;
const DAY = 24 * HOUR;

/**
 * Mocks a single device with a latest position plus a 3-point trail, and
 * records every trail request (/api/positions/points) on window.__trailRequests.
 */
async function mockTrailApi(page: Page) {
  await page.addInitScript(() => {
    const w = window as unknown as { __trailRequests: string[] };
    w.__trailRequests = [];
    const now = Date.now();
    const point = (minutesAgo: number, lat: number) => ({
      lat,
      lon: 9.95,
      speed: 10,
      fixTime: new Date(now - minutesAgo * 60_000).toISOString(),
    });
    const latest = {
      id: 3,
      deviceId: 4242,
      fixTime: new Date(now - 60_000).toISOString(),
      valid: true,
      latitude: 49.8,
      longitude: 9.95,
      speed: 10,
      course: 90,
      outdated: false,
    };
    const device = {
      id: 4242,
      uniqueId: '4242424242',
      name: 'Trail Range Device',
      status: 'online',
      disabled: false,
      lastUpdate: new Date(now).toISOString(),
      createdAt: new Date(now - 100 * 24 * 3600_000).toISOString(),
      updatedAt: new Date(now).toISOString(),
    };
    const json = (body: unknown) =>
      new Response(JSON.stringify(body), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      });
    const origFetch = window.fetch;
    window.fetch = async function (input: RequestInfo | URL, init?: RequestInit) {
      const url = typeof input === 'string' ? input : input instanceof URL ? input.href : input.url;
      if (url.includes('/api/devices')) {
        return json([device]);
      }
      if (url.includes('/api/positions/points')) {
        w.__trailRequests.push(url);
        return json([point(120, 49.78), point(60, 49.79), point(1, 49.8)]);
      }
      if (url.includes('/api/positions')) {
        return json([latest]);
      }
      return origFetch.apply(globalThis, [input, init] as Parameters<typeof fetch>);
    } as typeof fetch;
  });
}

async function trailRequests(page: Page): Promise<URL[]> {
  const urls = await page.evaluate(
    () => (window as unknown as { __trailRequests: string[] }).__trailRequests,
  );
  return urls.map((u) => new URL(u, 'http://localhost'));
}

function spanMs(url: URL): number {
  return new Date(url.searchParams.get('to')!).getTime() - new Date(url.searchParams.get('from')!).getTime();
}

test.describe('Map trail time ranges', () => {
  let mapPage: MapPage;

  test.beforeEach(async ({ authedPage }) => {
    await authedPage.evaluate(() => localStorage.removeItem('motus_trail_range'));
    await mockTrailApi(authedPage);
    mapPage = new MapPage(authedPage);
    await mapPage.goto();
    await mapPage.clickDevice(0);
    await expect(mapPage.detailPanel).toBeVisible();
  });

  test('defaults to the last 24h', async ({ authedPage }) => {
    await expect(mapPage.trailRangeSelect).toHaveValue('24h');
    await mapPage.trailButton.click();
    await expect(mapPage.trailStatus).toContainText('3 points');

    const requests = await trailRequests(authedPage);
    expect(requests).toHaveLength(1);
    expect(requests[0].pathname).toBe('/api/positions/points');
    expect(requests[0].searchParams.get('deviceId')).toBe('4242');
    expect(spanMs(requests[0])).toBe(DAY);
    expect(requests[0].searchParams.get('limit')).toBe('5000');
  });

  test('lists all range presets', async () => {
    await expect(mapPage.trailRangeSelect.locator('option')).toHaveText([
      'Last 24h',
      'Last 48h',
      'Last 7 days',
      'Last 30 days',
      'All time',
      'Custom range',
    ]);
  });

  test('selecting a preset loads the trail for that range', async ({ authedPage }) => {
    await mapPage.trailRangeSelect.selectOption('7d');
    await expect(mapPage.hideTrailButton).toBeVisible();
    await expect(mapPage.trailStatus).toContainText('3 points');

    const requests = await trailRequests(authedPage);
    expect(spanMs(requests[requests.length - 1])).toBe(7 * DAY);
    await expect(authedPage).toHaveURL(/trail=7d/);
  });

  test('all time starts at 2020-01-01 like heatmap and reports', async ({ authedPage }) => {
    await mapPage.trailRangeSelect.selectOption('all');
    await expect(mapPage.trailStatus).toContainText('3 points');
    const requests = await trailRequests(authedPage);
    expect(requests[requests.length - 1].searchParams.get('from')).toBe(
      '2020-01-01T00:00:00.000Z',
    );
  });

  test('custom range uses native date inputs with optional time', async ({ authedPage }) => {
    await mapPage.trailRangeSelect.selectOption('custom');
    await expect(mapPage.trailCustomRange).toBeVisible();
    await expect(mapPage.trailFromDate).toHaveAttribute('type', 'date');
    await expect(mapPage.trailFromTime).toHaveAttribute('type', 'time');

    await mapPage.trailFromDate.fill('2026-09-01');
    await mapPage.trailFromTime.fill('');
    await mapPage.trailToDate.fill('2026-09-02');
    await mapPage.trailToTime.fill('18:30');
    await mapPage.trailApply.click();
    await expect(mapPage.trailStatus).toContainText('3 points');

    const requests = await trailRequests(authedPage);
    const last = requests[requests.length - 1];
    const expected = await authedPage.evaluate(() => ({
      from: new Date(2026, 8, 1, 0, 0, 0).toISOString(),
      to: new Date(2026, 8, 2, 18, 30, 59).toISOString(),
    }));
    expect(new Date(last.searchParams.get('from')!).toISOString()).toBe(expected.from);
    expect(new Date(last.searchParams.get('to')!).toISOString()).toBe(expected.to);
    await expect(authedPage).toHaveURL(/from=.*&to=/);
  });

  test('custom range rejects a start after the end', async ({ authedPage }) => {
    await mapPage.trailRangeSelect.selectOption('custom');
    const before = (await trailRequests(authedPage)).length;
    await mapPage.trailFromDate.fill('2026-09-03');
    await mapPage.trailToDate.fill('2026-09-02');
    await mapPage.trailApply.click();
    await expect(mapPage.trailRangeError).toBeVisible();
    expect((await trailRequests(authedPage)).length).toBe(before);
  });

  test('selection persists across reloads', async ({ authedPage }) => {
    await mapPage.trailRangeSelect.selectOption('30d');
    await expect(mapPage.trailStatus).toContainText('3 points');

    await authedPage.goto('/map');
    await mapPage.waitForMapLoad();
    await mapPage.clickDevice(0);
    await expect(mapPage.trailRangeSelect).toHaveValue('30d');
  });

  test('URL from/to with device opens that trail without saving it', async ({ authedPage }) => {
    await authedPage.goto(
      '/map?device=4242&from=2026-09-01T00:00:00.000Z&to=2026-09-02T00:00:00.000Z',
    );
    await mapPage.waitForMapLoad();
    await expect(mapPage.trailRangeSelect).toHaveValue('custom');
    await expect(mapPage.trailStatus).toContainText('3 points');
    const requests = await trailRequests(authedPage);
    const last = requests[requests.length - 1];
    expect(last.searchParams.get('from')).toBe('2026-09-01T00:00:00.000Z');
    expect(last.searchParams.get('to')).toBe('2026-09-02T00:00:00.000Z');

    // The shared link applies to that view only; the saved default is kept.
    await authedPage.goto('/map');
    await mapPage.waitForMapLoad();
    await mapPage.clickDevice(0);
    await expect(mapPage.trailRangeSelect).toHaveValue('24h');
  });
});
