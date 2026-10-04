import { type Page } from '@playwright/test';

export class UsersPage {
  constructor(private page: Page) {}

  async goto() {
    await this.page.goto('/admin/users');
    await this.page.waitForSelector('h1.page-title', { timeout: 10000 });
  }

  get table() {
    return this.page.locator('table.users-table');
  }

  get cards() {
    return this.page.locator('.mobile-view .user-card');
  }

  get cardSummaries() {
    return this.cards.locator('.user-summary');
  }

  get detailGrid() {
    return this.cards.locator('.detail-grid').first();
  }

  get detailActions() {
    return this.cards.locator('.detail-actions').first();
  }
}
