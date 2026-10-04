import { type Locator, type Page, expect } from '@playwright/test';

export class UsersPage {
  constructor(private page: Page) {}

  async goto() {
    await this.page.goto('/admin/users');
    await this.page.waitForSelector('h1.page-title', { timeout: 10000 });
  }

  get table() {
    return this.page.locator('table.users-table');
  }

  get modal() {
    return this.page.locator('[role="dialog"]');
  }

  row(text: string) {
    return this.table.locator('tbody tr.table-row').filter({ hasText: text });
  }

  /** Creates a user through the modal and returns its desktop-table row. */
  async createUser(name: string, email: string): Promise<Locator> {
    await this.page.click('button:has-text("Add User")');
    await this.modal.locator('input[name="userName"]').fill(name);
    await this.modal.locator('input[name="userEmail"]').fill(email);
    await this.modal.locator('input[name="userPassword"]').fill('password123');
    await this.modal.locator('button:has-text("Create User")').click();
    await expect(this.modal).toHaveCount(0, { timeout: 5000 });
    const row = this.row(email);
    await expect(row).toBeVisible({ timeout: 5000 });
    return row;
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
