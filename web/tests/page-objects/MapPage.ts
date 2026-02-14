import { type Page, expect } from '@playwright/test';

export class MapPage {
  constructor(private page: Page) {}

  async goto() {
    await this.page.goto('/map');
    await this.waitForMapLoad();
  }

  async waitForMapLoad() {
    await this.page.waitForSelector('.leaflet-container', {
      state: 'visible',
      timeout: 15000,
    });
    // Allow map tiles and markers to initialize
    await this.page.waitForTimeout(1000);
  }

  get sidebar() {
    return this.page.locator('.sidebar').first();
  }

  get sidebarTitle() {
    return this.page.locator('.sidebar-title');
  }

  get searchInput() {
    return this.page.locator('.search-input');
  }

  get deviceItems() {
    return this.page.locator('.device-item');
  }

  get selectedDevice() {
    return this.page.locator('.device-item.selected');
  }

  get detailPanel() {
    return this.page.locator('.detail-panel');
  }

  get mapContainer() {
    return this.page.locator('.leaflet-container').first();
  }

  get markers() {
    return this.page.locator('.custom-marker');
  }

  get zoomIn() {
    return this.page.locator('.leaflet-control-zoom-in');
  }

  get zoomOut() {
    return this.page.locator('.leaflet-control-zoom-out');
  }

  get sidebarToggle() {
    return this.page.locator('.sidebar-toggle');
  }

  get popup() {
    return this.page.locator('.leaflet-popup');
  }

  get popupContent() {
    return this.page.locator('.leaflet-popup-content');
  }

  get tiles() {
    return this.page.locator('.leaflet-tile');
  }

  get trailButton() {
    return this.page.locator('button:has-text("Show Trail")');
  }

  async searchDevices(query: string) {
    await this.searchInput.fill(query);
  }

  async clickDevice(index: number) {
    await this.deviceItems.nth(index).click();
  }

  async toggleSidebar() {
    await this.sidebarToggle.click();
  }

  async expectLoaded() {
    await expect(this.mapContainer).toBeVisible();
    await expect(this.sidebarTitle).toContainText('Devices');
  }
}
