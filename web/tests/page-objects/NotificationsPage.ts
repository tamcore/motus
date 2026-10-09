import { type Page, expect } from '@playwright/test';

export class NotificationsPage {
  constructor(private page: Page) {}

  async goto() {
    await this.page.goto('/notifications');
    await this.page.waitForSelector('h1:has-text("Notification Rules")');
  }

  get title() {
    return this.page.locator('h1.page-title');
  }

  get createButton() {
    return this.page.locator('button:has-text("Create Rule")');
  }

  get ruleCards() {
    return this.page.locator('.rule-card');
  }

  get emptyState() {
    return this.page.locator('.empty-state');
  }

  get modal() {
    return this.page.locator('[role="dialog"]');
  }

  get modalTitle() {
    return this.page.locator('#modal-title');
  }

  get nameInput() {
    return this.page.locator('[role="dialog"] input[name="name"]');
  }

  get eventTypeCheckboxes() {
    return this.page.locator('[role="dialog"] .event-type-checkbox input[type="checkbox"]');
  }

  /** Check one or more event type checkboxes by value. */
  async selectEventTypes(...values: string[]) {
    for (const val of values) {
      await this.page.locator(`[role="dialog"] .event-type-checkbox input[value="${val}"]`).check();
    }
  }

  get webhookUrlInput() {
    return this.page.locator('[role="dialog"] input[name="webhookUrl"]');
  }

  get templateTextarea() {
    return this.page.locator('[role="dialog"] #template');
  }

  get addHeaderButton() {
    return this.page.locator('[role="dialog"] button:has-text("Add Header")');
  }

  get variableButtons() {
    return this.page.locator('[role="dialog"] .variable-btn');
  }

  get cancelButton() {
    return this.page.locator('[role="dialog"] button:has-text("Cancel")');
  }

  get submitButton() {
    return this.page.locator('[role="dialog"] button:has-text("Create")');
  }

  get updateButton() {
    return this.page.locator('[role="dialog"] button:has-text("Update")');
  }

  get channelSelect() {
    return this.page.locator('[role="dialog"] #channel');
  }

  get commandTypeSelect() {
    return this.page.locator('[role="dialog"] #command-type');
  }

  get frequencyInput() {
    return this.page.locator('[role="dialog"] input[name="frequency"]');
  }

  get geofenceFilter() {
    return this.page.locator('[role="dialog"] .geofence-filter');
  }

  /** Geofence filter checkbox for the geofence with the given name. */
  geofenceCheckbox(name: string) {
    return this.page
      .locator('[role="dialog"] .geofence-checkbox')
      .filter({ hasText: name })
      .locator('input[type="checkbox"]');
  }

  /** Device filter checkbox for the device with the given name. */
  deviceCheckbox(name: string) {
    return this.page
      .locator('[role="dialog"] .device-checkbox')
      .filter({ hasText: name })
      .locator('input[type="checkbox"]');
  }

  get formError() {
    return this.page.locator('[role="dialog"] .form-error');
  }

  /** Reporting interval preset buttons (incl. "Custom") of ReportingIntervalPicker. */
  get intervalPresets() {
    return this.page.locator('[role="dialog"] button.interval-preset');
  }

  /** Interval preset button with exactly the given label, e.g. "20 sec" or "Custom". */
  intervalPreset(label: string) {
    return this.intervalPresets.filter({ hasText: new RegExp(`^${label}$`) });
  }

  /** Reconnect-loop warning (reboot/custom on device online/offline). */
  get commandConflict() {
    return this.page.locator('[role="dialog"] .command-conflict');
  }

  /** Geofence filter checkboxes of unavailable (deleted/inaccessible) geofences. */
  get unavailableGeofences() {
    return this.page.locator('[role="dialog"] .geofence-unavailable');
  }

  get unavailableGeofenceHint() {
    return this.page.locator('[role="dialog"] .geofence-unavailable-hint');
  }

  /** Rule card whose name contains the given text. */
  ruleCard(name: string) {
    return this.ruleCards.filter({ has: this.page.locator('.rule-name', { hasText: name }) });
  }

  getRuleEditButton(index: number) {
    return this.ruleCards.nth(index).locator('button:has-text("Edit")');
  }

  getRuleDeleteButton(index: number) {
    return this.ruleCards.nth(index).locator('button:has-text("Delete")');
  }

  getRuleTestButton(index: number) {
    return this.ruleCards.nth(index).locator('button:has-text("Test")');
  }

  async expectLoaded() {
    await expect(this.title).toContainText('Notification Rules');
  }
}
