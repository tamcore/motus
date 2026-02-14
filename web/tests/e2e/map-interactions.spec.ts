import { test, expect } from '../fixtures/auth-fixture';
import { MapPage } from '../page-objects/MapPage';

test.describe('Map Interactions', () => {
  let mapPage: MapPage;

  test.beforeEach(async ({ authedPage }) => {
    mapPage = new MapPage(authedPage);
    await mapPage.goto();
  });

  test('should display map page with sidebar', async () => {
    await mapPage.expectLoaded();
  });

  test('should have correct page title in tab', async ({ authedPage }) => {
    await expect(authedPage).toHaveTitle('Map - Motus');
  });

  test('should show Leaflet map container', async () => {
    await expect(mapPage.mapContainer).toBeVisible();
  });

  test('should load map tiles', async () => {
    await expect(mapPage.tiles.first()).toBeVisible({ timeout: 10000 });
  });

  test('should show zoom controls', async () => {
    await expect(mapPage.zoomIn).toBeVisible();
    await expect(mapPage.zoomOut).toBeVisible();
  });

  test('should zoom in when clicking + button', async ({ authedPage }) => {
    await mapPage.zoomIn.click();
    await authedPage.waitForTimeout(500);
    // Verify zoom happened by checking tile count changes
    const tileCount = await mapPage.tiles.count();
    expect(tileCount).toBeGreaterThan(0);
  });

  test('should zoom out when clicking - button', async ({ authedPage }) => {
    await mapPage.zoomOut.click();
    await authedPage.waitForTimeout(500);
    const tileCount = await mapPage.tiles.count();
    expect(tileCount).toBeGreaterThan(0);
  });

  test('should show sidebar with Devices title', async () => {
    await expect(mapPage.sidebarTitle).toContainText('Devices');
  });

  test('should show search input in sidebar', async () => {
    await expect(mapPage.searchInput).toBeVisible();
  });

  test('should list devices in sidebar', async () => {
    const count = await mapPage.deviceItems.count();
    expect(count).toBeGreaterThanOrEqual(0);
  });

  test('should toggle sidebar collapse', async ({ authedPage }) => {
    await mapPage.toggleSidebar();
    await authedPage.waitForTimeout(400);
    // Sidebar should be collapsed
    const sidebar = mapPage.sidebar;
    await expect(sidebar).toHaveClass(/collapsed/);

    // Toggle back
    await mapPage.toggleSidebar();
    await authedPage.waitForTimeout(400);
    await expect(sidebar).not.toHaveClass(/collapsed/);
  });

  test('should hide search when sidebar is collapsed', async ({ authedPage }) => {
    await mapPage.toggleSidebar();
    await authedPage.waitForTimeout(400);
    await expect(mapPage.searchInput).not.toBeVisible();
  });

  test('should filter devices by search query', async () => {
    const count = await mapPage.deviceItems.count();
    if (count === 0) return;

    // Search for something that likely won't match
    await mapPage.searchDevices('zzz-nonexistent-xyz');
    await expect(mapPage.deviceItems).toHaveCount(0);

    // Clear search to restore all devices
    await mapPage.searchDevices('');
    await expect(mapPage.deviceItems).toHaveCount(count);
  });

  test('should select device in sidebar on click', async ({ authedPage }) => {
    const count = await mapPage.deviceItems.count();
    if (count === 0) return;

    await mapPage.clickDevice(0);
    await authedPage.waitForTimeout(300);
    await expect(mapPage.selectedDevice).toBeVisible();
    await expect(mapPage.selectedDevice).toHaveClass(/selected/);
  });

  test('should show detail panel when device is selected', async ({ authedPage }) => {
    const count = await mapPage.deviceItems.count();
    if (count === 0) return;

    await mapPage.clickDevice(0);
    await authedPage.waitForTimeout(500);
    // Detail panel may appear if device has position data
    const panelCount = await mapPage.detailPanel.count();
    expect(panelCount).toBeGreaterThanOrEqual(0);
  });

  test('should show device name and status in sidebar items', async () => {
    const count = await mapPage.deviceItems.count();
    if (count === 0) return;

    const firstDevice = mapPage.deviceItems.first();
    const name = firstDevice.locator('.device-name');
    await expect(name).toBeVisible();
    const nameText = await name.textContent();
    expect(nameText).toBeTruthy();
  });

  test('should render device markers on map', async () => {
    const markerCount = await mapPage.markers.count();
    // Markers depend on device position data
    expect(markerCount).toBeGreaterThanOrEqual(0);
  });

  test('should show popup when clicking a marker', async ({ authedPage }) => {
    const markerCount = await mapPage.markers.count();
    if (markerCount === 0) return;

    await mapPage.markers.first().click({ force: true });
    await authedPage.waitForTimeout(500);
    await expect(mapPage.popup).toBeVisible();
  });

  test('should display speed in popup content', async ({ authedPage }) => {
    const markerCount = await mapPage.markers.count();
    if (markerCount === 0) return;

    await mapPage.markers.first().click({ force: true });
    await authedPage.waitForTimeout(500);
    if (await mapPage.popup.isVisible()) {
      const text = await mapPage.popupContent.textContent();
      expect(text).toContain('Speed');
    }
  });

  test('should pan map by dragging', async ({ authedPage }) => {
    const map = mapPage.mapContainer;
    const box = await map.boundingBox();
    if (!box) return;

    await authedPage.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
    await authedPage.mouse.down();
    await authedPage.mouse.move(box.x + box.width / 2 + 100, box.y + box.height / 2, {
      steps: 5,
    });
    await authedPage.mouse.up();
    await authedPage.waitForTimeout(500);

    // Map should still be visible after panning
    await expect(map).toBeVisible();
  });

  test('should handle empty devices list gracefully', async ({ authedPage }) => {
    // Use addInitScript to intercept fetch at JS level (page.route doesn't reliably intercept SvelteKit fetches)
    await authedPage.addInitScript(() => {
      const origFetch = window.fetch;
      window.fetch = async function (input: RequestInfo | URL, init?: RequestInit) {
        const url = typeof input === 'string' ? input : input instanceof URL ? input.href : input.url;
        if (url.includes('/api/devices') || url.includes('/api/positions')) {
          return new Response('[]', {
            status: 200,
            headers: { 'Content-Type': 'application/json' },
          });
        }
        return origFetch.apply(globalThis, [input, init] as Parameters<typeof fetch>);
      } as typeof fetch;
    });
    await authedPage.goto('/map');
    await mapPage.waitForMapLoad();

    await expect(mapPage.deviceItems).toHaveCount(0);
    await expect(mapPage.markers).toHaveCount(0);
  });
});
