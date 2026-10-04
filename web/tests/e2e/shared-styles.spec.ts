import type { Page } from '@playwright/test';
import { test, expect } from '../fixtures/auth-fixture';
import { mockFetch } from '../helpers/mock-fetch';

/** Resolves a CSS colour expression in the page, e.g. 'var(--bg-secondary)'. */
function resolveColor(page: Page, value: string): Promise<string> {
  return page.evaluate((v) => {
    const probe = document.createElement('div');
    probe.style.color = v;
    document.body.appendChild(probe);
    const color = getComputedStyle(probe).color;
    probe.remove();
    return color;
  }, value);
}

test.describe('Shared styles', () => {
  for (const theme of ['dark', 'light']) {
    test(`chat input follows the ${theme} theme tokens and fills the input row`, async ({ authedPage }) => {
      await authedPage.addInitScript((t) => localStorage.setItem('motus_theme', t), theme);
      await mockFetch(authedPage, [
        {
          match: '/api/server',
          body: {
            id: 1,
            registration: false,
            readonly: false,
            deviceReadonly: false,
            limitCommands: false,
            version: 'test',
            aiEnabled: true,
          },
        },
        { path: '/api/chat/history', body: { messages: [] } },
      ]);
      await authedPage.goto('/chat');
      const textarea = authedPage.locator('.input-area textarea');
      await textarea.waitFor({ state: 'visible', timeout: 10000 });
      await expect(authedPage.locator('html')).toHaveAttribute('data-theme', theme);

      const styles = await textarea.evaluate((el) => {
        const cs = getComputedStyle(el);
        return { bg: cs.backgroundColor, color: cs.color, border: cs.borderTopColor };
      });
      expect(styles.bg).toBe(await resolveColor(authedPage, 'var(--bg-secondary)'));
      expect(styles.color).toBe(await resolveColor(authedPage, 'var(--text-primary)'));
      expect(styles.border).toBe(await resolveColor(authedPage, 'var(--border-color)'));

      const row = await authedPage.locator('.input-area').boundingBox();
      const box = await textarea.boundingBox();
      const button = await authedPage.locator('.input-area button').boundingBox();
      expect(row && box && button).toBeTruthy();
      expect(Math.round(box!.x)).toBe(Math.round(row!.x));
      expect(box!.x + box!.width).toBeLessThan(button!.x);
      expect(Math.round(button!.x + button!.width)).toBe(Math.round(row!.x + row!.width));
    });
  }

  test('heatmap range sliders are native, accent-coloured and full width', async ({ authedPage }) => {
    await mockFetch(authedPage, [
      { match: '/api/devices', body: [] },
      { match: '/api/positions', body: [] },
    ]);
    await authedPage.goto('/heatmap');
    const accent = await resolveColor(authedPage, 'var(--accent-primary)');

    for (const id of ['#radius-slider', '#blur-slider', '#opacity-slider']) {
      const slider = authedPage.locator(id);
      await slider.waitFor({ state: 'visible', timeout: 10000 });
      const group = await slider.locator('xpath=..').boundingBox();
      const box = await slider.boundingBox();
      expect(group && box).toBeTruthy();
      expect(Math.round(box!.width)).toBe(Math.round(group!.width));
      expect(box!.height).toBeGreaterThanOrEqual(12);
      expect(await slider.evaluate((el) => getComputedStyle(el).accentColor)).toBe(accent);
    }
  });

  test('calendar cards fit a phone viewport and truncate long names', async ({ authedPage }) => {
    const calendar = {
      id: 9001,
      name: 'A very long calendar name that does not fit on one line of a phone screen',
      data: 'BEGIN:VCALENDAR\nBEGIN:VEVENT\nDTSTART:20260101T080000Z\nDTEND:20260101T170000Z\nEND:VEVENT\nEND:VCALENDAR',
      createdAt: '2026-01-01T00:00:00Z',
      updatedAt: '2026-01-01T00:00:00Z',
    };
    await mockFetch(authedPage, [
      { path: '/api/calendars', method: 'GET', body: [calendar] },
      { path: '/api/admin/calendars', method: 'GET', body: [calendar] },
    ]);
    await authedPage.setViewportSize({ width: 390, height: 844 });
    await authedPage.goto('/calendars');

    const card = authedPage.locator('.calendar-card').first();
    await card.waitFor({ state: 'visible', timeout: 10000 });
    const box = await card.boundingBox();
    expect(box).toBeTruthy();
    expect(box!.x).toBeGreaterThanOrEqual(0);
    expect(box!.x + box!.width).toBeLessThanOrEqual(390);

    const { scrollWidth, clientWidth } = await card.locator('.card-name').evaluate((el) => ({
      scrollWidth: el.scrollWidth,
      clientWidth: el.clientWidth,
    }));
    expect(scrollWidth).toBeGreaterThan(clientWidth);
    expect(await authedPage.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
  });

  test('items owned by another user get the warning highlight', async ({ authedPage }) => {
    const calendar = {
      id: 9002,
      name: 'Shared calendar',
      ownerName: 'Someone Else',
      data: 'BEGIN:VCALENDAR\nBEGIN:VEVENT\nDTSTART:20260101T080000Z\nDTEND:20260101T170000Z\nEND:VEVENT\nEND:VCALENDAR',
      createdAt: '2026-01-01T00:00:00Z',
      updatedAt: '2026-01-01T00:00:00Z',
    };
    await mockFetch(authedPage, [
      { path: '/api/calendars', method: 'GET', body: [calendar] },
      { path: '/api/admin/calendars', method: 'GET', body: [calendar] },
    ]);
    await authedPage.goto('/calendars');

    const card = authedPage.locator('.calendar-card.other-user').first();
    await card.waitFor({ state: 'visible', timeout: 10000 });
    const border = await card.evaluate((el) => {
      const cs = getComputedStyle(el);
      return { width: cs.borderLeftWidth, color: cs.borderLeftColor };
    });
    expect(border.width).toBe('3px');
    expect(border.color).toBe(await resolveColor(authedPage, 'var(--warning)'));
  });
});
