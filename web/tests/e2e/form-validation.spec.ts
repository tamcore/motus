import { test, expect } from '../fixtures/auth-fixture';

test.describe('Form Validation', () => {
  test.describe('Login Form', () => {
    test('should have required attribute on email input', async ({ page }) => {
      await page.goto('/login');
      await page.waitForSelector('input[name="email"]');
      const required = await page.locator('input[name="email"]').getAttribute('required');
      expect(required).not.toBeNull();
    });

    test('should have required attribute on password input', async ({ page }) => {
      await page.goto('/login');
      await page.waitForSelector('input[name="password"]');
      const required = await page.locator('input[name="password"]').getAttribute('required');
      expect(required).not.toBeNull();
    });

    test('should show required asterisk for email label', async ({ page }) => {
      await page.goto('/login');
      await page.waitForSelector('input[name="email"]');
      const asterisk = page.locator('label[for="email"] .required');
      await expect(asterisk).toBeVisible();
    });

    test('should show required asterisk for password label', async ({ page }) => {
      await page.goto('/login');
      await page.waitForSelector('input[name="password"]');
      const asterisk = page.locator('label[for="password"] .required');
      await expect(asterisk).toBeVisible();
    });

    test('should have email type on email input', async ({ page }) => {
      await page.goto('/login');
      const type = await page.locator('input[name="email"]').getAttribute('type');
      expect(type).toBe('email');
    });

    test('should have password type on password input', async ({ page }) => {
      await page.goto('/login');
      const type = await page.locator('input[name="password"]').getAttribute('type');
      expect(type).toBe('password');
    });
  });

  test.describe('Device Form', () => {
    test('should show error when name and identifier are empty', async ({ authedPage }) => {
      await authedPage.goto('/devices');
      await authedPage.waitForSelector('h1:has-text("Devices")');
      await authedPage.click('button:has-text("Add Device")');
      await authedPage.waitForSelector('[role="dialog"]');

      // Try to submit empty form
      await authedPage.click('[role="dialog"] button:has-text("Create Device")');
      await expect(authedPage.locator('.form-error')).toContainText('required');
    });

    test('should show error when only name is provided', async ({ authedPage }) => {
      await authedPage.goto('/devices');
      await authedPage.waitForSelector('h1:has-text("Devices")');
      await authedPage.click('button:has-text("Add Device")');
      await authedPage.waitForSelector('[role="dialog"]');

      await authedPage.fill('[role="dialog"] input[name="name"]', 'Test Device');
      await authedPage.click('[role="dialog"] button:has-text("Create Device")');
      await expect(authedPage.locator('.form-error')).toContainText('required');
    });

    test('should show error when only identifier is provided', async ({ authedPage }) => {
      await authedPage.goto('/devices');
      await authedPage.waitForSelector('h1:has-text("Devices")');
      await authedPage.click('button:has-text("Add Device")');
      await authedPage.waitForSelector('[role="dialog"]');

      await authedPage.fill('[role="dialog"] input[name="uniqueId"]', 'test-123');
      await authedPage.click('[role="dialog"] button:has-text("Create Device")');
      await expect(authedPage.locator('.form-error')).toContainText('required');
    });

    test('should accept valid name and identifier', async ({ authedPage }) => {
      await authedPage.goto('/devices');
      await authedPage.waitForSelector('h1:has-text("Devices")');
      await authedPage.click('button:has-text("Add Device")');
      await authedPage.waitForSelector('[role="dialog"]');

      const uid = `valid-test-${Date.now()}`;
      await authedPage.fill('[role="dialog"] input[name="name"]', 'Valid Device');
      await authedPage.fill('[role="dialog"] input[name="uniqueId"]', uid);
      await authedPage.click('[role="dialog"] button:has-text("Create Device")');

      // Modal should close on success
      await expect(authedPage.locator('[role="dialog"]')).toHaveCount(0, { timeout: 15000 });
    });

    test('should trim whitespace from name', async ({ authedPage }) => {
      await authedPage.goto('/devices');
      await authedPage.waitForSelector('h1:has-text("Devices")');
      await authedPage.click('button:has-text("Add Device")');
      await authedPage.waitForSelector('[role="dialog"]');

      // Whitespace-only name should trigger validation
      await authedPage.fill('[role="dialog"] input[name="name"]', '   ');
      await authedPage.fill('[role="dialog"] input[name="uniqueId"]', `ws-test-${Date.now()}`);
      await authedPage.click('[role="dialog"] button:has-text("Create Device")');
      await expect(authedPage.locator('.form-error')).toContainText('required');
    });

    test('should disable identifier field when editing', async ({ authedPage }) => {
      await authedPage.goto('/devices');
      await authedPage.waitForSelector('h1:has-text("Devices")');

      // Check if there are devices to edit
      const editBtns = authedPage.locator('.device-table tbody tr.table-row button:has-text("Edit")');
      const count = await editBtns.count();
      if (count === 0) return;

      await editBtns.first().click();
      await authedPage.waitForSelector('[role="dialog"]');

      const uniqueIdInput = authedPage.locator('[role="dialog"] input[name="uniqueId"]');
      await expect(uniqueIdInput).toBeDisabled();
    });
  });

  test.describe('Notification Form', () => {
    test('should show required fields in notification modal', async ({ authedPage }) => {
      await authedPage.goto('/notifications');
      await authedPage.waitForSelector('h1:has-text("Notification Rules")');
      await authedPage.click('button:has-text("Create Rule")');
      await authedPage.waitForSelector('[role="dialog"]');

      // Name input should be required
      const nameRequired = await authedPage
        .locator('[role="dialog"] input[name="name"]')
        .getAttribute('required');
      expect(nameRequired).not.toBeNull();

      // Webhook URL should be required
      const urlRequired = await authedPage
        .locator('[role="dialog"] input[name="webhookUrl"]')
        .getAttribute('required');
      expect(urlRequired).not.toBeNull();
    });

    test('should have default template pre-filled', async ({ authedPage }) => {
      await authedPage.goto('/notifications');
      await authedPage.waitForSelector('h1:has-text("Notification Rules")');
      await authedPage.click('button:has-text("Create Rule")');
      await authedPage.waitForSelector('[role="dialog"]');

      const templateValue = await authedPage.locator('#template').inputValue();
      expect(templateValue).toContain('device');
      expect(templateValue).toContain('event');
    });
  });
});
