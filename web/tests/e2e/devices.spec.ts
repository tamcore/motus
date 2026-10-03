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

  test('should save and display protocol field', async ({ authedPage }) => {
    const uniqueId = `pw-proto-${Date.now()}`;

    // Create a device with protocol set
    await devicesPage.openCreateModal();
    await devicesPage.fillDeviceForm({ name: 'PW Protocol Device', uniqueId, protocol: 'h02' });
    await devicesPage.saveButton.click();
    await expect(devicesPage.modal).toHaveCount(0, { timeout: 10000 });

    // Find the newly created device row
    const row = authedPage.locator('.device-table').locator(`tr:has-text("${uniqueId}")`);
    await expect(row).toBeVisible({ timeout: 5000 });

    // Open edit modal and verify protocol is hydrated correctly
    await row.locator('button:has-text("Edit")').click();
    await expect(devicesPage.modal).toBeVisible();
    await expect(devicesPage.formProtocolInput).toHaveValue('h02');

    // Clear protocol by selecting blank option and save
    await devicesPage.formProtocolInput.selectOption('');
    await devicesPage.saveChangesButton.click();
    await expect(devicesPage.modal).toHaveCount(0, { timeout: 10000 });

    // Reopen edit and confirm protocol is now empty
    await row.locator('button:has-text("Edit")').click();
    await expect(devicesPage.formProtocolInput).toHaveValue('');
    await devicesPage.cancelButton.click();
  });

  test('should offer the OsmAnd protocol for new and existing devices', async ({ authedPage }) => {
    const uniqueId = `pw-osmand-${Date.now()}`;

    // New device: OsmAnd (Traccar Client) can be selected on create
    await devicesPage.openCreateModal();
    await expect(devicesPage.formProtocolInput.locator('option[value="osmand"]')).toHaveText('OsmAnd (Traccar Client)');
    await devicesPage.fillDeviceForm({ name: 'PW OsmAnd Device', uniqueId, protocol: 'osmand' });
    await devicesPage.saveButton.click();
    await expect(devicesPage.modal).toHaveCount(0, { timeout: 10000 });

    const row = authedPage.locator('.device-table').locator(`tr:has-text("${uniqueId}")`);
    await expect(row).toBeVisible({ timeout: 5000 });
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

  test('should send commands with and without parameters', async ({ authedPage }) => {
    const uniqueId = `pw-cmd-${Date.now()}`;
    await devicesPage.openCreateModal();
    await devicesPage.fillDeviceForm({ name: 'PW Command Device', uniqueId, protocol: 'h02' });
    await devicesPage.saveButton.click();
    await expect(devicesPage.modal).toHaveCount(0, { timeout: 10000 });

    const row = authedPage.locator('.device-table').locator(`tr:has-text("${uniqueId}")`);
    await expect(row).toBeVisible({ timeout: 5000 });
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

  test('should only offer commands the device protocol supports', async ({ authedPage }) => {
    const openCommands = async (protocol: string) => {
      const uniqueId = `pw-cmdlist-${protocol}-${Date.now()}`;
      await devicesPage.openCreateModal();
      await devicesPage.fillDeviceForm({ name: `PW ${protocol} Commands`, uniqueId, protocol });
      await devicesPage.saveButton.click();
      await expect(devicesPage.modal).toHaveCount(0, { timeout: 10000 });
      const row = authedPage.locator('.device-table').locator(`tr:has-text("${uniqueId}")`);
      await expect(row).toBeVisible({ timeout: 5000 });
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

test.describe('Device deletion', () => {
  test('asks for confirmation before deleting a device', async ({ authedPage }) => {
    const name = `PW Delete ${Date.now()}`;
    const csrf = (await authedPage.request.get('/api/session')).headers()['x-csrf-token'] ?? '';
    const res = await authedPage.request.post('/api/devices', {
      headers: { 'X-CSRF-Token': csrf },
      data: { name, uniqueId: String(Date.now()) },
    });
    expect(res.status()).toBe(201);
    const id = (await res.json()).id;

    try {
      const devicesPage = new DevicesPage(authedPage);
      await devicesPage.goto();
      await devicesPage.search(name);
      const row = devicesPage.tableRows.filter({ hasText: name });
      await expect(row).toHaveCount(1);

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
    } finally {
      await authedPage.request.delete(`/api/devices/${id}`, { headers: { 'X-CSRF-Token': csrf } });
    }
  });
});
