import { test as base, expect, type Page } from '@playwright/test';
import { TEST_CREDENTIALS } from './test-data';

/**
 * Provides an `authedPage` fixture that is already authenticated via
 * the shared storageState (set up by auth.setup.ts) and navigated to
 * the dashboard.
 *
 * Falls back to a manual login when the stored session has expired.
 */
type AuthFixtures = {
  authedPage: Page;
};

export const test = base.extend<AuthFixtures>({
  authedPage: async ({ page }, use) => {
    await page.goto('/');

    try {
      await page.waitForSelector('h1:has-text("Dashboard")', { timeout: 10000 });
    } catch {
      // storageState session may have expired – re-authenticate
      if (page.url().includes('/login')) {
        await page.fill('input[name="email"]', TEST_CREDENTIALS.email);
        await page.fill('input[name="password"]', TEST_CREDENTIALS.password);
        await page.click('button:has-text("Login")');
        await page.waitForURL('/', { timeout: 10000 });
        await page.waitForSelector('h1:has-text("Dashboard")', { timeout: 10000 });
      }
    }

    await use(page);
  },
});

export { expect };
