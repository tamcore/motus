import type { Locator } from '@playwright/test';
import { test, expect } from '../fixtures/auth-fixture';
import { BookmarksPage } from '../page-objects/BookmarksPage';
import { DevicesPage } from '../page-objects/DevicesPage';
import { NotificationsPage } from '../page-objects/NotificationsPage';

async function box(locator: Locator) {
  const b = await locator.boundingBox();
  expect(b).toBeTruthy();
  return b!;
}

// Shared page header, container and modal rules live in app.css.
test.describe('Shared page layout', () => {
  test('uses the 1280px centered container on list pages', async ({ authedPage }) => {
    await new BookmarksPage(authedPage).goto();
    const container = await box(authedPage.locator('.bookmarks-page .container'));
    const vw = authedPage.viewportSize()!.width;
    expect(container.width).toBeGreaterThan(1200);
    expect(container.width).toBeLessThanOrEqual(1280);
    expect(Math.abs(container.x + container.width / 2 - vw / 2)).toBeLessThan(10);
  });

  test('puts the page title and header actions on one row', async ({ authedPage }) => {
    const notifications = new NotificationsPage(authedPage);
    await notifications.goto();
    const header = await box(authedPage.locator('.page-header'));
    const title = await box(notifications.title);
    const create = await box(notifications.createButton);
    expect(title.x - header.x).toBeLessThan(2);
    expect(header.x + header.width - (create.x + create.width)).toBeLessThan(2);
    expect(Math.abs(title.y + title.height / 2 - (create.y + create.height / 2))).toBeLessThan(6);
  });

  test('right-aligns modal actions', async ({ authedPage }) => {
    const devices = new DevicesPage(authedPage);
    await devices.goto();
    await devices.addDeviceButton.click();
    await expect(devices.modal).toBeVisible();

    const dialog = await box(devices.modal);
    const vw = authedPage.viewportSize()!.width;
    expect(Math.abs(dialog.x + dialog.width / 2 - vw / 2)).toBeLessThan(2);

    const actions = devices.modal.locator('.modal-actions');
    const cancel = await box(actions.locator('button:has-text("Cancel")'));
    const create = await box(actions.locator('button:has-text("Create Device")'));
    expect(Math.abs(cancel.y - create.y)).toBeLessThan(1);
    expect(create.x).toBeGreaterThan(cancel.x + cancel.width);
    expect(dialog.x + dialog.width - (create.x + create.width)).toBeLessThan(40);
  });

  test('stacks the admin page header on mobile', async ({ authedPage }) => {
    await authedPage.setViewportSize({ width: 390, height: 844 });
    await authedPage.goto('/admin/audit');
    const header = await box(authedPage.locator('.page-header'));
    const left = await box(authedPage.locator('.page-header-left'));
    const refresh = await box(authedPage.locator('.page-header button:has-text("Refresh")'));
    expect(left.x - header.x).toBeLessThan(2);
    expect(refresh.x - header.x).toBeLessThan(2);
    expect(refresh.y).toBeGreaterThanOrEqual(left.y + left.height);
  });
});
