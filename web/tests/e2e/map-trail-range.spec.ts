import { test, expect } from '../fixtures/auth-fixture';
import type { Page } from '@playwright/test';
import { MapPage } from '../page-objects/MapPage';
import { mockFetch, recordedUrls } from '../helpers/mock-fetch';

const HOUR = 60 * 60 * 1000;
const DAY = 24 * HOUR;

/** A single device with a latest position plus a 3-point trail; trail requests are recorded. */
async function mockTrailApi(page: Page) {
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
    createdAt: new Date(now - 100 * DAY).toISOString(),
    updatedAt: new Date(now).toISOString(),
  };
  await mockFetch(page, [
    { match: '/api/devices', body: [device] },
    {
      match: '/api/positions/points',
      body: [point(120, 49.78), point(60, 49.79), point(1, 49.8)],
      record: '__trailRequests',
    },
    { match: '/api/positions', body: [latest] },
  ]);
}

async function trailRequests(page: Page): Promise<URL[]> {
  return (await recordedUrls(page, '__trailRequests')).map((u) => new URL(u, 'http://localhost'));
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

  test('selecting a preset loads the trail for that range', async ({ authedPage }) => {
    await mapPage.trailRangeSelect.selectOption('7d');
    await expect(mapPage.hideTrailButton).toBeVisible();
    await expect(mapPage.trailStatus).toContainText('3 points');

    const requests = await trailRequests(authedPage);
    expect(spanMs(requests[requests.length - 1])).toBe(7 * DAY);
    await expect(authedPage).toHaveURL(/trail=7d/);
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
