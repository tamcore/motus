import { expect, type Page } from '@playwright/test';

/**
 * Waits for all loading spinners to disappear from the page.
 */
export async function waitForLoadingComplete(page: Page) {
  await expect(page.locator('.spinner')).toHaveCount(0, { timeout: 10000 });
}

/**
 * Waits for the Leaflet map container to be visible.
 */
export async function waitForMapReady(page: Page) {
  await page.waitForSelector('.leaflet-container', {
    state: 'visible',
    timeout: 15000,
  });
  // Allow tiles to start loading
  await page.waitForTimeout(500);
}

/**
 * Waits for a modal dialog to become visible.
 */
export async function waitForModal(page: Page) {
  await page.waitForSelector('[role="dialog"]', {
    state: 'visible',
    timeout: 5000,
  });
}

/**
 * Waits for a modal dialog to close.
 */
export async function waitForModalClose(page: Page) {
  await expect(page.locator('[role="dialog"]')).toHaveCount(0, {
    timeout: 5000,
  });
}

/**
 * Waits for a network request and its response.
 */
export async function waitForApiResponse(
  page: Page,
  urlPattern: string,
  action: () => Promise<void>,
) {
  const [response] = await Promise.all([
    page.waitForResponse((resp) => resp.url().includes(urlPattern)),
    action(),
  ]);
  return response;
}
