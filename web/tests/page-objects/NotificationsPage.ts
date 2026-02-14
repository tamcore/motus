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

  getRuleEditButton(index: number) {
    return this.ruleCards.nth(index).locator('button:has-text("Edit")');
  }

  getRuleDeleteButton(index: number) {
    return this.ruleCards.nth(index).locator('button:has-text("Delete")');
  }

  getRuleTestButton(index: number) {
    return this.ruleCards.nth(index).locator('button:has-text("Test")');
  }

  getRuleToggle(index: number) {
    return this.ruleCards.nth(index).locator('.toggle-switch input');
  }

  async expectLoaded() {
    await expect(this.title).toContainText('Notification Rules');
  }
}
