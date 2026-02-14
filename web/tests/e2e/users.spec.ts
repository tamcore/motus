import { test, expect } from '../fixtures/auth-fixture';

test.describe('Admin User Management', () => {
  test.beforeEach(async ({ authedPage }) => {
    await authedPage.goto('/admin/users');
    await authedPage.waitForSelector('h1.page-title', { timeout: 10000 });
  });

  test('should display User Management page title', async ({ authedPage }) => {
    await expect(authedPage.locator('h1.page-title')).toContainText('User Management');
  });

  test('should have correct page title in tab', async ({ authedPage }) => {
    await expect(authedPage).toHaveTitle(/User Management|Motus/);
  });

  test('should show Add User button', async ({ authedPage }) => {
    await expect(authedPage.locator('button:has-text("Add User")')).toBeVisible();
  });

  test('should display at least one user in the table', async ({ authedPage }) => {
    // The admin user who is logged in should be visible.
    const table = authedPage.locator('table.users-table');
    await expect(table).toBeVisible();
    const rows = table.locator('tbody tr.table-row');
    const count = await rows.count();
    expect(count).toBeGreaterThan(0);
  });

  test('should show the logged-in admin user in the table', async ({ authedPage }) => {
    await expect(authedPage.locator('table.users-table')).toContainText('admin@motus.local');
  });

  test('should open Add User modal', async ({ authedPage }) => {
    await authedPage.click('button:has-text("Add User")');
    await expect(authedPage.locator('[role="dialog"]')).toBeVisible();
    await expect(authedPage.locator('[role="dialog"] #modal-title, [role="dialog"] .modal-title')).toContainText('Add User');
  });

  test('should close modal on Cancel', async ({ authedPage }) => {
    await authedPage.click('button:has-text("Add User")');
    await expect(authedPage.locator('[role="dialog"]')).toBeVisible();
    await authedPage.click('[role="dialog"] button:has-text("Cancel")');
    await expect(authedPage.locator('[role="dialog"]')).toHaveCount(0);
  });

  test('should create a new user', async ({ authedPage }) => {
    const uniqueEmail = `pw-user-${Date.now()}@example.com`;

    await authedPage.click('button:has-text("Add User")');
    await expect(authedPage.locator('[role="dialog"]')).toBeVisible();

    await authedPage.fill('[role="dialog"] input[name="userName"]', 'PW Test User');
    await authedPage.fill('[role="dialog"] input[name="userEmail"]', uniqueEmail);
    await authedPage.fill('[role="dialog"] input[name="userPassword"]', 'password123');

    await authedPage.click('[role="dialog"] button:has-text("Create User")');

    // Modal should close.
    await expect(authedPage.locator('[role="dialog"]')).toHaveCount(0, { timeout: 5000 });

    // New user should appear in the table.
    await expect(authedPage.locator('table.users-table').locator(`text=${uniqueEmail}`)).toBeVisible({ timeout: 5000 });
  });

  test('should edit an existing user name', async ({ authedPage }) => {
    const uniqueEmail = `pw-edit-${Date.now()}@example.com`;
    const updatedName = `Edited-${Date.now()}`;

    // Create a user first.
    await authedPage.click('button:has-text("Add User")');
    await authedPage.fill('[role="dialog"] input[name="userName"]', 'Before Edit');
    await authedPage.fill('[role="dialog"] input[name="userEmail"]', uniqueEmail);
    await authedPage.fill('[role="dialog"] input[name="userPassword"]', 'password123');
    await authedPage.click('[role="dialog"] button:has-text("Create User")');
    await expect(authedPage.locator('[role="dialog"]')).toHaveCount(0, { timeout: 5000 });
    await expect(authedPage.locator('table.users-table').locator(`text=${uniqueEmail}`)).toBeVisible({ timeout: 5000 });

    // Click Edit on the row containing the user.
    const row = authedPage.locator('table.users-table tbody tr.table-row').filter({ hasText: uniqueEmail });
    await row.locator('button:has-text("Edit")').click();

    await expect(authedPage.locator('[role="dialog"]')).toBeVisible();
    await authedPage.fill('[role="dialog"] input[name="userName"]', updatedName);
    await authedPage.click('[role="dialog"] button:has-text("Update User")');

    await expect(authedPage.locator('[role="dialog"]')).toHaveCount(0, { timeout: 5000 });
    await expect(authedPage.locator('table.users-table').locator(`text=${updatedName}`)).toBeVisible({ timeout: 5000 });
  });

  test('should delete a user', async ({ authedPage }) => {
    const uniqueEmail = `pw-delete-${Date.now()}@example.com`;

    // Create a user to delete.
    await authedPage.click('button:has-text("Add User")');
    await authedPage.fill('[role="dialog"] input[name="userName"]', 'Delete Me');
    await authedPage.fill('[role="dialog"] input[name="userEmail"]', uniqueEmail);
    await authedPage.fill('[role="dialog"] input[name="userPassword"]', 'password123');
    await authedPage.click('[role="dialog"] button:has-text("Create User")');
    await expect(authedPage.locator('[role="dialog"]')).toHaveCount(0, { timeout: 5000 });
    await expect(authedPage.locator('table.users-table').locator(`text=${uniqueEmail}`)).toBeVisible({ timeout: 5000 });

    // Accept the confirm dialog when Delete is clicked.
    authedPage.on('dialog', (dialog) => dialog.accept());

    const row = authedPage.locator('table.users-table tbody tr.table-row').filter({ hasText: uniqueEmail });
    await row.locator('button:has-text("Delete")').click();

    // User should be removed from the table.
    await expect(authedPage.locator('table.users-table').locator(`text=${uniqueEmail}`)).toHaveCount(0, { timeout: 5000 });
  });
});
