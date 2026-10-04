import { test, expect } from '../fixtures/auth-fixture';
import { mockFetch } from '../helpers/mock-fetch';

test.describe('Error Handling', () => {
  test('should show error on dashboard when API returns 500', async ({ authedPage }) => {
    await mockFetch(authedPage, [{ path: '/api/devices', status: 500, text: 'Internal Server Error' }]);
    await authedPage.goto('/');
    // App should not crash - dashboard should still render
    await expect(authedPage.locator('h1:has-text("Dashboard")')).toBeVisible();
    // The user must see that loading failed, distinct from the empty state
    const banner = authedPage.locator('.error-banner[role="alert"]');
    await expect(banner).toBeVisible();
    await expect(banner).toContainText(/failed to load/i);
    await expect(authedPage.locator('text=No devices yet')).not.toBeVisible();
  });

  for (const [name, route] of [
    ['500', { status: 500, text: 'Internal Server Error' }],
    ['malformed JSON', { text: 'not json' }],
  ] as const) {
    test(`should leave the loading state when devices API returns ${name}`, async ({ authedPage }) => {
      await mockFetch(authedPage, [{ path: '/api/devices', ...route }]);
      await authedPage.goto('/devices');
      await expect(authedPage.locator('h1:has-text("Devices")')).toBeVisible();
      await expect(authedPage.locator('.empty-state')).toBeVisible();
    });
  }

  test('should handle devices API returning empty array', async ({ authedPage }) => {
    await mockFetch(authedPage, [{ path: '/api/devices', body: [] }]);
    await authedPage.goto('/devices');
    await expect(authedPage.locator('.empty-state')).toBeVisible({ timeout: 15000 });
    await expect(authedPage.locator('.empty-state')).toContainText('No devices');
  });

  test('should initialize the map when device and position APIs fail', async ({ authedPage }) => {
    await mockFetch(authedPage, [
      { path: '/api/devices', status: 500, text: 'Internal Server Error' },
      { match: '/api/positions', status: 404, text: 'Not Found' },
    ]);
    await authedPage.goto('/map');
    await expect(authedPage.locator('.leaflet-container').first()).toBeVisible();
    await expect(authedPage.locator('.leaflet-tile-pane')).toBeAttached();
  });

  test('should handle session expiry by redirecting to login', async ({ authedPage }) => {
    // Clear session cookie to simulate expiry
    await authedPage.context().clearCookies();
    await authedPage.evaluate(() => {
      localStorage.setItem('motus_authenticated', 'false');
      localStorage.setItem('motus_user', 'null');
    });
    await authedPage.goto('/');
    await authedPage.waitForURL(/\/login/, { timeout: 10000 });
    await expect(authedPage.locator('h1:has-text("Motus")')).toBeVisible();
  });

  test('should handle reports API failure', async ({ authedPage }) => {
    await mockFetch(authedPage, [{ match: '/api/reports/activity', status: 500, text: 'Server Error' }]);
    await authedPage.goto('/reports');
    await authedPage.waitForSelector('h1:has-text("Reports")');

    await authedPage.click('button:has-text("Apply")');
    await authedPage.waitForTimeout(2000);
    // Should not crash, empty state should show
    await expect(authedPage.locator('.empty-state')).toBeVisible();
  });
});
