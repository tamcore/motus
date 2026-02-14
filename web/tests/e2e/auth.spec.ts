import { test, expect } from '@playwright/test';
import { LoginPage } from '../page-objects/LoginPage';
import { TEST_CREDENTIALS, INVALID_CREDENTIALS } from '../fixtures/test-data';

test.describe('Authentication', () => {
  let loginPage: LoginPage;

  test.beforeEach(async ({ page }) => {
    loginPage = new LoginPage(page);
    await loginPage.goto();
  });

  test('should display login page with branding', async () => {
    await loginPage.expectOnLoginPage();
  });

  test('should show email and password inputs', async () => {
    await expect(loginPage.emailInput).toBeVisible();
    await expect(loginPage.passwordInput).toBeVisible();
    await expect(loginPage.submitButton).toBeVisible();
  });

  test('should have correct page title', async ({ page }) => {
    await expect(page).toHaveTitle('Login - Motus', { timeout: 10000 });
  });

  test('should login with valid credentials', async ({ page }) => {
    await loginPage.login(TEST_CREDENTIALS.email, TEST_CREDENTIALS.password);
    await page.waitForURL('/', { timeout: 15000 });
    await expect(page.locator('h1:has-text("Dashboard")')).toBeVisible({ timeout: 10000 });
  });

  test('should show error with invalid credentials', async () => {
    await loginPage.login(INVALID_CREDENTIALS.email, INVALID_CREDENTIALS.password);
    await loginPage.expectError('Invalid email or password');
  });

  test('should show error with empty password', async () => {
    await loginPage.fillEmail(TEST_CREDENTIALS.email);
    await loginPage.clickLogin();
    // Browser validation prevents submit, or app shows error
    await loginPage.expectOnLoginPage();
  });

  test('should redirect unauthenticated users to login', async ({ page }) => {
    // Clear any auth state
    await page.evaluate(() => localStorage.clear());
    await page.goto('/');
    await page.waitForURL(/\/login/, { timeout: 10000 });
    await loginPage.expectOnLoginPage();
  });

  test('should logout successfully', async ({ page }) => {
    // First login
    await loginPage.login(TEST_CREDENTIALS.email, TEST_CREDENTIALS.password);
    await page.waitForURL('/', { timeout: 15000 });
    await page.waitForSelector('h1:has-text("Dashboard")', { timeout: 10000 });

    // Click user menu and logout
    await page.click('.user-button');
    await page.click('button:has-text("Logout")');
    await page.waitForURL(/\/login/, { timeout: 10000 });
    await loginPage.expectOnLoginPage();
  });
});
