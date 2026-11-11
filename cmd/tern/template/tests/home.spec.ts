import { expect, test } from '@playwright/test';

test('the home page renders, and runs without errors', async ({ page }) => {
	const errors: string[] = [];
	page.on('pageerror', (e) => errors.push(e.message));
	page.on('console', (m) => {
		if (m.type() === 'error') errors.push(m.text());
	});
	const res = await page.goto('/');
	expect(res?.status()).toBe(200);
	await expect(page.locator('h1').first()).toBeVisible();
	await page.waitForLoadState('networkidle');
	expect(errors).toEqual([]);
});
