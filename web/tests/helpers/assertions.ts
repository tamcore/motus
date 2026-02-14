import { expect, type Page } from '@playwright/test';

/**
 * Asserts the number of rows in a table body.
 */
export async function expectTableRowCount(
  page: Page,
  tableSelector: string,
  expectedCount: number,
) {
  const rows = page.locator(`${tableSelector} tbody tr`);
  await expect(rows).toHaveCount(expectedCount);
}

/**
 * Asserts that a page title is set correctly.
 */
export async function expectPageTitle(page: Page, title: string) {
  await expect(page).toHaveTitle(title);
}

/**
 * Asserts that an element with text is visible on the page.
 */
export async function expectTextVisible(page: Page, text: string) {
  await expect(page.getByText(text).first()).toBeVisible();
}

/**
 * Asserts that the current URL matches the expected path.
 */
export async function expectCurrentPath(page: Page, path: string) {
  const url = new URL(page.url());
  expect(url.pathname).toBe(path);
}

/**
 * Asserts that error message is visible (used for form validation).
 */
export async function expectErrorVisible(page: Page, errorText?: string) {
  if (errorText) {
    await expect(
      page.locator('.error-message, .form-error, [role="alert"]').filter({ hasText: errorText }),
    ).toBeVisible();
  } else {
    await expect(
      page.locator('.error-message, .form-error, [role="alert"]').first(),
    ).toBeVisible();
  }
}
