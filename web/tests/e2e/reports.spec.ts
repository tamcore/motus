import { test, expect } from '../fixtures/auth-fixture';
import { ReportsPage } from '../page-objects/ReportsPage';

test.describe('Reports Page', () => {
  let reportsPage: ReportsPage;

  test.beforeEach(async ({ authedPage }) => {
    reportsPage = new ReportsPage(authedPage);
    await reportsPage.goto();
  });

  test('should display reports page title', async () => {
    await reportsPage.expectLoaded();
  });

  test('should have correct page title in tab', async ({ authedPage }) => {
    await expect(authedPage).toHaveTitle('Reports - Motus');
  });

  test('should show device selector', async () => {
    await expect(reportsPage.deviceSelect).toBeVisible();
  });

  test('should populate device selector with options', async () => {
    const options = reportsPage.deviceSelect.locator('option');
    const count = await options.count();
    // At least "All Devices" option
    expect(count).toBeGreaterThanOrEqual(1);
    await expect(options.first()).toContainText('All Devices');
  });

  test('should show date range preset buttons', async () => {
    await expect(reportsPage.presetButtons).toHaveCount(4);
  });

  test('should highlight Last 7d as default preset', async ({ authedPage }) => {
    const weekBtn = authedPage.locator('.preset-btn:has-text("Last 7d")');
    await expect(weekBtn).toHaveClass(/active/);
  });

  test('should switch date preset to Last 24h', async ({ authedPage }) => {
    await reportsPage.selectPreset('Last 24h');
    const dayBtn = authedPage.locator('.preset-btn:has-text("Last 24h")');
    await expect(dayBtn).toHaveClass(/active/);
  });

  test('should switch date preset to Last 30d', async ({ authedPage }) => {
    await reportsPage.selectPreset('Last 30d');
    const monthBtn = authedPage.locator('.preset-btn:has-text("Last 30d")');
    await expect(monthBtn).toHaveClass(/active/);
  });

  test('should show custom date inputs when Custom is selected', async () => {
    await reportsPage.selectPreset('Custom');
    await expect(reportsPage.customFromInput).toBeVisible();
    await expect(reportsPage.customToInput).toBeVisible();
  });

  test('should hide custom date inputs when non-custom preset selected', async () => {
    await reportsPage.selectPreset('Custom');
    await expect(reportsPage.customFromInput).toBeVisible();

    await reportsPage.selectPreset('Last 7d');
    await expect(reportsPage.customFromInput).toHaveCount(0);
  });

  test('should show Apply button', async () => {
    await expect(reportsPage.applyButton).toBeVisible();
  });

  test('should show Trips tab as active by default', async () => {
    await expect(reportsPage.tripsTab).toHaveClass(/active/);
  });

  test('should show all three tabs', async () => {
    await expect(reportsPage.tripsTab).toBeVisible();
    await expect(reportsPage.stopsTab).toBeVisible();
    await expect(reportsPage.summaryTab).toBeVisible();
  });

  test('should switch to Stops tab', async () => {
    await reportsPage.stopsTab.click();
    await expect(reportsPage.stopsTab).toHaveClass(/active/);
  });

  test('should switch to Summary tab', async () => {
    await reportsPage.summaryTab.click();
    await expect(reportsPage.summaryTab).toHaveClass(/active/);
  });

  test('should show tabs with proper ARIA roles', async ({ authedPage }) => {
    const tablist = authedPage.locator('[role="tablist"]');
    await expect(tablist).toBeVisible();
    const tabs = authedPage.locator('[role="tab"]');
    await expect(tabs).toHaveCount(3);
  });

  test('should show empty state before applying filters', async () => {
    await expect(reportsPage.emptyState).toBeVisible();
    await expect(reportsPage.emptyState).toContainText('No trips detected');
  });

  test('should show hint text in empty state', async ({ authedPage }) => {
    await expect(authedPage.locator('.empty-hint')).toContainText(
      'Select a device and date range',
    );
  });

  test('should attempt to fetch reports when Apply is clicked', async ({ authedPage }) => {
    // Wait for loading to finish (device select becomes visible)
    await authedPage.waitForSelector('#device-select', { timeout: 15000 });
    // Wait for initial auto-fetch to complete before clicking Apply
    await authedPage.waitForLoadState('networkidle');

    const responsePromise = authedPage.waitForResponse(
      (resp) => resp.url().includes('/api/positions'),
      { timeout: 15000 },
    );
    await reportsPage.applyButton.click();
    const response = await responsePromise;
    expect(response.status()).toBe(200);
  });
});
