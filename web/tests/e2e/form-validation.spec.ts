import { test, expect } from '../fixtures/auth-fixture';

test.describe('Form Validation', () => {
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
