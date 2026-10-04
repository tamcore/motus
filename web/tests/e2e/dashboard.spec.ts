import { test, expect } from '../fixtures/auth-fixture';
import { mockFetch } from '../helpers/mock-fetch';
import { DashboardPage } from '../page-objects/DashboardPage';

test.describe('Dashboard', () => {
  let dashboard: DashboardPage;

  test.beforeEach(async ({ authedPage }) => {
    dashboard = new DashboardPage(authedPage);
    // Already on dashboard after auth
  });

  test('should display page title', async () => {
    await expect(dashboard.title).toContainText('Dashboard');
  });

  test('should have correct page title in tab', async ({ authedPage }) => {
    await expect(authedPage).toHaveTitle('Dashboard - Motus');
  });

  test('should show four stat cards', async () => {
    await expect(dashboard.statCards).toHaveCount(4);
  });

  test('should show Total Devices stat', async () => {
    await expect(dashboard.totalDevicesStat).toBeVisible();
    const value = await dashboard.getStatValue('Total Devices');
    expect(parseInt(value)).toBeGreaterThanOrEqual(0);
  });

  test('should show Online stat', async () => {
    await expect(dashboard.onlineStat).toBeVisible();
  });

  test('should show Offline stat', async () => {
    await expect(dashboard.offlineStat).toBeVisible();
  });

  test('should show Positions Today stat', async () => {
    await expect(dashboard.positionsStat).toBeVisible();
  });

  test('should show All Devices section', async () => {
    await expect(dashboard.sectionTitle).toContainText('All Devices');
  });

  test('should navigate to devices page from empty state', async ({ authedPage }) => {
    await mockFetch(authedPage, [
      { path: '/api/devices', body: [] },
      { path: '/api/admin/devices', body: [] },
      { path: '/api/positions', body: [] },
      { path: '/api/admin/positions', body: [] },
    ]);
    await authedPage.reload();
    await authedPage.locator('a:has-text("Add your first device")').click();
    await authedPage.waitForURL('/devices');
  });

  test('should show device cards linking to map', async ({ authedPage }) => {
    // Wait for data loading to complete (device-count badge only appears after loading)
    await authedPage.waitForSelector('.device-count', { timeout: 10000 });
    const cards = authedPage.locator('a.device-card');
    const count = await cards.count();
    if (count > 0) {
      const href = await cards.first().getAttribute('href');
      expect(href).toContain('/map?device=');
    }
  });

  test('should display navigation bar with all links', async ({ authedPage }) => {
    const nav = authedPage.locator('nav.navbar');
    await expect(nav).toBeVisible();
    await expect(nav.locator('a:has-text("Dashboard")')).toBeVisible();
    await expect(nav.locator('a:has-text("Devices")')).toBeVisible();
    await expect(nav.getByRole('link', { name: 'Map', exact: true })).toBeVisible();
    await expect(nav.locator('a:has-text("Reports")')).toBeVisible();
    await expect(nav.locator('a:has-text("Geofences")')).toBeVisible();
    await expect(nav.locator('a:has-text("Notifications")')).toBeVisible();
  });

  test('should show active state on Dashboard nav link', async ({ authedPage }) => {
    const dashLink = authedPage.locator('a.nav-link:has-text("Dashboard")');
    await expect(dashLink).toHaveClass(/active/);
  });

  test('should show theme switcher', async ({ authedPage }) => {
    const switcher = authedPage.locator('.theme-switcher');
    await expect(switcher).toBeVisible();
  });
});
