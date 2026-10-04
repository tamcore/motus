import { test as setup } from '@playwright/test';
import { TEST_CREDENTIALS } from './fixtures/test-data';
import { LoginPage } from './page-objects/LoginPage';

const authFile = '.auth/user.json';

setup('authenticate', async ({ page }) => {
  await page.goto('/login');
  await page.waitForSelector('h1:has-text("Motus")', { timeout: 15000 });
  await new LoginPage(page).login(TEST_CREDENTIALS.email, TEST_CREDENTIALS.password);
  await page.waitForURL('**/', { timeout: 15000 });
  await page.waitForSelector('h1:has-text("Dashboard")', { timeout: 15000 });

  await page.context().storageState({ path: authFile });
});
