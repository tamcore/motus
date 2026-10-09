import { test, expect } from '../fixtures/auth-fixture';
import { csrfToken } from '../helpers/csrf';
import { DevicesPage } from '../page-objects/DevicesPage';

// Attaching geofences to devices from the device edit form. A device with
// attached geofences only gets enter/exit events for those geofences.
test.describe('Device geofence attachments', () => {
  const suffix = Date.now();
  const homeName = `PW Home ${suffix}`;
  const parkName = `PW Park ${suffix}`;
  const uniqueId = `pwgeo${suffix}`;
  const geofenceIds: number[] = [];

  test.beforeAll(async ({ browser }) => {
    const ctx = await browser.newContext({ storageState: '.auth/user.json' });
    const page = await ctx.newPage();
    const token = await csrfToken(page);
    for (const name of [homeName, parkName]) {
      const res = await page.request.post('/api/geofences', {
        headers: { 'X-CSRF-Token': token },
        data: {
          name,
          area: 'POLYGON((11.57 48.12,11.6 48.12,11.6 48.15,11.57 48.15,11.57 48.12))',
        },
      });
      expect(res.status()).toBe(201);
      geofenceIds.push((await res.json()).id);
    }
    await ctx.close();
  });

  test.afterAll(async ({ browser }) => {
    const ctx = await browser.newContext({ storageState: '.auth/user.json' });
    const page = await ctx.newPage();
    const token = await csrfToken(page);
    const devices = await (await page.request.get('/api/devices')).json();
    for (const d of devices.filter((d: { uniqueId: string }) => d.uniqueId === uniqueId)) {
      await page.request.delete(`/api/devices/${d.id}`, { headers: { 'X-CSRF-Token': token } });
    }
    for (const id of geofenceIds) {
      await page.request.delete(`/api/geofences/${id}`, { headers: { 'X-CSRF-Token': token } });
    }
    await ctx.close();
  });

  test('attaches and detaches geofences from the device edit form', async ({ authedPage }) => {
    const devicesPage = new DevicesPage(authedPage);
    await devicesPage.goto();
    await devicesPage.createDevice({ name: `PW Geo Dog ${suffix}`, uniqueId });
    await expect(devicesPage.geofenceNames(uniqueId)).toHaveCount(0);

    // Attach Home only.
    await devicesPage.openEditModal(uniqueId);
    await expect(devicesPage.geofenceCheckbox(homeName)).not.toBeChecked();
    await devicesPage.geofenceCheckbox(homeName).check();
    await devicesPage.saveChangesButton.click();
    await expect(devicesPage.modal).toHaveCount(0, { timeout: 10000 });
    await expect(devicesPage.geofenceNames(uniqueId)).toHaveText(homeName);

    // The selection is restored when editing again; attach Park as well.
    await devicesPage.openEditModal(uniqueId);
    await expect(devicesPage.geofenceCheckbox(homeName)).toBeChecked();
    await expect(devicesPage.geofenceCheckbox(parkName)).not.toBeChecked();
    await devicesPage.geofenceCheckbox(parkName).check();
    await devicesPage.saveChangesButton.click();
    await expect(devicesPage.modal).toHaveCount(0, { timeout: 10000 });
    await expect(devicesPage.geofenceNames(uniqueId)).toContainText(homeName);
    await expect(devicesPage.geofenceNames(uniqueId)).toContainText(parkName);

    // Untick both: the device falls back to all geofences.
    await devicesPage.openEditModal(uniqueId);
    await devicesPage.geofenceCheckbox(homeName).uncheck();
    await devicesPage.geofenceCheckbox(parkName).uncheck();
    await devicesPage.saveChangesButton.click();
    await expect(devicesPage.modal).toHaveCount(0, { timeout: 10000 });
    await expect(devicesPage.geofenceNames(uniqueId)).toHaveCount(0);
  });
});
