import { type Page, expect } from '@playwright/test';

export class DashboardPage {
  constructor(private page: Page) {}

  async goto() {
    await this.page.goto('/');
    await this.page.waitForSelector('h1:has-text("Dashboard")');
  }

  get title() {
    return this.page.locator('h1.page-title');
  }

  get statCards() {
    return this.page.locator('.stat-card');
  }

  get deviceCards() {
    return this.page.locator('a.device-card');
  }

  get totalDevicesStat() {
    return this.page.locator('.stat-card').filter({ hasText: 'Total Devices' });
  }

  get onlineStat() {
    return this.page.locator('.stat-card').filter({ hasText: 'Online' });
  }

  get offlineStat() {
    return this.page.locator('.stat-card').filter({ hasText: 'Offline' });
  }

  get positionsStat() {
    return this.page.locator('.stat-card').filter({ hasText: 'Positions Today' });
  }

  get sectionTitle() {
    return this.page.locator('h2.section-title');
  }

  get emptyState() {
    return this.page.locator('.empty-state');
  }

  async getStatValue(label: string): Promise<string> {
    const card = this.page.locator('.stat-card').filter({ hasText: label });
    const value = card.locator('.stat-value');
    return (await value.textContent()) || '';
  }

  async expectLoaded() {
    await expect(this.title).toContainText('Dashboard');
    await expect(this.statCards.first()).toBeVisible();
  }
}
