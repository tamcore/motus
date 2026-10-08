import type { Page } from '@playwright/test';
import { test, expect } from '../fixtures/auth-fixture';
import { NotificationsPage } from '../page-objects/NotificationsPage';

const RULE = {
  id: 9001,
  userId: 1,
  name: 'Log context rule',
  eventTypes: ['motion', 'geofenceEnter'],
  channel: 'webhook',
  config: { channel: 'webhook', webhookUrl: 'https://example.com/hook' },
  template: 'x',
  enabled: true,
  geofenceIds: [],
  createdAt: '2026-10-01T10:00:00Z',
  updatedAt: '2026-10-01T10:00:00Z',
};

const LOGS = [
  {
    id: 2,
    ruleId: RULE.id,
    eventId: 21,
    status: 'sent',
    responseCode: 200,
    createdAt: '2026-10-08T12:31:00Z',
    eventType: 'motion',
    eventTime: '2026-10-08T12:30:58Z',
    eventAttributes: { speed: 42, previousSpeed: 0 },
    deviceId: 5,
    deviceName: 'Family Car',
  },
  {
    id: 1,
    ruleId: RULE.id,
    eventId: 20,
    status: 'sent',
    responseCode: 200,
    createdAt: '2026-10-08T12:00:00Z',
    eventType: 'geofenceEnter',
    eventTime: '2026-10-08T11:59:59Z',
    deviceId: 5,
    deviceName: 'Family Car',
    geofenceName: 'Home',
  },
];

async function mockNotificationApi(page: Page) {
  await page.addInitScript(
    ({ rule, logs }) => {
      const origFetch = window.fetch;
      window.fetch = async function (input: RequestInfo | URL, init?: RequestInit) {
        const url = typeof input === 'string' ? input : input instanceof URL ? input.href : input.url;
        const path = new URL(url, window.location.origin).pathname;
        const json = (body: unknown) =>
          new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } });
        if (path === '/api/notifications' || path === '/api/admin/notifications') return json([rule]);
        if (path === `/api/notifications/${rule.id}/logs`) return json(logs);
        return origFetch.apply(globalThis, [input, init] as Parameters<typeof fetch>);
      } as typeof fetch;
    },
    { rule: RULE, logs: LOGS },
  );
}

test.describe('Notification delivery logs', () => {
  test('rule Logs link shows triggering event and changes', async ({ authedPage }) => {
    await mockNotificationApi(authedPage);
    const notifPage = new NotificationsPage(authedPage);
    await notifPage.goto();

    await notifPage.ruleCards.filter({ hasText: RULE.name }).locator('a.logs-link').click();
    await expect(authedPage).toHaveURL(new RegExp(`/notifications/history\\?rule=${RULE.id}`));

    const table = authedPage.locator('table.history-table');
    const rows = table.locator('tbody tr');
    await expect(rows).toHaveCount(2);
    await expect(table).not.toContainText('Invalid Date');

    const motion = rows.filter({ hasText: 'Motion Started' });
    await expect(motion.locator('.cell-event')).toContainText('Family Car · Motion Started');
    await expect(motion.locator('.log-changes')).toContainText('Speed:');
    await expect(motion.locator('.change-from')).toContainText('0.0');
    await expect(motion.locator('.change-to')).toContainText('42.0');

    const geofence = rows.filter({ hasText: 'Geofence Enter' });
    await expect(geofence.locator('.cell-event')).toContainText('Family Car · Geofence Enter · Home');
    await expect(geofence.locator('.change-from')).toContainText('outside');
    await expect(geofence.locator('.change-to')).toContainText('inside');
  });
});
