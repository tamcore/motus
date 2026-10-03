import type { Page } from '@playwright/test';
import { test, expect } from '../fixtures/auth-fixture';
import { DevicesPage } from '../page-objects/DevicesPage';
import { DashboardPage } from '../page-objects/DashboardPage';
import { MapPage } from '../page-objects/MapPage';

// Devices with a full, a low and an unknown battery level. IDs are high to
// avoid clashing with real seeded devices.
const MOCK_DEVICES = [
  { id: 990001, name: 'Battery Full', uniqueId: '990001', status: 'online', disabled: false, batteryLevel: 87, lastUpdate: new Date().toISOString(), attributes: {}, createdAt: '2026-01-01T00:00:00Z', updatedAt: '2026-01-01T00:00:00Z' },
  { id: 990002, name: 'Battery Low', uniqueId: '990002', status: 'offline', disabled: false, batteryLevel: 12, lastUpdate: new Date().toISOString(), attributes: {}, createdAt: '2026-01-01T00:00:00Z', updatedAt: '2026-01-01T00:00:00Z' },
  { id: 990003, name: 'Battery Unknown', uniqueId: '990003', status: 'offline', disabled: false, batteryLevel: null, attributes: {}, createdAt: '2026-01-01T00:00:00Z', updatedAt: '2026-01-01T00:00:00Z' },
];

/** Serve MOCK_DEVICES for the device list endpoints (user and admin scope). */
async function mockDevices(page: Page) {
  // page.route() does not reliably intercept SvelteKit fetches; patch fetch instead.
  await page.addInitScript((devices) => {
    const origFetch = window.fetch;
    window.fetch = async function (input: RequestInfo | URL, init?: RequestInit) {
      const url = typeof input === 'string' ? input : input instanceof URL ? input.href : input.url;
      const path = new URL(url, window.location.origin).pathname;
      if (path === '/api/devices' || path === '/api/admin/devices') {
        return new Response(JSON.stringify(devices), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        });
      }
      if (path === '/api/positions' || path === '/api/admin/positions') {
        return new Response('[]', { status: 200, headers: { 'Content-Type': 'application/json' } });
      }
      return origFetch.apply(globalThis, [input, init] as Parameters<typeof fetch>);
    } as typeof fetch;
  }, MOCK_DEVICES);
}

test.describe('Device battery level', () => {
  test('devices table shows battery percentage with low-battery indication', async ({ authedPage }) => {
    await mockDevices(authedPage);
    const devicesPage = new DevicesPage(authedPage);
    await devicesPage.goto();

    // Scope to the desktop table (mobile cards render the same data).
    const table = devicesPage.table;
    await expect(table.locator('th', { hasText: 'Battery' })).toBeVisible();

    const full = devicesPage.batteryCell('Battery Full');
    await expect(full).toContainText('87%');
    await expect(full.locator('.battery')).not.toHaveClass(/battery-low/);

    const low = devicesPage.batteryCell('Battery Low');
    await expect(low).toContainText('12%');
    await expect(low.locator('.battery')).toHaveClass(/battery-low/);
    await expect(low.locator('.battery')).toHaveAttribute('aria-label', 'Battery 12%, low');

    const unknown = devicesPage.batteryCell('Battery Unknown');
    await expect(unknown).toHaveText('—');
    await expect(unknown.locator('.battery')).toHaveCount(0);
  });

  test('dashboard device cards show battery level', async ({ authedPage }) => {
    await mockDevices(authedPage);
    const dashboard = new DashboardPage(authedPage);
    await authedPage.goto('/');

    const lowCard = dashboard.deviceCards.filter({ hasText: 'Battery Low' });
    await expect(lowCard.locator('.battery.battery-low')).toContainText('12%');

    const unknownCard = dashboard.deviceCards.filter({ hasText: 'Battery Unknown' });
    await expect(unknownCard).toBeVisible();
    await expect(unknownCard.locator('.battery')).toHaveCount(0);
  });

  test('map sidebar shows battery level', async ({ authedPage }) => {
    await mockDevices(authedPage);
    const mapPage = new MapPage(authedPage);
    await authedPage.goto('/map');

    const fullItem = mapPage.deviceItems.filter({ hasText: 'Battery Full' });
    await expect(fullItem.locator('.battery')).toContainText('87%');
    const lowItem = mapPage.deviceItems.filter({ hasText: 'Battery Low' });
    await expect(lowItem.locator('.battery.battery-low')).toContainText('12%');
  });
});
