import { test, expect, type BrowserContext, type Locator, type Page } from '@playwright/test';
import { GeofencesPage } from '../page-objects/GeofencesPage';
import { MapPage } from '../page-objects/MapPage';

const START = { latitude: 52.52, longitude: 13.405 };
const MOVED = { latitude: 52.53, longitude: 13.42 };

test.use({ storageState: '.auth/user.json', permissions: ['geolocation'], geolocation: START });

async function expectMarkerFollowsFix(context: BrowserContext, page: Page, locate: Locator) {
  await locate.click();
  await expect(locate).toHaveClass(/active/);
  const marker = page.locator('.user-location-marker');
  await expect(marker).toHaveCount(1);
  // The first fix may pan the map; let it settle before moving.
  await page.waitForTimeout(500);
  const settled = await marker.boundingBox();
  expect(settled).toBeTruthy();

  await context.setGeolocation(MOVED);
  await expect
    .poll(async () => {
      const b = await marker.boundingBox();
      return b ? Math.hypot(b.x - settled!.x, b.y - settled!.y) : 0;
    })
    .toBeGreaterThan(5);

  await locate.click();
  await expect(marker).toHaveCount(0);
}

test.describe('User location marker', () => {
  test('follows geolocation fixes on /map', async ({ context, page }) => {
    const map = new MapPage(page);
    await map.goto();
    await expectMarkerFollowsFix(context, page, map.locateButton);
  });

  test('follows geolocation fixes on /geofences', async ({ context, page }) => {
    const geofences = new GeofencesPage(page);
    await geofences.goto();
    await expectMarkerFollowsFix(context, page, geofences.locateButton);
  });

  test('follows geolocation fixes on a share page', async ({ context, page }) => {
    const csrf = (await page.request.get('/api/session')).headers()['x-csrf-token'] ?? '';
    const headers = { 'X-CSRF-Token': csrf };
    const deviceRes = await page.request.post('/api/devices', {
      headers,
      data: { name: 'PW Share Locate', uniqueId: `pw-locate-${Date.now()}` },
    });
    expect(deviceRes.ok()).toBeTruthy();
    const device = await deviceRes.json();
    try {
      const shareRes = await page.request.post(`/api/devices/${device.id}/share`, { headers, data: {} });
      expect(shareRes.status()).toBe(201);
      const { token } = await shareRes.json();

      await page.goto(`/share/${token}`);
      await page.waitForSelector('.leaflet-container', { state: 'visible', timeout: 15000 });
      await expectMarkerFollowsFix(context, page, page.getByRole('button', { name: /my location/ }));
    } finally {
      await page.request.delete(`/api/devices/${device.id}`, { headers });
    }
  });
});
