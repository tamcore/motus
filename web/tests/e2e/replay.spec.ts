import { test, expect } from '../fixtures/auth-fixture';
import { mockFetch, recordedUrls } from '../helpers/mock-fetch';

const points = [
  { lat: 50, lon: 10, speed: 30, fixTime: '2026-10-03T05:48:24Z', course: 45, altitude: 100 },
  { lat: 50.01, lon: 10.01, speed: 40, fixTime: '2026-10-03T05:49:24Z', course: 45, altitude: 110 },
];

const mockPoints = (page: import('@playwright/test').Page) =>
  mockFetch(page, [{ match: '/api/positions/points?', body: points, record: '__pointsUrls' }]);

test.describe('Trip replay', () => {
  // Trip times carry the server's UTC offset (e.g. "+02:00"). Unencoded in a
  // link the "+" decodes to a space and /api/positions/points rejects it.
  test('loads a trip whose offset "+" was decoded to a space', async ({ authedPage }) => {
    await mockPoints(authedPage);

    await authedPage.goto('/reports/replay?deviceId=1&from=2026-10-03T07:48:24+02:00&to=2026-10-03T08:17:34+02:00');

    await expect(authedPage.locator('.playback-bar')).toBeVisible({ timeout: 20000 });
    await expect(authedPage.locator('.error-message')).toHaveCount(0);
    const [url] = await recordedUrls(authedPage, '__pointsUrls');
    const params = new URL(url, 'http://localhost').searchParams;
    expect(params.get('from')).toBe('2026-10-03T05:48:24.000Z');
    expect(params.get('to')).toBe('2026-10-03T06:17:34.000Z');
  });

  test('downloads the trip as GPX', async ({ authedPage }) => {
    await mockPoints(authedPage);
    await authedPage.goto('/reports/replay?deviceId=1&from=2026-10-03T05:48:24Z&to=2026-10-03T06:17:34Z');
    await expect(authedPage.locator('.playback-bar')).toBeVisible({ timeout: 20000 });

    const [download] = await Promise.all([
      authedPage.waitForEvent('download'),
      authedPage.getByRole('button', { name: 'Download GPX' }).click(),
    ]);
    expect(download.suggestedFilename()).toBe('route-1.gpx');
  });

  test('reports page links trips to the replay with encoded offset times', async ({ authedPage }) => {
    await mockFetch(authedPage, [
      { path: '/api/devices', body: [{ id: 1, name: 'Offset Car', uniqueId: '9000000000099', status: 'offline' }] },
      { path: '/api/admin/devices', body: [{ id: 1, name: 'Offset Car', uniqueId: '9000000000099', status: 'offline' }] },
      {
        path: '/api/reports/activity',
        body: {
          trips: [{
            deviceId: 1, deviceName: 'Offset Car',
            startTime: '2026-10-03T07:48:24+02:00', endTime: '2026-10-03T08:17:34+02:00',
            duration: 1750, distance: 11.5, avgSpeed: 20, maxSpeed: 25,
          }],
          stops: [],
        },
      },
    ]);

    await authedPage.goto('/reports');
    await authedPage.waitForSelector('h1:has-text("Reports")');
    await authedPage.click('button:has-text("Apply")');

    const replay = authedPage.locator('table.trips-table a.replay-link').first();
    await expect(replay).toBeVisible({ timeout: 20000 });
    const href = new URL((await replay.getAttribute('href')) ?? '', 'http://localhost');
    expect(href.pathname).toBe('/reports/replay');
    expect(href.searchParams.get('from')).toBe('2026-10-03T07:48:24+02:00');
    expect(href.searchParams.get('to')).toBe('2026-10-03T08:17:34+02:00');
    await expect(authedPage.locator('table.trips-table a:has-text("Route")')).toHaveCount(0);
  });
});
