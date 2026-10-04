import type { Locator } from '@playwright/test';
import { test, expect } from '../fixtures/auth-fixture';
import { BookmarksPage } from '../page-objects/BookmarksPage';
import { DevicesPage } from '../page-objects/DevicesPage';
import { NotificationsPage } from '../page-objects/NotificationsPage';
import { ReportsPage } from '../page-objects/ReportsPage';
import { mockFetch } from '../helpers/mock-fetch';

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
    expect(Math.abs(title.x - header.x)).toBeLessThan(2);
    expect(Math.abs(header.x + header.width - (create.x + create.width))).toBeLessThan(2);
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
    expect(Math.abs(dialog.x + dialog.width - (create.x + create.width))).toBeLessThan(40);
  });

  test('stacks the admin page header on mobile', async ({ authedPage }) => {
    await authedPage.setViewportSize({ width: 390, height: 844 });
    await authedPage.goto('/admin/audit');
    const header = await box(authedPage.locator('.page-header'));
    const left = await box(authedPage.locator('.page-header-left'));
    const refresh = await box(authedPage.locator('.page-header button:has-text("Refresh")'));
    expect(Math.abs(left.x - header.x)).toBeLessThan(2);
    expect(Math.abs(refresh.x - header.x)).toBeLessThan(2);
    expect(refresh.y).toBeGreaterThanOrEqual(left.y + left.height);
  });

  test('has no horizontal overflow on /notifications at phone width', async ({ authedPage }) => {
    await authedPage.setViewportSize({ width: 390, height: 844 });
    const notifications = new NotificationsPage(authedPage);
    await notifications.goto();
    const header = await box(authedPage.locator('.page-header'));
    const title = await box(notifications.title);
    const create = await box(notifications.createButton);
    expect(header.x + header.width).toBeLessThanOrEqual(390);
    expect(create.x + create.width).toBeLessThanOrEqual(header.x + header.width + 1);
    expect(create.y).toBeGreaterThanOrEqual(title.y + title.height);
    const overflow = await authedPage.evaluate(
      () => document.documentElement.scrollWidth - document.documentElement.clientWidth
    );
    expect(overflow).toBeLessThanOrEqual(0);
  });

  test('keeps the bookmarks page header in the empty state', async ({ authedPage }) => {
    await mockFetch(authedPage, [{ path: '/api/trail-bookmarks', body: [] }]);
    const bookmarks = new BookmarksPage(authedPage);
    await bookmarks.goto();
    await expect(bookmarks.emptyState).toBeVisible();
    const header = await box(authedPage.locator('.bookmarks-page .page-header'));
    const title = await box(bookmarks.heading);
    const icon = await box(authedPage.locator('.page-header .page-icon'));
    expect(Math.abs(icon.x - header.x)).toBeLessThan(2);
    expect(title.x).toBeGreaterThan(icon.x + icon.width);
    expect(title.y).toBeGreaterThanOrEqual(header.y - 1);
    expect(title.y + title.height).toBeLessThanOrEqual(header.y + header.height + 1);
  });

  test('stretches report filter actions to the bar width on mobile', async ({ authedPage }) => {
    await authedPage.setViewportSize({ width: 390, height: 844 });
    const reports = new ReportsPage(authedPage);
    await reports.goto();
    const bar = authedPage.locator('.filters-bar');
    const inner = await bar.evaluate((el) => {
      const cs = getComputedStyle(el);
      return el.clientWidth - parseFloat(cs.paddingLeft) - parseFloat(cs.paddingRight);
    });
    const apply = await box(reports.applyButton);
    expect(Math.abs(apply.width - inner)).toBeLessThan(2);
  });

  test('stretches the replay load button to the bar width on mobile', async ({ authedPage }) => {
    await authedPage.setViewportSize({ width: 390, height: 844 });
    await authedPage.goto('/reports/replay');
    const bar = authedPage.locator('.filters-bar');
    await expect(bar).toBeVisible();
    const inner = await bar.evaluate((el) => {
      const cs = getComputedStyle(el);
      return el.clientWidth - parseFloat(cs.paddingLeft) - parseFloat(cs.paddingRight);
    });
    const load = await box(bar.locator('button:has-text("Load Drive")'));
    expect(Math.abs(load.width - inner)).toBeLessThan(2);
  });
});
