import { test as base, expect, type Page } from '@playwright/test';

/** `authedPage`: authenticated via the shared storageState (auth.setup.ts) and on the dashboard. */
export const test = base.extend<{ authedPage: Page }>({
  authedPage: async ({ page }, use) => {
    await page.goto('/');
    await page.waitForSelector('h1:has-text("Dashboard")', { timeout: 10000 });
    await use(page);
  },
});

export { expect };
