import { type Page } from '@playwright/test';

export class SettingsPage {
  constructor(private page: Page) {}

  async goto() {
    await this.page.goto('/settings');
    await this.heading.waitFor({ state: 'visible' });
  }

  get heading() {
    return this.page.locator('h1:has-text("Settings")');
  }

  get container() {
    return this.page.locator('.settings-page .container');
  }

  section(title: string) {
    return this.page.locator('.settings-section', {
      has: this.page.locator('h2.section-title', { hasText: title }),
    });
  }

  get apiKeysSection() {
    return this.page.locator('.api-keys-section');
  }

  get sessionsSection() {
    return this.page.locator('.sessions-section');
  }

  get sessionCards() {
    return this.page.locator('.session-card');
  }

  get createApiKeyButton() {
    return this.apiKeysSection.locator('button:has-text("Create API Key")');
  }

  get dialog() {
    return this.page.locator('.modal[role="dialog"]');
  }
}
