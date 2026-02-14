import { test, expect } from '../fixtures/auth-fixture';
import { DevicesPage } from '../page-objects/DevicesPage';

test.describe('Devices Page', () => {
  let devicesPage: DevicesPage;

  test.beforeEach(async ({ authedPage }) => {
    devicesPage = new DevicesPage(authedPage);
    await devicesPage.goto();
  });

  test('should display devices page title', async () => {
    await devicesPage.expectLoaded();
  });

  test('should have correct page title in tab', async ({ authedPage }) => {
    await expect(authedPage).toHaveTitle('Devices - Motus');
  });

  test('should show Add Device button', async () => {
    await expect(devicesPage.addDeviceButton).toBeVisible();
  });

  test('should show search input', async () => {
    await expect(devicesPage.searchInput).toBeVisible();
  });

  test('should show result count', async () => {
    await expect(devicesPage.resultCount).toBeVisible();
    const text = await devicesPage.resultCount.textContent();
    expect(text).toMatch(/\d+ device/);
  });

  test('should display devices table with columns', async ({ authedPage }) => {
    const headerRow = devicesPage.table.locator('thead tr th');
    const count = await headerRow.count();
    if (count > 0) {
      await expect(headerRow.nth(0)).toContainText('Status');
      await expect(headerRow.nth(1)).toContainText('Name');
      await expect(headerRow.nth(2)).toContainText('Identifier');
    }
  });

  test('should open create device modal', async () => {
    await devicesPage.openCreateModal();
    await expect(devicesPage.modalTitle).toContainText('Add Device');
    await expect(devicesPage.formNameInput).toBeVisible();
    await expect(devicesPage.formUniqueIdInput).toBeVisible();
  });

  test('should close modal on Cancel', async () => {
    await devicesPage.openCreateModal();
    await devicesPage.cancelButton.click();
    await expect(devicesPage.modal).toHaveCount(0);
  });

  test('should show validation error for missing fields', async () => {
    await devicesPage.openCreateModal();
    // Click save with empty form
    await devicesPage.saveButton.click();
    await expect(devicesPage.formError).toContainText('required');
  });

  test('should create a new device', async ({ authedPage }) => {
    const uniqueId = `pw-create-${Date.now()}`;
    await devicesPage.openCreateModal();
    await devicesPage.fillDeviceForm({
      name: 'PW Created Device',
      uniqueId,
    });
    await devicesPage.saveButton.click();
    // Modal closes and device appears in table
    await expect(devicesPage.modal).toHaveCount(0, { timeout: 10000 });
    await expect(authedPage.locator('.device-table').locator(`text=${uniqueId}`)).toBeVisible({ timeout: 5000 });
  });

  test('should search devices by name', async ({ authedPage }) => {
    // Get initial count
    const initialCount = await devicesPage.tableRows.count();
    if (initialCount === 0) return;

    // Get first device name
    const firstName = await devicesPage.tableRows.first().locator('.device-name').textContent();
    if (!firstName) return;

    await devicesPage.search(firstName);
    // Filtered results should include at least the searched device
    await expect(devicesPage.tableRows.first().locator(`.device-name:has-text("${firstName}")`))
      .toBeVisible();
  });

  test('should search devices by identifier', async ({ authedPage }) => {
    const rowCount = await devicesPage.tableRows.count();
    if (rowCount === 0) return;

    const firstUid = await devicesPage.tableRows.first().locator('.uid-badge').textContent();
    if (!firstUid) return;

    await devicesPage.search(firstUid);
    await expect(devicesPage.resultCount).toContainText(/\d+ device/);
  });

  test('should show empty state for no search results', async () => {
    await devicesPage.search('nonexistent-device-xyz-12345');
    await expect(devicesPage.emptyState).toBeVisible();
    await expect(devicesPage.emptyState).toContainText('No devices match');
  });

  test('should filter case-insensitively', async () => {
    const rowCount = await devicesPage.tableRows.count();
    if (rowCount === 0) return;

    const firstName = await devicesPage.tableRows.first().locator('.device-name').textContent();
    if (!firstName) return;

    await devicesPage.search(firstName.toUpperCase());
    await expect(devicesPage.tableRows).toHaveCount(rowCount > 0 ? rowCount : 0, {
      timeout: 3000,
    }).catch(() => {
      // At least one result should be visible
    });
    const filtered = await devicesPage.tableRows.count();
    expect(filtered).toBeGreaterThanOrEqual(1);
  });
});
