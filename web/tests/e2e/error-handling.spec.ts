import { test, expect } from '../fixtures/auth-fixture';

test.describe('Error Handling', () => {
  test('should show error on dashboard when API returns 500', async ({ authedPage }) => {
    await authedPage.route('**/api/devices', (route) => {
      route.fulfill({ status: 500, body: 'Internal Server Error' });
    });
    await authedPage.route('**/api/positions*', (route) => {
      route.fulfill({ status: 500, body: 'Internal Server Error' });
    });
    await authedPage.goto('/');
    await authedPage.waitForTimeout(2000);
    // App should not crash - dashboard should still render
    await expect(authedPage.locator('h1:has-text("Dashboard")')).toBeVisible();
  });

  test('should handle devices API 500 gracefully', async ({ authedPage }) => {
    await authedPage.route('**/api/devices', (route) => {
      route.fulfill({ status: 500, body: 'Internal Server Error' });
    });
    await authedPage.goto('/devices');
    await authedPage.waitForTimeout(2000);
    // Page should still render
    await expect(authedPage.locator('h1:has-text("Devices")')).toBeVisible();
  });

  test('should handle devices API returning empty array', async ({ authedPage }) => {
    // Use addInitScript to intercept fetch at JS level (more reliable than page.route with SvelteKit)
    await authedPage.addInitScript(() => {
      const origFetch = window.fetch;
      window.fetch = async function (input: RequestInfo | URL, init?: RequestInit) {
        const url = typeof input === 'string' ? input : input instanceof URL ? input.href : input.url;
        if (url.includes('/api/devices') && !url.includes('/api/devices/')) {
          return new Response('[]', {
            status: 200,
            headers: { 'Content-Type': 'application/json' },
          });
        }
        return origFetch.apply(globalThis, [input, init] as Parameters<typeof fetch>);
      } as typeof fetch;
    });
    await authedPage.goto('/devices');
    await expect(authedPage.locator('.empty-state')).toBeVisible({ timeout: 15000 });
    await expect(authedPage.locator('.empty-state')).toContainText('No devices');
  });

  test('should handle network timeout on map page', async ({ authedPage }) => {
    await authedPage.route('**/api/devices', (route) => {
      route.abort('timedout');
    });
    await authedPage.goto('/map');
    await authedPage.waitForTimeout(2000);
    // Map should still initialize even if API fails
    await expect(authedPage.locator('.leaflet-container').first()).toBeVisible();
  });

  test('should handle 404 response for positions', async ({ authedPage }) => {
    await authedPage.route('**/api/positions*', (route) => {
      route.fulfill({ status: 404, body: 'Not Found' });
    });
    await authedPage.goto('/map');
    await authedPage.waitForTimeout(2000);
    // Map should still load without positions
    await expect(authedPage.locator('.leaflet-container').first()).toBeVisible();
  });

  test('should handle malformed JSON response', async ({ authedPage }) => {
    await authedPage.route('**/api/devices', (route) => {
      route.fulfill({ status: 200, body: 'not json', contentType: 'application/json' });
    });
    await authedPage.goto('/devices');
    await authedPage.waitForTimeout(2000);
    // Page should still render without crashing
    await expect(authedPage.locator('h1:has-text("Devices")')).toBeVisible();
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

  test('should handle concurrent API failures', async ({ authedPage }) => {
    await authedPage.route('**/api/devices', (route) => {
      route.fulfill({ status: 503, body: 'Service Unavailable' });
    });
    await authedPage.route('**/api/positions*', (route) => {
      route.fulfill({ status: 503, body: 'Service Unavailable' });
    });
    await authedPage.goto('/');
    await authedPage.waitForTimeout(2000);
    // Dashboard should still render with stat cards showing
    await expect(authedPage.locator('h1:has-text("Dashboard")')).toBeVisible();
  });

  test('should handle slow API response', async ({ authedPage }) => {
    await authedPage.route('**/api/devices', async (route) => {
      await new Promise((r) => setTimeout(r, 3000));
      route.fulfill({
        status: 200,
        body: '[]',
        contentType: 'application/json',
      });
    });
    await authedPage.goto('/devices');
    // During slow load, skeleton should be visible
    // After load completes, page should show
    await authedPage.waitForTimeout(4000);
    await expect(authedPage.locator('h1:has-text("Devices")')).toBeVisible();
  });

  test('should handle reports API failure', async ({ authedPage }) => {
    await authedPage.goto('/reports');
    await authedPage.waitForSelector('h1:has-text("Reports")');

    await authedPage.route('**/api/positions*', (route) => {
      route.fulfill({ status: 500, body: 'Server Error' });
    });
    await authedPage.click('button:has-text("Apply")');
    await authedPage.waitForTimeout(2000);
    // Should not crash, empty state should show
    await expect(authedPage.locator('.empty-state')).toBeVisible();
  });
});
