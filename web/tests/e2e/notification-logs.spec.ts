import type { Page } from '@playwright/test';
import { test, expect } from '../fixtures/auth-fixture';
import { mockFetch } from '../helpers/mock-fetch';
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
    id: 3,
    ruleId: RULE.id,
    eventId: 21,
    status: 'sent',
    responseCode: 200,
    sentAt: '2026-10-08T12:31:00Z',
    createdAt: '2026-10-08T12:31:00Z',
    eventType: 'motion',
    eventTime: '2026-10-08T12:30:58Z',
    eventAttributes: { type: 'motion', speed: 42, previousSpeed: 0 },
    deviceId: 5,
    deviceName: 'Family Car',
  },
  {
    id: 2,
    ruleId: RULE.id,
    eventId: 20,
    status: 'sent',
    responseCode: 200,
    sentAt: '2026-10-08T12:00:00Z',
    createdAt: '2026-10-08T12:00:00Z',
    eventType: 'geofenceEnter',
    eventTime: '2026-10-08T11:59:59Z',
    eventAttributes: { type: 'geofenceEnter' },
    deviceId: 5,
    deviceName: 'Family Car',
    geofenceName: 'Home',
  },
  // The triggering event was deleted (event_id ON DELETE SET NULL).
  {
    id: 1,
    ruleId: RULE.id,
    eventId: null,
    status: 'sent',
    responseCode: 200,
    sentAt: '2026-10-07T09:00:00Z',
    createdAt: '2026-10-07T09:00:00Z',
    eventTime: null,
    deviceId: null,
  },
];

async function mockNotificationApi(page: Page) {
  await mockFetch(page, [
    { path: '/api/notifications', body: [RULE] },
    { path: '/api/admin/notifications', body: [RULE] },
    { path: `/api/notifications/${RULE.id}/logs`, body: LOGS },
  ]);
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
    await expect(rows).toHaveCount(3);
    await expect(table).not.toContainText('Invalid Date');
    await expect(rows.first().locator('.cell-time')).toContainText('2026-10-08');

    const motion = rows.filter({ hasText: 'Motion Started' });
    await expect(motion.locator('.cell-event')).toContainText('Family Car · Motion Started');
    await expect(motion.locator('.log-changes')).toContainText('Speed:');
    await expect(motion.locator('.log-changes')).not.toContainText('motion');
    await expect(motion.locator('.change-from')).toContainText('0.0');
    await expect(motion.locator('.change-to')).toContainText('42.0');

    const geofence = rows.filter({ hasText: 'Geofence Enter' });
    await expect(geofence.locator('.cell-event')).toContainText('Family Car · Geofence Enter · Home');
    await expect(geofence.locator('.change-from')).toContainText('outside');
    await expect(geofence.locator('.change-to')).toContainText('inside');

    await expect(rows.last().locator('.cell-event')).toHaveText('Triggering event no longer available');
  });
});
