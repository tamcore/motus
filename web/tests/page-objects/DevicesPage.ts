import { type Page, expect } from '@playwright/test';

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

  get formPhoneInput() {
    return this.page.locator('[role="dialog"] input[name="phone"]');
  }

  get formModelInput() {
    return this.page.locator('[role="dialog"] input[name="model"]');
  }

  get formCategoryInput() {
    return this.page.locator('[role="dialog"] input[name="category"]');
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

  get deleteConfirmButton() {
    return this.page.locator('[role="dialog"] button:has-text("Delete")');
  }

  async search(query: string) {
    await this.searchInput.fill(query);
  }

  async openCreateModal() {
    await this.addDeviceButton.click();
    await expect(this.modal).toBeVisible();
  }

  async fillDeviceForm(data: {
    name?: string;
    uniqueId?: string;
    phone?: string;
    model?: string;
    category?: string;
  }) {
    if (data.name) await this.formNameInput.fill(data.name);
    if (data.uniqueId) await this.formUniqueIdInput.fill(data.uniqueId);
    if (data.phone) await this.formPhoneInput.fill(data.phone);
    if (data.model) await this.formModelInput.fill(data.model);
    if (data.category) await this.formCategoryInput.fill(data.category);
  }

  getEditButton(rowIndex: number) {
    return this.tableRows.nth(rowIndex).locator('button:has-text("Edit")');
  }

  getDeleteButton(rowIndex: number) {
    return this.tableRows.nth(rowIndex).locator('button:has-text("Delete")');
  }

  async expectLoaded() {
    await expect(this.title).toContainText('Devices');
    await expect(this.searchInput).toBeVisible();
  }
}
