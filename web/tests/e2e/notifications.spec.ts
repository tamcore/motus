import type { Page } from '@playwright/test';
import { test, expect } from '../fixtures/auth-fixture';
import { NotificationsPage } from '../page-objects/NotificationsPage';
import { mockFetch } from '../helpers/mock-fetch';

test.describe('Notifications Page', () => {
  let notifPage: NotificationsPage;

  test.beforeEach(async ({ authedPage }) => {
    notifPage = new NotificationsPage(authedPage);
    await notifPage.goto();
  });

  test('should display notifications page title', async () => {
    await notifPage.expectLoaded();
  });

  test('should have correct page title in tab', async ({ authedPage }) => {
    await expect(authedPage).toHaveTitle('Notifications - Motus');
  });

  test('should show Create Rule button', async () => {
    await expect(notifPage.createButton).toBeVisible();
  });

  test('should show empty state initially', async () => {
    await expect(notifPage.emptyState).toBeVisible();
    await expect(notifPage.emptyState).toContainText('No notification rules yet');
  });

  test('should show Create first rule button in empty state', async ({ authedPage }) => {
    const firstRuleBtn = authedPage.locator('.empty-state button:has-text("Create your first rule")');
    await expect(firstRuleBtn).toBeVisible();
  });

  test('should open create modal when clicking Create Rule', async () => {
    await notifPage.createButton.click();
    await expect(notifPage.modal).toBeVisible();
    await expect(notifPage.modalTitle).toContainText('Create Notification Rule');
  });

  test('should show all form fields in create modal', async () => {
    await notifPage.createButton.click();
    await expect(notifPage.nameInput).toBeVisible();
    await expect(notifPage.eventTypeCheckboxes.first()).toBeVisible();
    await expect(notifPage.webhookUrlInput).toBeVisible();
    await expect(notifPage.templateTextarea).toBeVisible();
  });

  test('should show event type checkboxes', async ({ authedPage }) => {
    await notifPage.createButton.click();
    const count = await notifPage.eventTypeCheckboxes.count();
    expect(count).toBeGreaterThanOrEqual(6);
  });

  test('should show template variables section', async () => {
    await notifPage.createButton.click();
    await expect(notifPage.variableButtons.first()).toBeVisible();
    const count = await notifPage.variableButtons.count();
    expect(count).toBeGreaterThanOrEqual(8); // 8 template variables
  });

  test('should insert variable into template on click', async ({ authedPage }) => {
    await notifPage.createButton.click();
    // Clear template first
    await notifPage.templateTextarea.fill('');
    // Click a variable button
    await notifPage.variableButtons.first().click();
    const value = await notifPage.templateTextarea.inputValue();
    expect(value).toContain('{{');
  });

  test('should show Add Header button', async () => {
    await notifPage.createButton.click();
    await expect(notifPage.addHeaderButton).toBeVisible();
  });

  test('should add header row when clicking Add Header', async ({ authedPage }) => {
    await notifPage.createButton.click();
    await notifPage.addHeaderButton.click();
    const headerRows = authedPage.locator('[role="dialog"] .header-row');
    await expect(headerRows).toHaveCount(1);
  });

  test('should close modal on Cancel', async () => {
    await notifPage.createButton.click();
    await expect(notifPage.modal).toBeVisible();
    await notifPage.cancelButton.click();
    await expect(notifPage.modal).toHaveCount(0);
  });

  test('should close modal on X button', async ({ authedPage }) => {
    await notifPage.createButton.click();
    await expect(notifPage.modal).toBeVisible();
    await authedPage.click('[role="dialog"] .close-button');
    await expect(notifPage.modal).toHaveCount(0);
  });

  test('should center the modal in the viewport', async ({ authedPage }) => {
    await notifPage.createButton.click();
    await expect(notifPage.modal).toBeVisible();
    const box = await notifPage.modal.boundingBox();
    const viewport = authedPage.viewportSize();
    expect(box && viewport).toBeTruthy();
    const centerX = box!.x + box!.width / 2;
    expect(Math.abs(centerX - viewport!.width / 2)).toBeLessThan(2);
  });

  test('should close modal on Escape key and restore focus', async ({ authedPage }) => {
    await notifPage.createButton.click();
    await expect(notifPage.modal).toBeVisible();
    await authedPage.keyboard.press('Escape');
    await expect(notifPage.modal).toHaveCount(0);
    await expect(notifPage.createButton).toBeFocused();
  });

  test('should create notification rule', async ({ authedPage }) => {
    const ruleName = `Test Alert ${Date.now()}`;
    await notifPage.createButton.click();
    await notifPage.nameInput.fill(ruleName);
    await notifPage.selectEventTypes('geofenceEnter');
    await notifPage.webhookUrlInput.fill('https://example.com/hook');
    await notifPage.submitButton.click();

    // Modal should close, rule should appear
    await expect(notifPage.modal).toHaveCount(0, { timeout: 10000 });
    await expect(notifPage.ruleCards.first()).toBeVisible();
    await expect(authedPage.locator(`.rule-name:has-text("${ruleName}")`)).toBeVisible();
  });

  test('should show toggle switch on rule card', async ({ authedPage }) => {
    // Create a rule first
    const ruleName = `Toggle Test ${Date.now()}`;
    await notifPage.createButton.click();
    await notifPage.nameInput.fill(ruleName);
    await notifPage.selectEventTypes('geofenceEnter');
    await notifPage.webhookUrlInput.fill('https://example.com/hook');
    await notifPage.submitButton.click();
    await expect(notifPage.modal).toHaveCount(0, { timeout: 10000 });

    // The checkbox input is hidden (opacity:0) for the custom toggle UI
    const toggleSwitch = notifPage.ruleCards.first().locator('.toggle-switch');
    await expect(toggleSwitch).toBeVisible();
  });

  test('should show Edit and Delete buttons on rule card', async ({ authedPage }) => {
    // Create a rule first
    const ruleName = `Button Test ${Date.now()}`;
    await notifPage.createButton.click();
    await notifPage.nameInput.fill(ruleName);
    await notifPage.selectEventTypes('geofenceEnter');
    await notifPage.webhookUrlInput.fill('https://example.com/hook');
    await notifPage.submitButton.click();
    await expect(notifPage.modal).toHaveCount(0, { timeout: 10000 });

    await expect(notifPage.getRuleEditButton(0)).toBeVisible();
    await expect(notifPage.getRuleDeleteButton(0)).toBeVisible();
    await expect(notifPage.getRuleTestButton(0)).toBeVisible();
  });
});

// Geofence filter + "Device Command" channel (e.g. pet tracking: report
// every 20 s after leaving home, every 300 s once back home).
test.describe('Notification geofence filter and command actions', () => {
  let notifPage: NotificationsPage;
  const geofenceName = `PW Home ${Date.now()}`;
  const deviceName = `PW Pet ${Date.now()}`;
  let deviceId: number;
  let geofenceId: number;

  async function csrfToken(page: Page): Promise<string> {
    const res = await page.request.get('/api/session');
    return res.headers()['x-csrf-token'] ?? '';
  }

  test.beforeAll(async ({ browser }) => {
    const ctx = await browser.newContext({ storageState: '.auth/user.json' });
    const page = await ctx.newPage();
    const res = await page.request.post('/api/geofences', {
      headers: { 'X-CSRF-Token': await csrfToken(page) },
      data: {
        name: geofenceName,
        area: 'POLYGON((11.57 48.12,11.6 48.12,11.6 48.15,11.57 48.15,11.57 48.12))',
      },
    });
    expect(res.status()).toBe(201);
    geofenceId = (await res.json()).id;
    const deviceRes = await page.request.post('/api/devices', {
      headers: { 'X-CSRF-Token': await csrfToken(page) },
      data: { name: deviceName, uniqueId: `pw-pet-${Date.now()}` },
    });
    expect(deviceRes.ok()).toBeTruthy();
    deviceId = (await deviceRes.json()).id;
    await ctx.close();
  });

  test.afterAll(async ({ browser }) => {
    const ctx = await browser.newContext({ storageState: '.auth/user.json' });
    const page = await ctx.newPage();
    await page.request.delete(`/api/devices/${deviceId}`, {
      headers: { 'X-CSRF-Token': await csrfToken(page) },
    });
    await page.request.delete(`/api/geofences/${geofenceId}`, {
      headers: { 'X-CSRF-Token': await csrfToken(page) },
    });
    await ctx.close();
  });

  test.beforeEach(async ({ authedPage }) => {
    notifPage = new NotificationsPage(authedPage);
    await notifPage.goto();
  });

  // Remove the rules these tests create so other notification tests (e.g.
  // the empty state) are unaffected.
  test.afterEach(async ({ authedPage }) => {
    const res = await authedPage.request.get('/api/notifications');
    if (!res.ok()) return;
    const rules: Array<{ id: number; name: string }> = await res.json();
    const csrf = await csrfToken(authedPage);
    for (const rule of rules.filter((r) => r.name.includes(geofenceName) || r.name.startsWith('PW Pet'))) {
      await authedPage.request.delete(`/api/notifications/${rule.id}`, {
        headers: { 'X-CSRF-Token': csrf },
      });
    }
  });

  test('offers Device Command channel with interval presets instead of webhook fields', async () => {
    await notifPage.createButton.click();
    await notifPage.channelSelect.selectOption('command');

    await expect(notifPage.commandTypeSelect).toBeVisible();
    await expect(notifPage.commandTypeSelect).toHaveValue('positionPeriodic');
    // Same ReportingIntervalPicker as the device command dialog; 1 min is the default.
    await expect(notifPage.intervalPresets).toHaveText(['5 sec', '20 sec', '1 min', '5 min', '10 min', 'Custom']);
    await expect(notifPage.intervalPreset('1 min')).toHaveAttribute('aria-pressed', 'true');
    await expect(notifPage.frequencyInput).toHaveCount(0);
    await expect(notifPage.webhookUrlInput).toHaveCount(0);
    await expect(notifPage.templateTextarea).toHaveCount(0);
    await expect(notifPage.commandTypeSelect.locator('option[value="factoryReset"]')).toHaveCount(0);
    await notifPage.cancelButton.click();
  });

  test('shows the geofence filter only for geofence events', async () => {
    await notifPage.createButton.click();
    await expect(notifPage.geofenceFilter).toHaveCount(0);

    await notifPage.selectEventTypes('geofenceExit');
    await expect(notifPage.geofenceFilter).toBeVisible();
    await expect(notifPage.geofenceCheckbox(geofenceName)).toBeVisible();
    await expect(notifPage.geofenceFilter).toContainText('all geofences');
  });

  test('rejects a custom interval above one day', async () => {
    await notifPage.createButton.click();
    await notifPage.nameInput.fill('PW Pet invalid');
    await notifPage.selectEventTypes('geofenceExit');
    await notifPage.channelSelect.selectOption('command');
    await notifPage.intervalPreset('Custom').click();
    await notifPage.frequencyInput.fill('86401');
    await notifPage.submitButton.click();

    await expect(notifPage.formError).toContainText('86400');
    await expect(notifPage.modal).toBeVisible();
  });

  test('blocks reboot commands on device online/offline', async () => {
    await notifPage.createButton.click();
    await notifPage.nameInput.fill('PW Pet reboot loop');
    await notifPage.selectEventTypes('deviceOnline');
    await notifPage.channelSelect.selectOption('command');
    await notifPage.commandTypeSelect.selectOption('rebootDevice');

    await expect(notifPage.commandConflict).toContainText('Device Online');
    await notifPage.submitButton.click();
    await expect(notifPage.formError).toContainText('reconnect');
    await expect(notifPage.modal).toBeVisible();

    // A non-reconnect event is fine.
    await notifPage.commandTypeSelect.selectOption('positionSingle');
    await expect(notifPage.commandConflict).toHaveCount(0);
  });

  test('creates a command rule for a selected geofence', async () => {
    const ruleName = 'PW Pet left home';
    await notifPage.createButton.click();
    await notifPage.nameInput.fill(ruleName);
    await notifPage.selectEventTypes('geofenceExit');
    await notifPage.geofenceCheckbox(geofenceName).check();
    await notifPage.channelSelect.selectOption('command');
    await notifPage.intervalPreset('20 sec').click();
    await notifPage.submitButton.click();

    await expect(notifPage.modal).toHaveCount(0, { timeout: 10000 });
    const card = notifPage.ruleCard(ruleName);
    await expect(card).toBeVisible();
    await expect(card.locator('.rule-destination')).toHaveText('Set Reporting Interval: 20 s');
    await expect(card.locator('.rule-geofences')).toHaveText(geofenceName);
    await expect(card.locator('.channel-command')).toBeVisible();
    // Command rules cannot be tested without sending a real command.
    await expect(card.locator('button:has-text("Test")')).toHaveCount(0);

    // Editing restores the command and geofence selection.
    await card.locator('button:has-text("Edit")').click();
    await expect(notifPage.channelSelect).toHaveValue('command');
    await expect(notifPage.intervalPreset('20 sec')).toHaveAttribute('aria-pressed', 'true');
    await expect(notifPage.geofenceCheckbox(geofenceName)).toBeChecked();
    await notifPage.cancelButton.click();
  });

  test('keeps a deleted geofence visible in the rule until it is removed explicitly', async ({ authedPage }) => {
    const ruleName = 'PW Pet back home';
    const tmpName = `PW Tmp ${Date.now()}`;
    const csrf = await csrfToken(authedPage);
    const gfRes = await authedPage.request.post('/api/geofences', {
      headers: { 'X-CSRF-Token': csrf },
      data: {
        name: tmpName,
        area: 'POLYGON((11.5 48.1,11.52 48.1,11.52 48.12,11.5 48.12,11.5 48.1))',
      },
    });
    expect(gfRes.status()).toBe(201);
    const tmpId: number = (await gfRes.json()).id;
    const ruleRes = await authedPage.request.post('/api/notifications', {
      headers: { 'X-CSRF-Token': csrf },
      data: {
        name: ruleName,
        eventTypes: ['geofenceEnter'],
        channel: 'command',
        geofenceIds: [tmpId],
        config: {
          channel: 'command',
          commandType: 'positionPeriodic',
          attributes: { type: 'positionPeriodic', frequency: 300 },
        },
        enabled: true,
      },
    });
    expect(ruleRes.status()).toBe(201);
    await authedPage.request.delete(`/api/geofences/${tmpId}`, { headers: { 'X-CSRF-Token': csrf } });

    await notifPage.goto();
    const card = notifPage.ruleCard(ruleName);
    await expect(card.locator('.rule-destination')).toHaveText('Set Reporting Interval: 300 s (5 min)');
    await expect(card.locator('.rule-geofences')).toHaveText(`Geofence #${tmpId} (unavailable)`);

    // The enabled toggle resends the stored (deleted) geofence ID and must work.
    await card.locator('.toggle-switch').click();
    await expect(card).toHaveClass(/disabled/);
    await expect(authedPage.locator('.error-banner')).toHaveCount(0);

    // The editor shows the unavailable geofence selected instead of silently
    // dropping it (an empty filter would mean all geofences).
    await card.locator('button:has-text("Edit")').click();
    await expect(notifPage.unavailableGeofences).toHaveCount(1);
    const unavailable = notifPage.unavailableGeofences.locator('input');
    await expect(unavailable).toBeChecked();
    await expect(notifPage.unavailableGeofenceHint).toBeVisible();
    // Unticking keeps the row (unchecked) so it can be ticked again.
    await unavailable.uncheck();
    await expect(notifPage.unavailableGeofences).toHaveCount(1);
    await expect(unavailable).not.toBeChecked();
    await expect(notifPage.unavailableGeofenceHint).toHaveCount(0);
    await unavailable.check();
    await expect(notifPage.unavailableGeofenceHint).toBeVisible();
    await unavailable.uncheck();
    await notifPage.geofenceCheckbox(geofenceName).check();
    await notifPage.updateButton.click();

    await expect(notifPage.modal).toHaveCount(0, { timeout: 10000 });
    await expect(card.locator('.rule-geofences')).toHaveText(geofenceName);
  });

  test('interval automation creates exit and enter rules for the geofence and device', async () => {
    await notifPage.automationButton.click();
    await expect(notifPage.modal).toBeVisible();
    await notifPage.automationGeofenceSelect.selectOption({ label: geofenceName });
    await notifPage.automationDeviceCheckbox(deviceName).check();
    // Same ReportingIntervalPicker as the rule form, preset to 20 s / 5 min.
    await expect(notifPage.automationPreset('away', '20 sec')).toHaveAttribute('aria-pressed', 'true');
    await expect(notifPage.automationPreset('home', '5 min')).toHaveAttribute('aria-pressed', 'true');
    await notifPage.createRulesButton.click();

    await expect(notifPage.modal).toHaveCount(0, { timeout: 10000 });
    const exitRule = notifPage.ruleCard(`Left ${geofenceName}: report every 20 s`);
    const enterRule = notifPage.ruleCard(`Entered ${geofenceName}: report every 300 s (5 min)`);
    await expect(exitRule.locator('.rule-destination')).toHaveText('Set Reporting Interval: 20 s');
    await expect(enterRule.locator('.rule-destination')).toHaveText('Set Reporting Interval: 300 s (5 min)');
    for (const rule of [exitRule, enterRule]) {
      await expect(rule.locator('.rule-geofences')).toHaveText(geofenceName);
      await expect(rule.locator('.rule-devices')).toHaveText(deviceName);
    }
  });

  test('interval automation requires a device', async () => {
    await notifPage.automationButton.click();
    await notifPage.automationGeofenceSelect.selectOption({ label: geofenceName });
    // Other devices may exist; make sure none is selected.
    const boxes = notifPage.modal.locator('.automation-device-checkbox input[type="checkbox"]');
    for (let i = 0; i < (await boxes.count()); i++) await boxes.nth(i).uncheck();
    await notifPage.createRulesButton.click();
    await expect(notifPage.formError).toContainText('Select at least one device');
    await expect(notifPage.modal).toBeVisible();
  });

  test('interval automation rejects an interval above one day', async () => {
    await notifPage.automationButton.click();
    await notifPage.automationGeofenceSelect.selectOption({ label: geofenceName });
    await notifPage.automationDeviceCheckbox(deviceName).check();
    await notifPage.automationPreset('home', 'Custom').click();
    await notifPage.homeIntervalInput.fill('86401');
    await notifPage.createRulesButton.click();
    await expect(notifPage.formError).toContainText('1 day');
    await expect(notifPage.modal).toBeVisible();
  });
});

// Device filter: a rule can be limited to selected devices (any event type).
test.describe('Notification device filter', () => {
  const deviceName = `PW Filter Device ${Date.now()}`;
  const ruleName = `PW Device Rule ${Date.now()}`;
  let deviceId: number;

  async function csrfToken(page: Page): Promise<string> {
    const res = await page.request.get('/api/session');
    return res.headers()['x-csrf-token'] ?? '';
  }

  test.beforeAll(async ({ browser }) => {
    const ctx = await browser.newContext({ storageState: '.auth/user.json' });
    const page = await ctx.newPage();
    const res = await page.request.post('/api/devices', {
      headers: { 'X-CSRF-Token': await csrfToken(page) },
      data: { name: deviceName, uniqueId: `pw-filter-${Date.now()}` },
    });
    expect(res.ok()).toBeTruthy();
    deviceId = (await res.json()).id;
    await ctx.close();
  });

  test.afterAll(async ({ browser }) => {
    const ctx = await browser.newContext({ storageState: '.auth/user.json' });
    const page = await ctx.newPage();
    const csrf = await csrfToken(page);
    const rules: Array<{ id: number; name: string }> = await (await page.request.get('/api/notifications')).json();
    for (const rule of rules.filter((r) => r.name === ruleName)) {
      await page.request.delete(`/api/notifications/${rule.id}`, { headers: { 'X-CSRF-Token': csrf } });
    }
    await page.request.delete(`/api/devices/${deviceId}`, { headers: { 'X-CSRF-Token': csrf } });
    await ctx.close();
  });

  test('limits a rule to the selected device', async ({ authedPage }) => {
    const notifPage = new NotificationsPage(authedPage);
    await notifPage.goto();
    await notifPage.createButton.click();
    await notifPage.nameInput.fill(ruleName);
    await notifPage.selectEventTypes('deviceOnline');
    await notifPage.webhookUrlInput.fill('https://example.com/hook');
    await notifPage.deviceCheckbox(deviceName).check();
    await notifPage.submitButton.click();
    await expect(notifPage.modal).toHaveCount(0, { timeout: 10000 });

    const card = notifPage.ruleCard(ruleName);
    await expect(card.locator('.rule-devices')).toHaveText(deviceName);

    await card.locator('button:has-text("Edit")').click();
    await expect(notifPage.deviceCheckbox(deviceName)).toBeChecked();
    await notifPage.cancelButton.click();
  });
});

test.describe('Delivery logs', () => {
  const rule = (id: number, name: string) => ({
    id, userId: 1, name, eventTypes: ['deviceOnline'], channel: 'webhook',
    config: { channel: 'webhook', webhookUrl: 'https://example.com/hook' },
    template: '', enabled: true, geofenceIds: [],
    createdAt: '2026-01-01T00:00:00Z', updatedAt: '2026-01-01T00:00:00Z',
  });
  const log = (id: number, ruleId: number) => ({
    id, ruleId, status: 'sent', responseCode: 200, createdAt: `2026-01-0${id}T10:00:00Z`,
  });

  test('rule Logs link opens the history filtered to that rule', async ({ authedPage }) => {
    const rules = [rule(1, 'PW Rule One'), rule(2, 'PW Rule Two')];
    await mockFetch(authedPage, [
      { path: '/api/notifications', body: rules },
      { path: '/api/admin/notifications', body: rules },
      { path: '/api/notifications/1/logs', body: [log(1, 1)] },
      { path: '/api/notifications/2/logs', body: [log(2, 2), log(3, 2)] },
    ]);

    await authedPage.goto('/notifications');
    const card = authedPage.locator('.rule-card', { hasText: 'PW Rule Two' });
    await card.locator('a:has-text("Logs")').click();

    await expect(authedPage).toHaveURL(/\/notifications\/history\?rule=2/);
    await expect(authedPage.locator('#rule-filter')).toHaveValue('2');
    const rows = authedPage.locator('table.history-table tbody tr');
    await expect(rows).toHaveCount(2);
    await expect(rows.first()).toContainText('PW Rule Two');

    await authedPage.selectOption('#rule-filter', 'all');
    await expect(rows).toHaveCount(3);
  });
});
