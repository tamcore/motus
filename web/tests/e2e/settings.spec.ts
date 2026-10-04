import type { Locator } from '@playwright/test';
import { test, expect } from '../fixtures/auth-fixture';
import { SettingsPage } from '../page-objects/SettingsPage';

async function box(locator: Locator) {
  const b = await locator.boundingBox();
  expect(b).toBeTruthy();
  return b!;
}

// Offset of a child's left edge inside a section (border + padding).
async function inset(section: Locator, child: Locator) {
  return (await box(child)).x - (await box(section)).x;
}

test.describe('Settings page layout', () => {
  let settings: SettingsPage;

  test.beforeEach(async ({ authedPage }) => {
    settings = new SettingsPage(authedPage);
    await settings.goto();
    await expect(settings.sessionCards.first()).toBeVisible();
  });

  test('centers the 800px container', async ({ authedPage }) => {
    const c = await box(settings.container);
    const vw = authedPage.viewportSize()!.width;
    expect(c.width).toBeLessThanOrEqual(800);
    expect(Math.abs(c.x + c.width / 2 - vw / 2)).toBeLessThan(10);
  });

  test('gives page and manager sections the same padding and spacing', async ({ authedPage }) => {
    const profile = settings.section('Profile');
    const apiKeys = settings.apiKeysSection;
    const profileInset = await inset(profile, profile.locator('h2.section-title'));
    const apiInset = await inset(apiKeys, apiKeys.locator('h2.section-title'));
    expect(profileInset).toBeGreaterThan(20);
    expect(Math.abs(apiInset - profileInset)).toBeLessThan(1);

    const form = await box(authedPage.locator('.settings-form'));
    const api = await box(apiKeys);
    expect(Math.abs(api.y - (form.y + form.height) - 24)).toBeLessThan(1);

    const sessions = await box(settings.sessionsSection);
    expect(sessions.width).toBeCloseTo(api.width, 0);
    expect(sessions.x).toBeCloseTo(api.x, 0);
  });

  test('puts the section action button at the right edge of the header', async () => {
    const section = await box(settings.apiKeysSection);
    const title = await box(settings.apiKeysSection.locator('h2.section-title'));
    const button = await box(settings.createApiKeyButton);
    const left = title.x - section.x;
    const right = section.x + section.width - (button.x + button.width);
    expect(Math.abs(right - left)).toBeLessThan(1);
    expect(button.y).toBeLessThan(title.y + title.height);
  });

  test('lays out session cards as rows spanning the section', async () => {
    const section = settings.sessionsSection;
    const card = settings.sessionCards.first();
    const cardInset = await inset(section, card);
    const titleInset = await inset(section, section.locator('h2.section-title'));
    expect(Math.abs(cardInset - titleInset)).toBeLessThan(1);

    const info = await box(card.locator('.session-info'));
    const actions = await box(card.locator('.session-actions'));
    expect(actions.x).toBeGreaterThanOrEqual(info.x + info.width - 1);
  });

  test('centers the create API key modal and right-aligns its actions', async ({ authedPage }) => {
    await settings.createApiKeyButton.click();
    await expect(settings.dialog).toBeVisible();

    const dialog = await box(settings.dialog);
    const vw = authedPage.viewportSize()!.width;
    expect(Math.abs(dialog.x + dialog.width / 2 - vw / 2)).toBeLessThan(2);

    const cancel = await box(settings.dialog.locator('.modal-actions button:has-text("Cancel")'));
    const create = await box(settings.dialog.locator('.modal-actions button:has-text("Create Key")'));
    expect(create.x).toBeGreaterThan(cancel.x + cancel.width);
    expect(Math.abs(create.y - cancel.y)).toBeLessThan(1);
    const rightGap = dialog.x + dialog.width - (create.x + create.width);
    expect(rightGap).toBeLessThan(40);

    const name = await box(settings.dialog.locator('input[name="keyName"]'));
    const permissions = await box(settings.dialog.locator('#keyPermissions'));
    expect(permissions.y).toBeGreaterThan(name.y + name.height);
    expect(Math.abs(permissions.width - name.width)).toBeLessThan(1);
  });

  test('stacks section headers and session cards on mobile', async ({ authedPage }) => {
    await authedPage.setViewportSize({ width: 390, height: 844 });
    const title = await box(settings.apiKeysSection.locator('h2.section-title'));
    const button = await box(settings.createApiKeyButton);
    expect(button.y).toBeGreaterThanOrEqual(title.y + title.height);

    const card = settings.sessionCards.first();
    const info = await box(card.locator('.session-info'));
    const actions = await box(card.locator('.session-actions'));
    expect(actions.y).toBeGreaterThanOrEqual(info.y + info.height - 1);
  });
});
