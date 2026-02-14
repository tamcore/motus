import { test, expect } from '../fixtures/auth-fixture';
import { DevicesPage } from '../page-objects/DevicesPage';

test.describe('Device Mileage Display', () => {
  let devicesPage: DevicesPage;

  test.beforeEach(async ({ authedPage }) => {
    devicesPage = new DevicesPage(authedPage);
    await devicesPage.goto();
  });

  test('should show mileage field in device details', async ({ authedPage }) => {
    // Expand the first device
    const firstRow = devicesPage.tableRows.first();
    const rowCount = await devicesPage.tableRows.count();
    if (rowCount === 0) return;

    await firstRow.locator('.device-summary').click();

    // Wait for detail panel to expand
    const detailPanel = authedPage.locator('.device-detail').first();
    await expect(detailPanel).toBeVisible({ timeout: 5000 });

    // The Mileage label should always be present
    const mileageLabel = detailPanel.locator('.detail-label:has-text("Mileage")');
    await expect(mileageLabel).toBeVisible();

    // The value should be either a formatted mileage or "—"
    const mileageValue = mileageLabel.locator('~ .detail-value');
    await expect(mileageValue).toBeVisible();
    const text = await mileageValue.textContent();
    expect(text).toBeTruthy();
    // Should be either "—" or contain "km" or "mi"
    expect(text!.trim()).toMatch(/—|km|mi/);
  });

  test('should show mileage input in device edit form', async ({ authedPage }) => {
    // Expand the first device
    const rowCount = await devicesPage.tableRows.count();
    if (rowCount === 0) return;

    await devicesPage.tableRows.first().locator('.device-summary').click();

    // Wait for details to expand, then click Edit
    const detailPanel = authedPage.locator('.device-detail').first();
    await expect(detailPanel).toBeVisible({ timeout: 5000 });

    const editBtn = detailPanel.locator('button:has-text("Edit")');
    await editBtn.click();

    // The edit modal should have a mileage input
    await expect(devicesPage.modal).toBeVisible({ timeout: 5000 });
    const mileageInput = authedPage.locator('[role="dialog"] input[name="mileage"]');
    await expect(mileageInput).toBeVisible();
  });
});
