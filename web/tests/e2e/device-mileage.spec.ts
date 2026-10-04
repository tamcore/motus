import { test, expect } from '../fixtures/auth-fixture';
import { mockFetch } from '../helpers/mock-fetch';
import { DevicesPage } from '../page-objects/DevicesPage';

const device = (id: number, name: string, mileage: number | null) => ({
  id, name, mileage, uniqueId: String(id), status: 'offline', disabled: false, attributes: {},
  createdAt: '2026-01-01T00:00:00Z', updatedAt: '2026-01-01T00:00:00Z',
});
const MOCK_DEVICES = [device(990101, 'Mileage Known', 1234), device(990102, 'Mileage Unknown', null)];

test.describe('Device Mileage Display', () => {
  let devicesPage: DevicesPage;

  test.beforeEach(async ({ authedPage }) => {
    await mockFetch(authedPage, [
      { path: '/api/devices', body: MOCK_DEVICES },
      { path: '/api/admin/devices', body: MOCK_DEVICES },
    ]);
    devicesPage = new DevicesPage(authedPage);
  });

  test('should show mileage in mobile device details', async ({ authedPage }) => {
    await authedPage.setViewportSize({ width: 390, height: 844 });
    await devicesPage.goto();

    for (const [name, value] of [['Mileage Known', /^(1,234 km|767 mi)$/], ['Mileage Unknown', /^—$/]] as const) {
      const card = authedPage.locator('.device-card', { hasText: name });
      await card.locator('.device-summary').click();
      await expect(card.locator('.detail-label:has-text("Mileage") ~ .detail-value')).toHaveText(value);
    }
  });

  test('should show mileage input in device edit form', async () => {
    await devicesPage.goto();
    await devicesPage.tableRows.filter({ hasText: 'Mileage Known' }).locator('button:has-text("Edit")').click();
    await expect(devicesPage.modal.locator('input[name="mileage"]')).toHaveValue(/^(1234|767)$/);
  });
});
