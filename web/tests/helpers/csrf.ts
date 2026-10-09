import type { Page } from '@playwright/test';

/** CSRF token for state-changing page.request calls (from the session endpoint). */
export async function csrfToken(page: Page): Promise<string> {
  const res = await page.request.get('/api/session');
  return res.headers()['x-csrf-token'] ?? '';
}
