import { type Page, expect } from '@playwright/test';

export class GeofencesPage {
  constructor(private page: Page) {}

  async goto() {
    await this.page.goto('/geofences');
    await this.page.waitForSelector('.sidebar-title:has-text("Geofences")');
    await this.page.waitForSelector('.leaflet-container', {
      state: 'visible',
      timeout: 15000,
    });
    await this.page.waitForTimeout(500);
  }

  get sidebarTitle() {
    return this.page.locator('.sidebar-title');
  }

  get fenceCount() {
    return this.page.locator('.fence-count');
  }

  get fenceItems() {
    return this.page.locator('.fence-item');
  }

  get noFencesMessage() {
    return this.page.locator('.no-fences');
  }

  get helpText() {
    return this.page.locator('.sidebar-help');
  }

  get mapContainer() {
    return this.page.locator('.leaflet-container').first();
  }

  get drawToolbar() {
    return this.page.locator('.leaflet-draw-toolbar');
  }

  get nameModal() {
    return this.page.locator('[role="dialog"]');
  }

  get nameInput() {
    return this.page.locator('[role="dialog"] input[name="geofence-name"]');
  }

  get saveButton() {
    return this.page.locator('[role="dialog"] button:has-text("Save Geofence")');
  }

  get cancelButton() {
    return this.page.locator('[role="dialog"] button:has-text("Cancel")');
  }

  getFenceDeleteButton(index: number) {
    return this.fenceItems.nth(index).locator('.fence-delete');
  }

  getFenceInfo(index: number) {
    return this.fenceItems.nth(index).locator('.fence-info');
  }

  async expectLoaded() {
    await expect(this.sidebarTitle).toContainText('Geofences');
    await expect(this.mapContainer).toBeVisible();
  }
}
