import { test, expect } from '../fixtures/auth-fixture';

test.describe('Pull to Refresh', () => {
  test('should show pull-to-refresh indicator on touch pull down', async ({ authedPage }) => {
    // Navigate to dashboard
    await authedPage.goto('/');
    await authedPage.waitForSelector('h1:has-text("Dashboard")');

    const scrollContainer = authedPage.locator('.ptr-container');
    await expect(scrollContainer).toBeVisible();

    // Simulate touch pull-down gesture via synthetic TouchEvents
    const box = await scrollContainer.boundingBox();
    if (!box) return;

    const startX = box.x + box.width / 2;
    const startY = box.y + 20;

    await authedPage.evaluate(async ({ x, y }) => {
      const el = document.querySelector('.ptr-container');
      if (!el) return;

      const touchStart = new TouchEvent('touchstart', {
        touches: [new Touch({ identifier: 0, target: el, clientX: x, clientY: y })],
        bubbles: true,
      });
      el.dispatchEvent(touchStart);

      // Move down 80px (past 60px threshold)
      const touchMove = new TouchEvent('touchmove', {
        touches: [new Touch({ identifier: 0, target: el, clientX: x, clientY: y + 80 })],
        bubbles: true,
        cancelable: true,
      });
      el.dispatchEvent(touchMove);
    }, { x: startX, y: startY });

    // The pull indicator should appear
    const indicator = authedPage.locator('.ptr-indicator');
    await expect(indicator).toBeVisible({ timeout: 3000 });
  });

  test('should have pull-to-refresh container on all main pages', async ({ authedPage }) => {
    const pages = ['/devices', '/map', '/reports', '/notifications', '/geofences'];

    for (const path of pages) {
      await authedPage.goto(path);
      await authedPage.waitForLoadState('networkidle');
      const container = authedPage.locator('.ptr-container');
      await expect(container).toBeVisible({ timeout: 10000 });
    }
  });
});
