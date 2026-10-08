import { type Locator, type Page, expect } from '@playwright/test';

export class DevicesPage {
  constructor(private page: Page) {}

  async goto() {
    await this.page.goto('/devices');
    await this.page.waitForSelector('h1:has-text("Devices")');
  }

  get title() {
    return this.page.locator('h1.page-title');
  }

  get addDeviceButton() {
    return this.page.locator('button:has-text("Add Device")');
  }

  get searchInput() {
    return this.page.locator('input[placeholder="Search devices..."]');
  }

  get resultCount() {
    return this.page.locator('.result-count');
  }

  get tableRows() {
    return this.page.locator('.device-table tbody tr.table-row');
  }

  get table() {
    return this.page.locator('.device-table');
  }

  get emptyState() {
    return this.page.locator('.empty-state');
  }

  get modal() {
    return this.page.locator('[role="dialog"]');
  }

  get modalTitle() {
    return this.page.locator('#modal-title');
  }

  get formNameInput() {
    return this.page.locator('[role="dialog"] input[name="name"]');
  }

  get formUniqueIdInput() {
    return this.page.locator('[role="dialog"] input[name="uniqueId"]');
  }

  get formProtocolInput() {
    return this.page.locator('[role="dialog"] select[name="protocol"]');
  }

  get formError() {
    return this.page.locator('.form-error');
  }

  get cancelButton() {
    return this.page.locator('[role="dialog"] button:has-text("Cancel")');
  }

  get saveButton() {
    return this.page.locator('[role="dialog"] button:has-text("Create Device")');
  }

  get saveChangesButton() {
    return this.page.locator('[role="dialog"] button:has-text("Save Changes")');
  }

  async search(query: string) {
    await this.searchInput.fill(query);
  }

  async openCreateModal() {
    await this.addDeviceButton.click();
    await expect(this.modal).toBeVisible();
  }

  async fillDeviceForm(data: { name?: string; uniqueId?: string; protocol?: string }) {
    if (data.name) await this.formNameInput.fill(data.name);
    if (data.uniqueId) await this.formUniqueIdInput.fill(data.uniqueId);
    if (data.protocol !== undefined) await this.formProtocolInput.selectOption(data.protocol);
  }

  /** Creates a device through the modal and returns its desktop-table row. */
  async createDevice(data: { name: string; uniqueId: string; protocol?: string }): Promise<Locator> {
    await this.openCreateModal();
    await this.fillDeviceForm(data);
    await this.saveButton.click();
    await expect(this.modal).toHaveCount(0, { timeout: 10000 });
    const row = this.tableRows.filter({ hasText: data.uniqueId });
    await expect(row).toBeVisible({ timeout: 5000 });
    return row;
  }

  /** Opens the edit modal from the desktop-table row of the device. */
  async openEditModal(uniqueId: string) {
    await this.tableRows.filter({ hasText: uniqueId }).locator('button:has-text("Edit")').click();
    await expect(this.modal).toBeVisible();
  }

  /** Geofence checkbox in the device form. */
  geofenceCheckbox(geofenceName: string) {
    return this.page
      .locator('[role="dialog"] [data-testid="device-geofences"] label')
      .filter({ hasText: geofenceName })
      .locator('input[type="checkbox"]');
  }

  /** Attached geofence names in the desktop-table row of the device. */
  geofenceNames(uniqueId: string) {
    return this.tableRows.filter({ hasText: uniqueId }).locator('.device-geofence-names');
  }

  /** Battery cell of the desktop-table row for the named device. */
  batteryCell(deviceName: string) {
    return this.tableRows.filter({ hasText: deviceName }).locator('td.td-battery');
  }

  async expectLoaded() {
    await expect(this.title).toContainText('Devices');
    await expect(this.searchInput).toBeVisible();
  }
}
