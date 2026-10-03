import { test, expect } from '../fixtures/auth-fixture';
import type { Page } from '@playwright/test';
import { MapPage } from '../page-objects/MapPage';
import { BookmarksPage } from '../page-objects/BookmarksPage';

const SEED_FROM = '2026-06-06T06:00:00.000Z';
const SEED_TO = '2026-06-06T14:30:00.000Z';

const DEVICE_ID = 4242;
const OTHER_DEVICE_ID = 4343;

interface RecordedRequest {
  method: string;
  url: string;
  body: Record<string, unknown> | null;
}

interface MockOptions {
  /** Device whose bookmark list responses are held back until released. */
  delayBookmarksForDevice?: number;
}

/**
 * Mocks two devices (with a trail) and an in-memory trail bookmark API seeded
 * with `seed` bookmarks. Trail requests are recorded on window.__trailRequests,
 * bookmark requests on window.__bookmarkRequests. Setting window.__failTrail
 * makes trail requests fail; window.__releaseBookmarks() releases delayed
 * bookmark lists.
 */
async function mockBookmarkApi(
  page: Page,
  seed: Array<Record<string, unknown>>,
  options: MockOptions = {},
) {
  await page.addInitScript(
    ({ seedBookmarks, opts, deviceId, otherDeviceId }) => {
      const w = window as unknown as {
        __trailRequests: string[];
        __bookmarkRequests: RecordedRequest[];
        __bookmarks: Array<Record<string, unknown>>;
        __failTrail: boolean;
        __releaseBookmarks: () => void;
      };
      w.__trailRequests = [];
      w.__bookmarkRequests = [];
      w.__bookmarks = seedBookmarks.map((b) => ({ ...b }));
      w.__failTrail = false;
      const held: Array<() => void> = [];
      w.__releaseBookmarks = () => held.splice(0).forEach((release) => release());
      let nextId = 100;
      const now = Date.now();
      const point = (minutesAgo: number, lat: number) => ({
        lat,
        lon: 9.95,
        speed: 10,
        fixTime: new Date(now - minutesAgo * 60_000).toISOString(),
      });
      const latest = (id: number, lat: number) => ({
        id,
        deviceId: id,
        fixTime: new Date(now - 60_000).toISOString(),
        valid: true,
        latitude: lat,
        longitude: 9.95,
        speed: 10,
        course: 90,
        outdated: false,
      });
      const device = (id: number, name: string) => ({
        id,
        uniqueId: String(id).repeat(2),
        name,
        status: 'online',
        disabled: false,
        lastUpdate: new Date(now).toISOString(),
        createdAt: new Date(now - 100 * 24 * 3600_000).toISOString(),
        updatedAt: new Date(now).toISOString(),
      });
      const devices = [device(deviceId, 'Hiking Watch'), device(otherDeviceId, 'Bike Tracker')];
      const json = (body: unknown, status = 200) =>
        new Response(status === 204 ? null : JSON.stringify(body), {
          status,
          headers: { 'Content-Type': 'application/json' },
        });
      const origFetch = window.fetch;
      window.fetch = async function (input: RequestInfo | URL, init?: RequestInit) {
        const url = typeof input === 'string' ? input : input instanceof URL ? input.href : input.url;
        const method = (init?.method ?? 'GET').toUpperCase();
        if (url.includes('/api/trail-bookmarks')) {
          const body = init?.body ? JSON.parse(String(init.body)) : null;
          w.__bookmarkRequests.push({ method, url, body });
          const idMatch = /\/api\/trail-bookmarks\/(\d+)/.exec(url);
          const id = idMatch ? Number(idMatch[1]) : null;
          const stamp = new Date().toISOString();
          if (method === 'GET' && id == null) {
            const filter = new URL(url, location.origin).searchParams.get('deviceId');
            const list = w.__bookmarks
              .filter((b) => filter == null || String(b.deviceId) === filter)
              .sort((a, b) => String(b.from).localeCompare(String(a.from)));
            if (opts.delayBookmarksForDevice != null && filter === String(opts.delayBookmarksForDevice)) {
              await new Promise<void>((resolve) => held.push(resolve));
            }
            return json(list);
          }
          if (method === 'POST') {
            const created = {
              ...body,
              id: ++nextId,
              deviceName: 'Hiking Watch',
              createdAt: stamp,
              updatedAt: stamp,
            };
            w.__bookmarks.push(created);
            return json(created, 201);
          }
          if (method === 'PUT' && id != null) {
            const idx = w.__bookmarks.findIndex((b) => b.id === id);
            w.__bookmarks[idx] = { ...w.__bookmarks[idx], ...body, updatedAt: stamp };
            return json(w.__bookmarks[idx]);
          }
          if (method === 'DELETE' && id != null) {
            w.__bookmarks = w.__bookmarks.filter((b) => b.id !== id);
            return json(null, 204);
          }
        }
        if (url.includes('/api/devices')) {
          return json(devices);
        }
        if (url.includes('/api/geofences')) {
          return json([]);
        }
        if (url.includes('/api/positions/points')) {
          w.__trailRequests.push(url);
          if (w.__failTrail) return json({ error: 'boom' }, 500);
          return json([point(120, 49.78), point(60, 49.79), point(1, 49.8)]);
        }
        if (url.includes('/api/positions')) {
          return json([latest(deviceId, 49.8), latest(otherDeviceId, 49.7)]);
        }
        return origFetch.apply(globalThis, [input, init] as Parameters<typeof fetch>);
      } as typeof fetch;
    },
    { seedBookmarks: seed, opts: options, deviceId: DEVICE_ID, otherDeviceId: OTHER_DEVICE_ID },
  );
}

const zugspitze = {
  id: 1,
  userId: 1,
  deviceId: DEVICE_ID,
  deviceName: 'Hiking Watch',
  name: 'Zugspitze',
  description: 'Via Höllental',
  from: SEED_FROM,
  to: SEED_TO,
  createdAt: '2026-06-07T00:00:00.000Z',
  updatedAt: '2026-06-07T00:00:00.000Z',
};

async function bookmarkRequests(page: Page): Promise<RecordedRequest[]> {
  return page.evaluate(
    () => (window as unknown as { __bookmarkRequests: RecordedRequest[] }).__bookmarkRequests,
  );
}

async function lastTrailRequest(page: Page): Promise<URL> {
  const urls = await page.evaluate(
    () => (window as unknown as { __trailRequests: string[] }).__trailRequests,
  );
  expect(urls.length).toBeGreaterThan(0);
  return new URL(urls[urls.length - 1], 'http://localhost');
}

function savedTrailRange(page: Page): Promise<string | null> {
  return page.evaluate(() => localStorage.getItem('motus_trail_range'));
}

test.describe('Trail bookmarks on the map', () => {
  let mapPage: MapPage;

  test.beforeEach(async ({ authedPage }) => {
    await authedPage.evaluate(() => localStorage.removeItem('motus_trail_range'));
    await mockBookmarkApi(authedPage, [zugspitze]);
    mapPage = new MapPage(authedPage);
    await mapPage.goto();
    await mapPage.deviceItem('Hiking Watch').click();
    await expect(mapPage.detailPanel).toBeVisible();
  });

  test('lists the bookmarks of the selected device', async ({ authedPage }) => {
    await expect(mapPage.bookmarkItems).toHaveCount(1);
    await expect(mapPage.bookmarkItems.first()).toContainText('Zugspitze');
    const requests = await bookmarkRequests(authedPage);
    const list = requests.find((r) => r.method === 'GET');
    expect(list && new URL(list.url, 'http://localhost').searchParams.get('deviceId')).toBe(
      String(DEVICE_ID),
    );
    await expect(mapPage.bookmarkList.locator('a[href="/bookmarks"]')).toBeVisible();
  });

  test('clicking a bookmark shows its trail range', async ({ authedPage }) => {
    await mapPage.bookmarkOpenButton('Zugspitze').click();
    await expect(mapPage.trailStatus).toContainText('3 points');
    await expect(mapPage.trailRangeSelect).toHaveValue('custom');
    await expect(mapPage.bookmarkItems.first()).toHaveClass(/active/);

    const trail = await lastTrailRequest(authedPage);
    expect(trail.pathname).toBe('/api/positions/points');
    expect(trail.searchParams.get('from')).toBe(SEED_FROM);
    expect(trail.searchParams.get('to')).toBe(SEED_TO);

    // The URL reopens this device's trail.
    const url = new URL(authedPage.url());
    expect(url.searchParams.get('device')).toBe(String(DEVICE_ID));
    expect(url.searchParams.get('from')).toBe(SEED_FROM);
    expect(url.searchParams.get('to')).toBe(SEED_TO);
  });

  test('opening a bookmark does not replace the saved default range', async ({ authedPage }) => {
    await mapPage.trailRangeSelect.selectOption('7d');
    await expect(mapPage.trailStatus).toContainText('3 points');
    const saved = await savedTrailRange(authedPage);
    expect(JSON.parse(saved!)).toEqual({ preset: '7d' });

    await mapPage.bookmarkOpenButton('Zugspitze').click();
    await expect(mapPage.trailRangeSelect).toHaveValue('custom');
    expect(await savedTrailRange(authedPage)).toBe(saved);

    await authedPage.goto('/map');
    await mapPage.waitForMapLoad();
    await mapPage.deviceItem('Hiking Watch').click();
    await expect(mapPage.trailRangeSelect).toHaveValue('7d');
  });

  test('a failed trail request does not keep the previous trail', async ({ authedPage }) => {
    const trailPaths = authedPage.locator('.leaflet-overlay-pane path');
    await mapPage.trailRangeSelect.selectOption('7d');
    await expect(mapPage.trailStatus).toContainText('3 points');
    await expect(trailPaths).not.toHaveCount(0);

    await authedPage.evaluate(() => {
      (window as unknown as { __failTrail: boolean }).__failTrail = true;
    });
    await mapPage.bookmarkOpenButton('Zugspitze').click();
    await expect(mapPage.trailStatus).toContainText('Failed to load trail');
    await expect(trailPaths).toHaveCount(0);
  });

  test('saves the current trail range as a bookmark', async ({ authedPage }) => {
    await mapPage.trailRangeSelect.selectOption('custom');
    await mapPage.trailFromDate.fill('2026-09-01');
    await mapPage.trailFromTime.fill('07:30');
    await mapPage.trailToDate.fill('2026-09-01');
    await mapPage.trailToTime.fill('15:00');
    await mapPage.trailApply.click();
    await expect(mapPage.trailStatus).toContainText('3 points');

    await mapPage.saveBookmarkButton.click();
    await expect(mapPage.bookmarkDialog).toBeVisible();
    await expect(mapPage.bookmarkDialog).toContainText('Save trail bookmark');
    // Prefilled with the applied range in native date/time inputs.
    await expect(mapPage.bookmarkFromDate).toHaveAttribute('type', 'date');
    await expect(mapPage.bookmarkFromTime).toHaveAttribute('type', 'time');
    await expect(mapPage.bookmarkFromDate).toHaveValue('2026-09-01');
    await expect(mapPage.bookmarkFromTime).toHaveValue('07:30');
    await expect(mapPage.bookmarkToDate).toHaveValue('2026-09-01');
    await expect(mapPage.bookmarkToTime).toHaveValue('15:00');

    await mapPage.bookmarkName.fill('Watzmann');
    await mapPage.bookmarkDescription.fill('Ostwand, long day');
    await mapPage.bookmarkSubmit.click();
    await expect(mapPage.bookmarkDialog).toBeHidden();

    const post = (await bookmarkRequests(authedPage)).find((r) => r.method === 'POST');
    expect(post?.body).toMatchObject({
      deviceId: DEVICE_ID,
      name: 'Watzmann',
      description: 'Ostwand, long day',
    });
    // Unchanged fields keep the applied range's exact boundaries.
    const applied = await lastTrailRequest(authedPage);
    expect(post?.body?.from).toBe(applied.searchParams.get('from'));
    expect(post?.body?.to).toBe(applied.searchParams.get('to'));

    await expect(mapPage.bookmarkItems).toHaveCount(2);
    await expect(mapPage.bookmarkOpenButton('Watzmann')).toBeVisible();
  });

  test('requires a name before saving', async ({ authedPage }) => {
    await mapPage.saveBookmarkButton.click();
    await mapPage.bookmarkSubmit.click();
    await expect(mapPage.bookmarkError).toContainText('Name is required');
    const posts = (await bookmarkRequests(authedPage)).filter((r) => r.method === 'POST');
    expect(posts).toHaveLength(0);
  });

  test('renaming a bookmark keeps its exact range', async ({ authedPage }) => {
    await mapPage.bookmarkEditButton('Zugspitze').click();
    await expect(mapPage.bookmarkDialog).toContainText('Edit trail bookmark');
    await expect(mapPage.bookmarkName).toHaveValue('Zugspitze');
    await mapPage.bookmarkName.fill('Zugspitze (summit)');
    await mapPage.bookmarkSubmit.click();
    await expect(mapPage.bookmarkDialog).toBeHidden();

    const put = (await bookmarkRequests(authedPage)).find((r) => r.method === 'PUT');
    expect(put?.url).toContain('/api/trail-bookmarks/1');
    expect(put?.body).toEqual({
      deviceId: DEVICE_ID,
      name: 'Zugspitze (summit)',
      description: 'Via Höllental',
      from: SEED_FROM,
      to: SEED_TO,
    });
    await expect(mapPage.bookmarkItems.first()).toContainText('Zugspitze (summit)');
  });

  test('deletes a bookmark after confirmation', async ({ authedPage }) => {
    authedPage.once('dialog', (dialog) => dialog.accept());
    await mapPage.bookmarkDeleteButton('Zugspitze').click();
    await expect(mapPage.bookmarkItems).toHaveCount(0);
    await expect(mapPage.bookmarkList).toContainText('No bookmarks for this device yet');
    const del = (await bookmarkRequests(authedPage)).find((r) => r.method === 'DELETE');
    expect(del?.url).toContain('/api/trail-bookmarks/1');
  });
});

test.describe('Trail bookmarks when switching devices', () => {
  test('hides the previous device bookmarks while the next list loads', async ({ authedPage }) => {
    await authedPage.evaluate(() => localStorage.removeItem('motus_trail_range'));
    await mockBookmarkApi(authedPage, [zugspitze], { delayBookmarksForDevice: OTHER_DEVICE_ID });
    const mapPage = new MapPage(authedPage);
    await mapPage.goto();

    await mapPage.deviceItem('Hiking Watch').click();
    await expect(mapPage.bookmarkItems).toHaveCount(1);
    await expect(mapPage.bookmarkOpenButton('Zugspitze')).toBeVisible();

    // The second device's bookmark list is held back by the mock.
    await mapPage.deviceItem('Bike Tracker').click();
    await expect(mapPage.detailPanel).toContainText('Bike Tracker');
    await expect(mapPage.bookmarkList).toContainText('Loading bookmarks');
    await expect(mapPage.bookmarkItems).toHaveCount(0);
    await expect(mapPage.bookmarkOpenButton('Zugspitze')).toHaveCount(0);

    await authedPage.evaluate(() =>
      (window as unknown as { __releaseBookmarks: () => void }).__releaseBookmarks(),
    );
    await expect(mapPage.bookmarkList).toContainText('No bookmarks for this device yet');
    await expect(mapPage.bookmarkItems).toHaveCount(0);
  });
});

test.describe('Bookmarks page', () => {
  test('lists bookmarks and opens one on the map', async ({ authedPage }) => {
    await mockBookmarkApi(authedPage, [zugspitze]);
    const bookmarksPage = new BookmarksPage(authedPage);
    await bookmarksPage.goto();

    const card = bookmarksPage.card('Zugspitze');
    await expect(card).toBeVisible();
    await expect(card).toContainText('Hiking Watch');
    await expect(card).toContainText('Via Höllental');
    await expect(card.locator('.bookmark-range')).toContainText('–');
    await expect(card.locator('.bookmark-duration')).toHaveText('8h 30m');

    const link = card.locator('a:has-text("Open on map")');
    const href = await link.getAttribute('href');
    const url = new URL(href!, 'http://localhost');
    expect(url.pathname).toBe('/map');
    expect(url.searchParams.get('device')).toBe(String(DEVICE_ID));
    expect(url.searchParams.get('from')).toBe(SEED_FROM);
    expect(url.searchParams.get('to')).toBe(SEED_TO);

    await link.click();
    await authedPage.waitForURL(/\/map\?/);
    const mapPage = new MapPage(authedPage);
    await mapPage.waitForMapLoad();
    await expect(mapPage.trailStatus).toContainText('3 points');
    const trail = await lastTrailRequest(authedPage);
    expect(trail.searchParams.get('from')).toBe(SEED_FROM);
    expect(trail.searchParams.get('to')).toBe(SEED_TO);
  });

  test('edits and deletes a bookmark', async ({ authedPage }) => {
    await mockBookmarkApi(authedPage, [zugspitze]);
    const bookmarksPage = new BookmarksPage(authedPage);
    await bookmarksPage.goto();

    await bookmarksPage.card('Zugspitze').locator('button:has-text("Edit")').click();
    await expect(bookmarksPage.dialog).toBeVisible();
    await bookmarksPage.nameInput.fill('Zugspitze 2026');
    await bookmarksPage.submit.click();
    await expect(bookmarksPage.dialog).toBeHidden();
    await expect(bookmarksPage.card('Zugspitze 2026')).toBeVisible();

    authedPage.once('dialog', (dialog) => dialog.accept());
    await bookmarksPage.card('Zugspitze 2026').locator('button:has-text("Delete")').click();
    await expect(bookmarksPage.cards).toHaveCount(0);
    const methods = (await bookmarkRequests(authedPage)).map((r) => r.method);
    expect(methods).toContain('PUT');
    expect(methods).toContain('DELETE');
  });

  test('filters bookmarks by search', async ({ authedPage }) => {
    await mockBookmarkApi(authedPage, [
      zugspitze,
      { ...zugspitze, id: 2, name: 'Watzmann', description: '', from: '2026-07-01T05:00:00.000Z', to: '2026-07-01T18:00:00.000Z' },
    ]);
    const bookmarksPage = new BookmarksPage(authedPage);
    await bookmarksPage.goto();
    await expect(bookmarksPage.cards).toHaveCount(2);
    await bookmarksPage.searchInput.fill('watz');
    await expect(bookmarksPage.cards).toHaveCount(1);
    await expect(bookmarksPage.card('Watzmann')).toBeVisible();
  });

  test('shows an empty state', async ({ authedPage }) => {
    await mockBookmarkApi(authedPage, []);
    const bookmarksPage = new BookmarksPage(authedPage);
    await bookmarksPage.goto();
    await expect(bookmarksPage.emptyState).toContainText('No bookmarks yet');
    await expect(bookmarksPage.emptyState.locator('a[href="/map"]')).toBeVisible();
  });

  test('is reachable from the navigation', async ({ authedPage }) => {
    await mockBookmarkApi(authedPage, []);
    await authedPage.goto('/');
    await authedPage.click('a.nav-link:has-text("Bookmarks")');
    await authedPage.waitForURL('/bookmarks');
    await expect(new BookmarksPage(authedPage).heading).toBeVisible();
  });
});
