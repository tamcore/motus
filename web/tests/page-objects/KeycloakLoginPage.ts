import { type Page } from '@playwright/test';

export class KeycloakLoginPage {
  constructor(private page: Page) {}

  async login(username: string, password: string) {
    await this.page.fill('#username', username);
    await this.page.fill('#password', password);
    await this.page.click('#kc-login');
  }
}
