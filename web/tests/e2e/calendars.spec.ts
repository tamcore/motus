import { test, expect } from '@playwright/test';
import { mockFetch } from '../helpers/mock-fetch';

test.describe('Calendar editor preview', () => {
  test('shows the server-checked active status inside the preview card', async ({ page }) => {
    await mockFetch(page, [{ path: '/api/calendars/check', method: 'POST', body: { active: true } }]);
    await page.goto('/calendars');
    await page.getByRole('button', { name: '+ Create Calendar' }).click();

    const card = page.locator('.preview-card');
    const status = card.locator('.preview-status');
    await expect(status).toHaveText('Active now');
    await expect(status).toHaveClass(/active/);

    const cardBox = await card.boundingBox();
    const statusBox = await status.boundingBox();
    expect(cardBox && statusBox).toBeTruthy();
    expect(statusBox!.y).toBeGreaterThanOrEqual(cardBox!.y);
    expect(statusBox!.y + statusBox!.height).toBeLessThanOrEqual(cardBox!.y + cardBox!.height);
  });
});
