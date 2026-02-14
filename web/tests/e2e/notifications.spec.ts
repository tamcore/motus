import { test, expect } from '../fixtures/auth-fixture';
import { NotificationsPage } from '../page-objects/NotificationsPage';

test.describe('Notifications Page', () => {
  let notifPage: NotificationsPage;

  test.beforeEach(async ({ authedPage }) => {
    notifPage = new NotificationsPage(authedPage);
    await notifPage.goto();
  });

  test('should display notifications page title', async () => {
    await notifPage.expectLoaded();
  });

  test('should have correct page title in tab', async ({ authedPage }) => {
    await expect(authedPage).toHaveTitle('Notifications - Motus');
  });

  test('should show Create Rule button', async () => {
    await expect(notifPage.createButton).toBeVisible();
  });

  test('should show empty state initially', async () => {
    await expect(notifPage.emptyState).toBeVisible();
    await expect(notifPage.emptyState).toContainText('No notification rules yet');
  });

  test('should show Create first rule button in empty state', async ({ authedPage }) => {
    const firstRuleBtn = authedPage.locator('.empty-state button:has-text("Create your first rule")');
    await expect(firstRuleBtn).toBeVisible();
  });

  test('should open create modal when clicking Create Rule', async () => {
    await notifPage.createButton.click();
    await expect(notifPage.modal).toBeVisible();
    await expect(notifPage.modalTitle).toContainText('Create Notification Rule');
  });

  test('should show all form fields in create modal', async () => {
    await notifPage.createButton.click();
    await expect(notifPage.nameInput).toBeVisible();
    await expect(notifPage.eventTypeCheckboxes.first()).toBeVisible();
    await expect(notifPage.webhookUrlInput).toBeVisible();
    await expect(notifPage.templateTextarea).toBeVisible();
  });

  test('should show event type checkboxes', async ({ authedPage }) => {
    await notifPage.createButton.click();
    const count = await notifPage.eventTypeCheckboxes.count();
    expect(count).toBeGreaterThanOrEqual(6);
  });

  test('should show template variables section', async () => {
    await notifPage.createButton.click();
    await expect(notifPage.variableButtons.first()).toBeVisible();
    const count = await notifPage.variableButtons.count();
    expect(count).toBeGreaterThanOrEqual(8); // 8 template variables
  });

  test('should insert variable into template on click', async ({ authedPage }) => {
    await notifPage.createButton.click();
    // Clear template first
    await notifPage.templateTextarea.fill('');
    // Click a variable button
    await notifPage.variableButtons.first().click();
    const value = await notifPage.templateTextarea.inputValue();
    expect(value).toContain('{{');
  });

  test('should show Add Header button', async () => {
    await notifPage.createButton.click();
    await expect(notifPage.addHeaderButton).toBeVisible();
  });

  test('should add header row when clicking Add Header', async ({ authedPage }) => {
    await notifPage.createButton.click();
    await notifPage.addHeaderButton.click();
    const headerRows = authedPage.locator('[role="dialog"] .header-row');
    await expect(headerRows).toHaveCount(1);
  });

  test('should close modal on Cancel', async () => {
    await notifPage.createButton.click();
    await expect(notifPage.modal).toBeVisible();
    await notifPage.cancelButton.click();
    await expect(notifPage.modal).toHaveCount(0);
  });

  test('should close modal on X button', async ({ authedPage }) => {
    await notifPage.createButton.click();
    await expect(notifPage.modal).toBeVisible();
    await authedPage.click('[role="dialog"] .close-button');
    await expect(notifPage.modal).toHaveCount(0);
  });

  test('should close modal on Escape key', async ({ authedPage }) => {
    await notifPage.createButton.click();
    await expect(notifPage.modal).toBeVisible();
    await authedPage.keyboard.press('Escape');
    await expect(notifPage.modal).toHaveCount(0);
  });

  test('should create notification rule', async ({ authedPage }) => {
    const ruleName = `Test Alert ${Date.now()}`;
    await notifPage.createButton.click();
    await notifPage.nameInput.fill(ruleName);
    await notifPage.selectEventTypes('geofenceEnter');
    await notifPage.webhookUrlInput.fill('https://example.com/hook');
    await notifPage.submitButton.click();

    // Modal should close, rule should appear
    await expect(notifPage.modal).toHaveCount(0, { timeout: 10000 });
    await expect(notifPage.ruleCards.first()).toBeVisible();
    await expect(authedPage.locator(`.rule-name:has-text("${ruleName}")`)).toBeVisible();
  });

  test('should show toggle switch on rule card', async ({ authedPage }) => {
    // Create a rule first
    const ruleName = `Toggle Test ${Date.now()}`;
    await notifPage.createButton.click();
    await notifPage.nameInput.fill(ruleName);
    await notifPage.selectEventTypes('geofenceEnter');
    await notifPage.webhookUrlInput.fill('https://example.com/hook');
    await notifPage.submitButton.click();
    await expect(notifPage.modal).toHaveCount(0, { timeout: 10000 });

    // The checkbox input is hidden (opacity:0) for the custom toggle UI
    const toggleSwitch = notifPage.ruleCards.first().locator('.toggle-switch');
    await expect(toggleSwitch).toBeVisible();
  });

  test('should show Edit and Delete buttons on rule card', async ({ authedPage }) => {
    // Create a rule first
    const ruleName = `Button Test ${Date.now()}`;
    await notifPage.createButton.click();
    await notifPage.nameInput.fill(ruleName);
    await notifPage.selectEventTypes('geofenceEnter');
    await notifPage.webhookUrlInput.fill('https://example.com/hook');
    await notifPage.submitButton.click();
    await expect(notifPage.modal).toHaveCount(0, { timeout: 10000 });

    await expect(notifPage.getRuleEditButton(0)).toBeVisible();
    await expect(notifPage.getRuleDeleteButton(0)).toBeVisible();
    await expect(notifPage.getRuleTestButton(0)).toBeVisible();
  });
});
