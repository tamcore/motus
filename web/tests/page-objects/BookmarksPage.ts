import { type Page } from '@playwright/test';

export class BookmarksPage {
  constructor(private page: Page) {}

  async goto() {
    await this.page.goto('/bookmarks');
    await this.heading.waitFor({ state: 'visible' });
  }

  get heading() {
    return this.page.locator('h1:has-text("Trail Bookmarks")');
  }

  get cards() {
    return this.page.locator('.bookmark-card');
  }

  card(name: string) {
    return this.cards.filter({ has: this.page.locator('.card-name', { hasText: name }) });
  }

  get emptyState() {
    return this.page.locator('.empty-state');
  }

  get searchInput() {
    return this.page.locator('.bookmark-search');
  }

  get dialog() {
    return this.page.locator('.modal[role="dialog"]');
  }

  get nameInput() {
    return this.page.locator('#bookmark-name');
  }

  get submit() {
    return this.dialog.locator('button[type="submit"]');
  }
}
