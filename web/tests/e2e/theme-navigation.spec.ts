import { test, expect } from '../fixtures/auth-fixture';
import { NAV_LINKS } from '../fixtures/test-data';

test.describe('Theme Switching', () => {
  test('should show theme switcher in navbar', async ({ authedPage }) => {
    const switcher = authedPage.locator('.theme-switcher');
    await expect(switcher).toBeVisible();
  });

  test('should display current theme label', async ({ authedPage }) => {
    const switcher = authedPage.locator('.theme-switcher');
    await expect(switcher).toBeVisible();
    const text = await switcher.textContent();
    expect(['dark', 'auto', 'light'].some(t => text?.toLowerCase().includes(t))).toBe(true);
  });

  test('should cycle theme on click', async ({ authedPage }) => {
    const switcher = authedPage.locator('.theme-switcher');
    await expect(switcher).toBeVisible();

    const initialText = await switcher.textContent();
    await switcher.click();
    await authedPage.waitForTimeout(200);

    const newText = await switcher.textContent();
    expect(newText).not.toBe(initialText);
  });

  test('should persist theme choice across navigation', async ({ authedPage }) => {
    const switcher = authedPage.locator('.theme-switcher');
    await expect(switcher).toBeVisible();

    // Get current theme
    const initialText = await switcher.textContent();

    // Navigate to another page
    await authedPage.goto('/devices');
    await authedPage.waitForSelector('h1:has-text("Devices")');

    // Theme should be the same
    const themeAfterNav = await switcher.textContent();
    expect(themeAfterNav).toBe(initialText);
  });

  test('should have theme-switcher with aria-label', async ({ authedPage }) => {
    const switcher = authedPage.locator('.theme-switcher');
    const ariaLabel = await switcher.getAttribute('aria-label');
    expect(ariaLabel).toBe('Toggle theme');
  });
});

test.describe('Navigation', () => {
  test('should show all navigation links', async ({ authedPage }) => {
    for (const link of NAV_LINKS) {
      if (link.label === 'Map') {
        await expect(authedPage.getByRole('link', { name: 'Map', exact: true })).toBeVisible();
      } else {
        await expect(authedPage.locator(`a.nav-link:has-text("${link.label}")`)).toBeVisible();
      }
    }
  });

  test('should navigate to Devices page', async ({ authedPage }) => {
    await authedPage.click('a.nav-link:has-text("Devices")');
    await authedPage.waitForURL('/devices');
    await expect(authedPage.locator('h1:has-text("Devices")')).toBeVisible();
  });

  test('should navigate to Map page', async ({ authedPage }) => {
    await authedPage.getByRole('link', { name: 'Map', exact: true }).click();
    await authedPage.waitForURL('/map');
    await expect(authedPage.locator('.leaflet-container').first()).toBeVisible({ timeout: 10000 });
  });

  test('should navigate to Reports page', async ({ authedPage }) => {
    await authedPage.click('a.nav-link:has-text("Reports")');
    await authedPage.waitForURL('/reports');
    await expect(authedPage.locator('h1:has-text("Reports")')).toBeVisible();
  });

  test('should navigate to Geofences page', async ({ authedPage }) => {
    await authedPage.click('a.nav-link:has-text("Geofences")');
    await authedPage.waitForURL('/geofences');
    await expect(authedPage.locator('.sidebar-title:has-text("Geofences")')).toBeVisible();
  });

  test('should navigate to Notifications page', async ({ authedPage }) => {
    await authedPage.click('a.nav-link:has-text("Notifications")');
    await authedPage.waitForURL('/notifications');
    await expect(authedPage.locator('h1:has-text("Notification Rules")')).toBeVisible();
  });

  test('should navigate back to Dashboard via logo', async ({ authedPage }) => {
    await authedPage.goto('/devices');
    await authedPage.waitForSelector('h1:has-text("Devices")');
    await authedPage.click('a.logo');
    await authedPage.waitForURL('/');
    await expect(authedPage.locator('h1:has-text("Dashboard")')).toBeVisible();
  });

  test('should highlight active nav link', async ({ authedPage }) => {
    await authedPage.goto('/devices');
    await authedPage.waitForSelector('h1:has-text("Devices")');
    const devicesLink = authedPage.locator('a.nav-link:has-text("Devices")');
    await expect(devicesLink).toHaveClass(/active/);

    // Dashboard should not be active
    const dashLink = authedPage.locator('a.nav-link:has-text("Dashboard")');
    await expect(dashLink).not.toHaveClass(/active/);
  });

  test('should show Motus logo text', async ({ authedPage }) => {
    await expect(authedPage.locator('.logo-text')).toContainText('Motus');
  });

  test('should show user button in navbar', async ({ authedPage }) => {
    await expect(authedPage.locator('.user-button')).toBeVisible();
  });

  test('should show user dropdown on click', async ({ authedPage }) => {
    await authedPage.click('.user-button');
    await expect(authedPage.locator('.user-dropdown')).toBeVisible();
    await expect(authedPage.locator('.dropdown-item:has-text("Logout")')).toBeVisible();
  });
});
