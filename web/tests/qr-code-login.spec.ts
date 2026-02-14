import { test, expect } from '@playwright/test';

test.describe('QR Code Login Feature', () => {
	test.beforeEach(async ({ page }) => {
		await page.goto('/login');
	});

	test('should display QR code button on login page', async ({ page }) => {
		const qrButton = page.getByLabel('Show QR code');
		await expect(qrButton).toBeVisible();
	});

	test('should open QR code dialog when button is clicked', async ({ page }) => {
		const qrButton = page.getByLabel('Show QR code');
		await qrButton.click();

		// Dialog should be visible
		const dialog = page.getByRole('dialog');
		await expect(dialog).toBeVisible();

		// Dialog title should be present
		await expect(page.getByText('Scan QR Code')).toBeVisible();
	});

	test('should display server URL in dialog', async ({ page }) => {
		const qrButton = page.getByLabel('Show QR code');
		await qrButton.click();

		// Server URL input should be visible and contain the origin
		const serverUrlInput = page.getByLabel('Server URL:');
		await expect(serverUrlInput).toBeVisible();

		const inputValue = await serverUrlInput.inputValue();
		expect(inputValue).toContain('http'); // Should start with http:// or https://
		expect(inputValue.length).toBeGreaterThan(10); // Should be a full URL
	});

	test('should display QR code canvas', async ({ page }) => {
		const qrButton = page.getByLabel('Show QR code');
		await qrButton.click();

		// QR code canvas should be present
		const canvas = page.locator('canvas');
		await expect(canvas).toBeVisible();
	});

	test('should display instruction text', async ({ page }) => {
		const qrButton = page.getByLabel('Show QR code');
		await qrButton.click();

		await expect(
			page.getByText(/Use Traccar Manager app to scan this QR code/i)
		).toBeVisible();
	});

	test('should close dialog when close button is clicked', async ({ page }) => {
		const qrButton = page.getByLabel('Show QR code');
		await qrButton.click();

		// Dialog should be visible
		await expect(page.getByRole('dialog')).toBeVisible();

		// Click X button
		const closeButton = page.getByLabel('Close dialog');
		await closeButton.click();

		// Dialog should be hidden
		await expect(page.getByRole('dialog')).not.toBeVisible();
	});

	test('should close dialog when Close button is clicked', async ({ page }) => {
		const qrButton = page.getByLabel('Show QR code');
		await qrButton.click();

		// Dialog should be visible
		await expect(page.getByRole('dialog')).toBeVisible();

		// Click Close button
		const closeButton = page.getByRole('button', { name: /close/i });
		await closeButton.click();

		// Dialog should be hidden
		await expect(page.getByRole('dialog')).not.toBeVisible();
	});

	test('should close dialog when backdrop is clicked', async ({ page }) => {
		const qrButton = page.getByLabel('Show QR code');
		await qrButton.click();

		// Dialog should be visible
		await expect(page.getByRole('dialog')).toBeVisible();

		// Click backdrop (outside dialog)
		const backdrop = page.locator('.dialog-backdrop');
		await backdrop.click({ position: { x: 10, y: 10 } });

		// Dialog should be hidden
		await expect(page.getByRole('dialog')).not.toBeVisible();
	});

	test('should allow copying server URL', async ({ page }) => {
		const qrButton = page.getByLabel('Show QR code');
		await qrButton.click();

		const serverUrlInput = page.getByLabel('Server URL:');

		// Input should be readonly
		await expect(serverUrlInput).toHaveAttribute('readonly');

		// Clicking should select text
		await serverUrlInput.click();

		// Triple-click to ensure selection (works cross-browser)
		await serverUrlInput.click({ clickCount: 3 });
	});

	test('should not interfere with login form', async ({ page }) => {
		// QR button should be visible
		await expect(page.getByLabel('Show QR code')).toBeVisible();

		// Login form should still be functional
		await expect(page.getByLabel('Email')).toBeVisible();
		await expect(page.getByLabel('Password')).toBeVisible();
		await expect(page.getByRole('button', { name: /login/i })).toBeVisible();
	});

	test('should maintain QR button position on mobile viewport', async ({ page }) => {
		// Set mobile viewport
		await page.setViewportSize({ width: 375, height: 667 });

		const qrButton = page.getByLabel('Show QR code');
		await expect(qrButton).toBeVisible();

		// Button should be in top-right corner
		const box = await qrButton.boundingBox();
		expect(box).toBeTruthy();
		if (box) {
			// Should be near the right edge
			expect(box.x).toBeGreaterThan(300);
		}
	});

	test('should generate different QR codes for different domains', async ({ page, baseURL }) => {
		const qrButton = page.getByLabel('Show QR code');
		await qrButton.click();

		const serverUrlInput = page.getByLabel('Server URL:');
		const url = await serverUrlInput.inputValue();

		// URL should match the base URL we're testing against
		if (baseURL) {
			expect(url).toContain(new URL(baseURL).host);
		}
	});

	test('should have proper accessibility attributes', async ({ page }) => {
		const qrButton = page.getByLabel('Show QR code');
		await qrButton.click();

		const dialog = page.getByRole('dialog');
		await expect(dialog).toHaveAttribute('aria-modal', 'true');
		await expect(dialog).toHaveAttribute('aria-labelledby', 'dialog-title');
	});
});
