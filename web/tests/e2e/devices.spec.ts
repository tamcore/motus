import type { Locator, Page } from '@playwright/test';
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

  for (const [label, data] of [
    ['empty form', {}],
    ['name only', { name: 'Test Device' }],
    ['identifier only', { uniqueId: 'test-123' }],
    ['whitespace name', { name: '   ', uniqueId: `ws-test-${Date.now()}` }],
  ] as const) {
    test(`should show required error for ${label}`, async () => {
      await devicesPage.openCreateModal();
      await devicesPage.fillDeviceForm(data);
      await devicesPage.saveButton.click();
      await expect(devicesPage.formError).toContainText('required');
    });
  }

  test('should create a new device', async () => {
    await devicesPage.createDevice({ name: 'PW Created Device', uniqueId: `pw-create-${Date.now()}` });
  });

  test('should save and display protocol field', async () => {
    const row = await devicesPage.createDevice({ name: 'PW Protocol Device', uniqueId: `pw-proto-${Date.now()}`, protocol: 'h02' });

    // Open edit modal and verify protocol is hydrated correctly
    await row.locator('button:has-text("Edit")').click();
    await expect(devicesPage.modal).toBeVisible();
    await expect(devicesPage.formProtocolInput).toHaveValue('h02');
    // Identifier stays editable so a defective tracker can be swapped without recreating the device.
    await expect(devicesPage.formUniqueIdInput).toBeEditable();

    // Clear protocol by selecting blank option and save
    await devicesPage.formProtocolInput.selectOption('');
    await devicesPage.saveChangesButton.click();
    await expect(devicesPage.modal).toHaveCount(0, { timeout: 10000 });

    // Reopen edit and confirm protocol is now empty
    await row.locator('button:has-text("Edit")').click();
    await expect(devicesPage.formProtocolInput).toHaveValue('');
    await devicesPage.cancelButton.click();
  });

  test('should offer the OsmAnd protocol for new and existing devices', async () => {
    // New device: OsmAnd (Traccar Client) can be selected on create
    await devicesPage.openCreateModal();
    await expect(devicesPage.formProtocolInput.locator('option[value="osmand"]')).toHaveText('OsmAnd (Traccar Client)');
    await devicesPage.cancelButton.click();
    const row = await devicesPage.createDevice({ name: 'PW OsmAnd Device', uniqueId: `pw-osmand-${Date.now()}`, protocol: 'osmand' });
    await row.locator('button:has-text("Edit")').click();
    await expect(devicesPage.formProtocolInput).toHaveValue('osmand');

    // Existing device: switch to H02 and back to OsmAnd
    await devicesPage.formProtocolInput.selectOption('h02');
    await devicesPage.saveChangesButton.click();
    await expect(devicesPage.modal).toHaveCount(0, { timeout: 10000 });
    await row.locator('button:has-text("Edit")').click();
    await expect(devicesPage.formProtocolInput).toHaveValue('h02');
    await devicesPage.formProtocolInput.selectOption('osmand');
    await devicesPage.saveChangesButton.click();
    await expect(devicesPage.modal).toHaveCount(0, { timeout: 10000 });

    await row.locator('button:has-text("Edit")').click();
    await expect(devicesPage.formProtocolInput).toHaveValue('osmand');
    await devicesPage.cancelButton.click();
  });

  test('should send commands with and without parameters', async () => {
    const row = await devicesPage.createDevice({ name: 'PW Command Device', uniqueId: `pw-cmd-${Date.now()}`, protocol: 'h02' });
    await row.locator('button:has-text("Commands")').click();
    await expect(devicesPage.modal).toBeVisible();

    const history = devicesPage.modal.locator('.cmd-history-item');
    const sendButton = devicesPage.modal.locator('button:has-text("Send")');

    // Command with parameters: "Set Reporting Interval" (positionPeriodic).
    // The device is offline, so the command is queued as pending.
    await devicesPage.modal.locator('#cmd-type').selectOption('positionPeriodic');
    // Quick-select presets plus a custom seconds input; 1 min is the default.
    const presets = devicesPage.modal.locator('button.interval-preset');
    await expect(presets).toHaveText(['5 sec', '20 sec', '1 min', '5 min', '10 min', 'Custom']);
    await expect(presets.filter({ hasText: /^1 min$/ })).toHaveAttribute('aria-pressed', 'true');
    await expect(devicesPage.modal.locator('input[name="frequency"]')).toHaveCount(0);

    // Preset: 10 min. The confirmation only says the interval was requested
    // and queued; the history row shows the requested interval.
    await presets.filter({ hasText: /^10 min$/ }).click();
    await sendButton.click();
    const sentInfo = devicesPage.modal.locator('.cmd-sent-info');
    await expect(sentInfo).toContainText('Reporting interval of 600 s (10 min) requested', { timeout: 15000 });
    await expect(sentInfo).toContainText('queued');
    const intervalRows = history.filter({ hasText: 'Set Reporting Interval' });
    await expect(intervalRows).toHaveCount(1, { timeout: 15000 });
    await expect(intervalRows.first().locator('.cmd-history-interval')).toHaveText('Interval: 600 s (10 min)');
    await expect(devicesPage.modal.locator('.form-error')).toHaveCount(0);

    // Custom interval.
    await presets.filter({ hasText: 'Custom' }).click();
    await devicesPage.modal.locator('input[name="frequency"]').fill('45');
    await sendButton.click();
    await expect(sentInfo).toContainText('Reporting interval of 45 s requested', { timeout: 15000 });
    await expect(intervalRows).toHaveCount(2, { timeout: 15000 });
    await expect(history.locator('.cmd-history-interval', { hasText: 'Interval: 45 s' })).toHaveCount(1);
    await expect(devicesPage.modal.locator('.form-error')).toHaveCount(0);

    // Command without parameters: "Reboot Device". Changing the command type
    // clears the previous confirmation.
    await devicesPage.modal.locator('#cmd-type').selectOption('rebootDevice');
    await expect(sentInfo).toHaveCount(0);
    await sendButton.click();
    await expect(history.filter({ hasText: 'Reboot Device' })).toHaveCount(1, { timeout: 15000 });
    await expect(devicesPage.modal.locator('.form-error')).toHaveCount(0);
  });

  test('should only offer commands the device protocol supports', async () => {
    const openCommands = async (protocol: string) => {
      const row = await devicesPage.createDevice({ name: `PW ${protocol} Commands`, uniqueId: `pw-cmdlist-${protocol}-${Date.now()}`, protocol });
      await row.locator('button:has-text("Commands")').click();
      await expect(devicesPage.modal).toBeVisible();
    };
    const options = () => devicesPage.modal.locator('#cmd-type option');
    const sendButton = () => devicesPage.modal.locator('button:has-text("Send")');

    // H02 supports every command type.
    await openCommands('h02');
    await expect(options()).toHaveCount(7);
    await expect(devicesPage.modal.locator('#cmd-type option[value="setSpeedAlarm"]')).toHaveCount(1);
    await devicesPage.modal.locator('button:has-text("Cancel")').click();

    // Watch: no speed alarm or factory reset.
    await openCommands('watch');
    await expect(devicesPage.modal.locator('#cmd-type option[value="positionPeriodic"]')).toHaveCount(1);
    await expect(devicesPage.modal.locator('#cmd-type option[value="setSpeedAlarm"]')).toHaveCount(0);
    await expect(devicesPage.modal.locator('#cmd-type option[value="factoryReset"]')).toHaveCount(0);
    await expect(options()).toHaveCount(5);
    await devicesPage.modal.locator('button:has-text("Cancel")').click();

    // OsmAnd (Traccar Client) takes no commands.
    await openCommands('osmand');
    await expect(devicesPage.modal.locator('.cmd-unsupported')).toContainText('not supported');
    await expect(devicesPage.modal.locator('#cmd-type')).toHaveCount(0);
    await expect(sendButton()).toBeDisabled();
    await devicesPage.modal.locator('button:has-text("Cancel")').click();
  });

  test('should search devices by name, identifier and case-insensitively', async () => {
    const name = `PW Search ${Date.now()}`;
    const uniqueId = `pw-search-${Date.now()}`;
    await devicesPage.createDevice({ name, uniqueId });

    for (const query of [name, uniqueId, name.toUpperCase()]) {
      await devicesPage.search(query);
      await expect(devicesPage.tableRows).toHaveCount(1);
      await expect(devicesPage.tableRows.first()).toContainText(uniqueId);
    }
  });

  test('should show empty state for no search results', async () => {
    await devicesPage.search('nonexistent-device-xyz-12345');
    await expect(devicesPage.emptyState).toBeVisible();
    await expect(devicesPage.emptyState).toContainText('No devices match');
  });
});

/** Creates a device via the API, runs `fn` on its desktop-table row, then deletes the device. */
async function withDeviceRow(page: Page, name: string, fn: (row: Locator) => Promise<void>) {
  const csrf = (await page.request.get('/api/session')).headers()['x-csrf-token'] ?? '';
  const res = await page.request.post('/api/devices', {
    headers: { 'X-CSRF-Token': csrf },
    data: { name, uniqueId: String(Date.now()) },
  });
  expect(res.status()).toBe(201);
  const id = (await res.json()).id;

  try {
    const devicesPage = new DevicesPage(page);
    await devicesPage.goto();
    await devicesPage.search(name);
    const row = devicesPage.tableRows.filter({ hasText: name });
    await expect(row).toHaveCount(1);
    await fn(row);
  } finally {
    await page.request.delete(`/api/devices/${id}`, { headers: { 'X-CSRF-Token': csrf } });
  }
}

test.describe('Device sharing', () => {
  test('share link expiry preset is sent as expiresAt and shown', async ({ authedPage }) => {
    await withDeviceRow(authedPage, `PW Share ${Date.now()}`, async (row) => {
      await row.locator('button:has-text("Share")').click();
      const modal = new DevicesPage(authedPage).modal;
      await modal.locator('#share-expiry').selectOption('1h');

      const before = Date.now();
      const [request] = await Promise.all([
        authedPage.waitForRequest((r) => r.method() === 'POST' && /\/api\/devices\/\d+\/share$/.test(r.url())),
        modal.locator('button:has-text("Generate Link")').click(),
      ]);
      const expiresAt = Date.parse(request.postDataJSON().expiresAt);
      expect(expiresAt).toBeGreaterThanOrEqual(before + 3_600_000 - 60_000);
      expect(expiresAt).toBeLessThanOrEqual(Date.now() + 3_600_000 + 60_000);

      await expect(modal.locator('.share-link-input')).toHaveValue(/\/share\/[^/]+$/);
      const expiry = modal.locator('.share-item .share-expiry');
      await expect(expiry).toHaveCount(1);
      await expect(expiry).not.toHaveText('Never');
    });
  });
});

test.describe('Device deletion', () => {
  test('asks for confirmation before deleting a device', async ({ authedPage }) => {
    const name = `PW Delete ${Date.now()}`;
    await withDeviceRow(authedPage, name, async (row) => {
      let message = '';
      authedPage.once('dialog', (d) => {
        message = d.message();
        void d.dismiss();
      });
      await row.locator('button:has-text("Delete")').click();
      await expect.poll(() => message).toContain(name);
      await expect(row).toHaveCount(1);

      authedPage.once('dialog', (d) => void d.accept());
      await row.locator('button:has-text("Delete")').click();
      await expect(row).toHaveCount(0);
    });
  });
});
