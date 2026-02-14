import { test as setup } from '@playwright/test';
import { TEST_CREDENTIALS } from './fixtures/test-data';

const authFile = '.auth/user.json';

setup('authenticate', async ({ page }) => {
  await page.goto('/login');
  await page.waitForSelector('h1:has-text("Motus")', { timeout: 15000 });
  await page.fill('input[name="email"]', TEST_CREDENTIALS.email);
  await page.fill('input[name="password"]', TEST_CREDENTIALS.password);
  await page.click('button:has-text("Login")');
  await page.waitForURL('**/', { timeout: 15000 });
  await page.waitForSelector('h1:has-text("Dashboard")', { timeout: 15000 });

  await page.context().storageState({ path: authFile });
});
