import { test, expect } from '../fixtures/auth-fixture';
import { mockFetch } from '../helpers/mock-fetch';
import { UsersPage } from '../page-objects/UsersPage';

test.describe('Admin User Management', () => {
  let users: UsersPage;

  test.beforeEach(async ({ authedPage }) => {
    users = new UsersPage(authedPage);
    await users.goto();
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

  test('should show the logged-in admin user in the table', async () => {
    await expect(users.row('admin@motus.local')).toHaveCount(1);
  });

  test('should open Add User modal', async ({ authedPage }) => {
    await authedPage.click('button:has-text("Add User")');
    await expect(users.modal).toBeVisible();
    await expect(users.modal.locator('#modal-title, .modal-title')).toContainText('Add User');
  });

  test('should close modal on Cancel', async ({ authedPage }) => {
    await authedPage.click('button:has-text("Add User")');
    await expect(users.modal).toBeVisible();
    await users.modal.locator('button:has-text("Cancel")').click();
    await expect(users.modal).toHaveCount(0);
  });

  test('should create a new user', async () => {
    await users.createUser('PW Test User', `pw-user-${Date.now()}@example.com`);
  });

  test('should edit an existing user name', async () => {
    const updatedName = `Edited-${Date.now()}`;
    const row = await users.createUser('Before Edit', `pw-edit-${Date.now()}@example.com`);

    await row.locator('button:has-text("Edit")').click();
    await expect(users.modal).toBeVisible();
    await users.modal.locator('input[name="userName"]').fill(updatedName);
    await users.modal.locator('button:has-text("Update User")').click();

    await expect(users.modal).toHaveCount(0, { timeout: 5000 });
    await expect(users.row(updatedName)).toBeVisible({ timeout: 5000 });
  });

  test('should delete a user', async ({ authedPage }) => {
    const row = await users.createUser('Delete Me', `pw-delete-${Date.now()}@example.com`);

    authedPage.on('dialog', (dialog) => dialog.accept());
    await row.locator('button:has-text("Delete")').click();
    await expect(row).toHaveCount(0, { timeout: 5000 });
  });
});

test.describe('Sudo bar', () => {
  test('shows when the sudo status reports an active session', async ({ authedPage }) => {
    await mockFetch(authedPage, [
      { path: '/api/admin/sudo', method: 'GET', body: { active: true, originalUserId: 1, targetUserId: 2 } },
    ]);
    await authedPage.goto('/');

    const bar = authedPage.locator('.sudo-bar');
    await expect(bar).toBeVisible();
    await expect(bar).toContainText('SUDO MODE');
    await expect(bar.locator('button.sudo-exit-btn')).toBeVisible();
  });

  test('stays hidden without a sudo session', async ({ authedPage }) => {
    await authedPage.goto('/');
    await expect(authedPage.locator('h1:has-text("Dashboard")')).toBeVisible();
    await expect(authedPage.locator('.sudo-bar')).toHaveCount(0);
  });
});

test.describe('Admin users mobile cards', () => {
  test('stacks full-width actions in an expanded card', async ({ authedPage }) => {
    await authedPage.setViewportSize({ width: 390, height: 844 });
    const users = new UsersPage(authedPage);
    await users.goto();
    await expect(users.table).toBeHidden();

    await users.cardSummaries.first().click();
    await expect(users.cards.first()).toHaveJSProperty('open', true);

    const items = users.detailGrid.locator('.detail-item');
    const a = (await items.nth(0).boundingBox())!;
    const b = (await items.nth(1).boundingBox())!;
    expect(Math.abs(a.y - b.y)).toBeLessThan(1);
    expect(b.x).toBeGreaterThan(a.x + a.width - 1);

    const actions = users.detailActions;
    const box = (await actions.boundingBox())!;
    const buttons = actions.locator('.btn');
    expect(await buttons.count()).toBeGreaterThan(1);
    const first = (await buttons.nth(0).boundingBox())!;
    const second = (await buttons.nth(1).boundingBox())!;
    expect(Math.abs(first.width - box.width)).toBeLessThan(1);
    expect(second.y).toBeGreaterThan(first.y + first.height);

    await actions.locator('.btn:has-text("Devices")').click();
    await expect(authedPage.locator('.modal[role="dialog"]')).toBeVisible();
    await expect(users.cards.first()).toHaveJSProperty('open', true);
  });
});
