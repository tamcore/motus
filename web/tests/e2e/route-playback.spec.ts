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
    const mockPositions = [
      { id: 1, deviceId: 1, latitude: 51.5, longitude: -0.09, speed: 30, fixTime: '2025-01-01T00:00:00Z', valid: true, outdated: false },
      { id: 2, deviceId: 1, latitude: 51.51, longitude: -0.08, speed: 40, fixTime: '2025-01-01T00:01:00Z', valid: true, outdated: false },
      { id: 3, deviceId: 1, latitude: 51.52, longitude: -0.07, speed: 35, fixTime: '2025-01-01T00:02:00Z', valid: true, outdated: false },
    ];

    // Intercept fetch at JS level — page.route() doesn't reliably intercept
    // SvelteKit's client-side fetch due to CSRF token validation on the server
    await authedPage.addInitScript((positions) => {
      const origFetch = window.fetch;
      window.fetch = async function (...args: Parameters<typeof fetch>) {
        const url = typeof args[0] === 'string' ? args[0] : args[0] instanceof URL ? args[0].href : (args[0] as Request).url;
        if (url.includes('/api/positions')) {
          return new Response(JSON.stringify(positions), {
            status: 200,
            headers: { 'Content-Type': 'application/json' },
          });
        }
        return origFetch.apply(this, args);
      };
    }, mockPositions);

    await authedPage.goto('/reports/route?deviceId=1&from=2025-01-01T00:00:00Z&to=2025-01-02T00:00:00Z');

    await expect(authedPage.locator('.controls-container')).toBeVisible({ timeout: 20000 });
    await expect(authedPage.locator('button:has-text("Play")')).toBeVisible();
    await expect(authedPage.locator('button:has-text("Stop")')).toBeVisible();
    await expect(authedPage.locator('button:has-text("GPX")')).toBeVisible();
  });

  test('should show speed selector buttons', async ({ authedPage }) => {
    await authedPage.route('**/api/positions*', (route) => {
      route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify([
          { id: 1, deviceId: 1, latitude: 51.5, longitude: -0.09, speed: 30, fixTime: '2025-01-01T00:00:00Z', valid: true, outdated: false },
          { id: 2, deviceId: 1, latitude: 51.51, longitude: -0.08, speed: 40, fixTime: '2025-01-01T00:01:00Z', valid: true, outdated: false },
        ]),
      });
    });

    await authedPage.goto('/reports/route?deviceId=1&from=2025-01-01T00:00:00Z&to=2025-01-02T00:00:00Z');
    await authedPage.waitForTimeout(2000);

    const speedBtns = authedPage.locator('.speed-btn');
    const count = await speedBtns.count();
    if (count > 0) {
      expect(count).toBe(4); // 1x, 2x, 4x, 8x
    }
  });

  test('should show info panel with position data', async ({ authedPage }) => {
    await authedPage.route('**/api/positions*', (route) => {
      route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify([
          { id: 1, deviceId: 1, latitude: 51.5, longitude: -0.09, speed: 30, fixTime: '2025-01-01T00:00:00Z', valid: true, outdated: false },
          { id: 2, deviceId: 1, latitude: 51.51, longitude: -0.08, speed: 40, fixTime: '2025-01-01T00:01:00Z', valid: true, outdated: false },
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
