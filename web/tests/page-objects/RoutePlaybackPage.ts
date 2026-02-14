import { type Page, expect } from '@playwright/test';

export class RoutePlaybackPage {
  constructor(private page: Page) {}

  async goto(deviceId: number, from: string, to: string) {
    const url = `/reports/route?deviceId=${deviceId}&from=${from}&to=${to}`;
    await this.page.goto(url);
    await this.page.waitForSelector('.route-page');
  }

  get routeMap() {
    return this.page.locator('.route-map');
  }

  get playButton() {
    return this.page.locator('button:has-text("Play")');
  }

  get pauseButton() {
    return this.page.locator('button:has-text("Pause")');
  }

  get stopButton() {
    return this.page.locator('button:has-text("Stop")');
  }

  get gpxButton() {
    return this.page.locator('button:has-text("GPX")');
  }

  get timelineSlider() {
    return this.page.locator('.timeline-slider');
  }

  get speedButtons() {
    return this.page.locator('.speed-btn');
  }

  get infoPanel() {
    return this.page.locator('.info-panel');
  }

  get controlsContainer() {
    return this.page.locator('.controls-container');
  }

  get timelineLabels() {
    return this.page.locator('.timeline-labels');
  }

  get loadingMessage() {
    return this.page.locator('.map-loading');
  }

  async expectLoaded() {
    await expect(this.routeMap).toBeVisible();
  }
}
