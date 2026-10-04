import { type Page, expect } from "@playwright/test";

export class LoginPage {
  constructor(private page: Page) {}

  async goto() {
    await this.page.goto("/login");
    await this.page.waitForSelector('h1:has-text("Motus")');
  }

  async fillEmail(email: string) {
    await this.page.fill('input[name="email"]', email);
  }

  async fillPassword(password: string) {
    await this.page.fill('input[name="password"]', password);
  }

  async clickLogin() {
    await this.page.click('button:has-text("Login")');
  }

  async login(email: string, password: string) {
    await this.fillEmail(email);
    await this.fillPassword(password);
    await this.clickLogin();
  }

  async expectOnLoginPage() {
    await expect(this.page.locator('h1:has-text("Motus")')).toBeVisible();
  }

  async expectError(message: string) {
    await expect(this.page.locator(".error-message")).toContainText(message);
  }

  get emailInput() {
    return this.page.locator('input[name="email"]');
  }

  get passwordInput() {
    return this.page.locator('input[name="password"]');
  }

  get submitButton() {
    return this.page.locator('button:has-text("Login")');
  }

  get ssoButton() {
    return this.page.locator('a.sso-button');
  }

  async expectSSOButtonVisible() {
    await expect(this.ssoButton).toBeVisible();
  }

  async clickSSO() {
    await this.ssoButton.click();
  }
}
