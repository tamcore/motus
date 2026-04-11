import { test, expect } from '@playwright/test';
import { LoginPage } from '../page-objects/LoginPage';
import { KeycloakLoginPage } from '../page-objects/KeycloakLoginPage';
import { OIDC_ADMIN_CREDENTIALS, OIDC_USER_CREDENTIALS } from '../fixtures/test-data';

test.describe('OIDC Authentication', () => {
  let loginPage: LoginPage;

  test.beforeEach(async ({ page }) => {
    loginPage = new LoginPage(page);
    await loginPage.goto();
  });

  test('should display SSO button when OIDC is enabled', async () => {
    await loginPage.expectSSOButtonVisible();
    await expect(loginPage.ssoButton).toContainText('Continue with SSO');
  });

  test('admin can login via OIDC', async ({ page }) => {
    await loginPage.expectSSOButtonVisible();
    await loginPage.clickSSO();

    // Keycloak login page
    const keycloakPage = new KeycloakLoginPage(page);
    await keycloakPage.login(OIDC_ADMIN_CREDENTIALS.username, OIDC_ADMIN_CREDENTIALS.password);

    // Should redirect back to motus dashboard
    await page.waitForURL('**/', { timeout: 30000 });
    await expect(page.locator('h1:has-text("Dashboard")')).toBeVisible({ timeout: 15000 });

    // Admin should see Admin nav link
    await expect(page.locator('a[href="/admin/users"]')).toBeVisible();
  });

  test('regular user can login via OIDC', async ({ page }) => {
    await loginPage.expectSSOButtonVisible();
    await loginPage.clickSSO();

    // Keycloak login page
    const keycloakPage = new KeycloakLoginPage(page);
    await keycloakPage.login(OIDC_USER_CREDENTIALS.username, OIDC_USER_CREDENTIALS.password);

    // Should redirect back to motus dashboard
    await page.waitForURL('**/', { timeout: 30000 });
    await expect(page.locator('h1:has-text("Dashboard")')).toBeVisible({ timeout: 15000 });

    // Regular user should NOT see Admin nav link
    await expect(page.locator('a[href="/admin/users"]')).toHaveCount(0);
  });

  test('OIDC user can logout', async ({ page }) => {
    await loginPage.expectSSOButtonVisible();
    await loginPage.clickSSO();

    const keycloakPage = new KeycloakLoginPage(page);
    await keycloakPage.login(OIDC_ADMIN_CREDENTIALS.username, OIDC_ADMIN_CREDENTIALS.password);

    await page.waitForURL('**/', { timeout: 30000 });
    await expect(page.locator('h1:has-text("Dashboard")')).toBeVisible({ timeout: 15000 });

    // Logout
    await page.click('.user-button');
    await page.click('button:has-text("Logout")');
    await page.waitForURL(/\/login/, { timeout: 10000 });
    await loginPage.expectOnLoginPage();
  });
});
