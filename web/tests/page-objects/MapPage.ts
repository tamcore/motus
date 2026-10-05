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

  get popupContent() {
    return this.page.locator('.leaflet-popup-content');
  }

  get tiles() {
    return this.page.locator('.leaflet-tile');
  }

  get wsIndicator() {
    return this.page.locator('.ws-indicator');
  }

  get locateButton() {
    return this.page.locator('.locate-me-btn');
  }

  get layerToggle() {
    return this.page.getByRole('button', { name: 'Map layers' });
  }

  get layerPanel() {
    return this.page.locator('.layer-panel');
  }

  get overlayRadios() {
    return this.layerPanel.getByRole('radio');
  }

  get hideTrailButton() {
    return this.page.locator('button:has-text("Hide Trail")');
  }

  get trailRangeSelect() {
    return this.page.locator('select.trail-range-select');
  }

  get trailFromDate() {
    return this.page.locator('#trail-from-date');
  }

  get trailFromTime() {
    return this.page.locator('#trail-from-time');
  }

  get trailToDate() {
    return this.page.locator('#trail-to-date');
  }

  get trailToTime() {
    return this.page.locator('#trail-to-time');
  }

  get trailApply() {
    return this.page.locator('button.trail-apply');
  }

  get trailStatus() {
    return this.page.locator('.trail-status');
  }

  get saveBookmarkButton() {
    return this.page.locator('.detail-actions button:has-text("Save as bookmark")');
  }

  get bookmarkList() {
    return this.page.locator('.trail-bookmarks');
  }

  get bookmarkItems() {
    return this.page.locator('.trail-bookmark-item');
  }

  bookmarkOpenButton(name: string) {
    return this.page.getByRole('button', { name: `Show bookmark ${name}` });
  }

  bookmarkEditButton(name: string) {
    return this.page.getByRole('button', { name: `Edit bookmark ${name}`, exact: true });
  }

  bookmarkDeleteButton(name: string) {
    return this.page.getByRole('button', { name: `Delete bookmark ${name}`, exact: true });
  }

  get bookmarkDialog() {
    return this.page.locator('.modal[role="dialog"]');
  }

  get bookmarkName() {
    return this.page.locator('#bookmark-name');
  }

  get bookmarkDescription() {
    return this.page.locator('#bookmark-description');
  }

  get bookmarkFromDate() {
    return this.page.locator('#bookmark-from-date');
  }

  get bookmarkFromTime() {
    return this.page.locator('#bookmark-from-time');
  }

  get bookmarkToDate() {
    return this.page.locator('#bookmark-to-date');
  }

  get bookmarkToTime() {
    return this.page.locator('#bookmark-to-time');
  }

  get bookmarkSubmit() {
    return this.bookmarkDialog.locator('button[type="submit"]');
  }

  async searchDevices(query: string) {
    await this.searchInput.fill(query);
  }

  deviceItem(name: string) {
    return this.deviceItems.filter({ hasText: name });
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
