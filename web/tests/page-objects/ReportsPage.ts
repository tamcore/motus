import { type Page, expect } from '@playwright/test';

export class ReportsPage {
  constructor(private page: Page) {}

  async goto() {
    await this.page.goto('/reports');
    await this.page.waitForSelector('h1:has-text("Reports")');
    // Wait for devices to load (skeleton is replaced by the select)
    await this.page.waitForSelector('#device-select', { timeout: 15000 });
  }

  get title() {
    return this.page.locator('h1.page-title');
  }

  get deviceSelect() {
    return this.page.locator('#device-select');
  }

  get applyButton() {
    return this.page.locator('button:has-text("Apply")');
  }

  get exportCSVButton() {
    return this.page.locator('button:has-text("Export CSV")');
  }

  get tripsTab() {
    return this.page.locator('button[role="tab"]:has-text("Trips")');
  }

  get stopsTab() {
    return this.page.locator('button[role="tab"]:has-text("Stops")');
  }

  get summaryTab() {
    return this.page.locator('button[role="tab"]:has-text("Summary")');
  }

  get tripsTable() {
    return this.page.locator('.trips-table');
  }

  get tripsTableRows() {
    return this.page.locator('.trips-table tbody tr');
  }

  get chartContainer() {
    return this.page.locator('.chart-container');
  }

  get statsGrid() {
    return this.page.locator('.stats-grid');
  }

  get emptyState() {
    return this.page.locator('.empty-state');
  }

  get presetButtons() {
    return this.page.locator('.preset-btn');
  }

  get customFromInput() {
    return this.page.locator('#date-from');
  }

  get customToInput() {
    return this.page.locator('#date-to');
  }

  async selectPreset(preset: string) {
    await this.page.locator(`.preset-btn:has-text("${preset}")`).click();
  }

  async expectLoaded() {
    await expect(this.title).toContainText('Reports');
    await expect(this.applyButton).toBeVisible();
  }
}
