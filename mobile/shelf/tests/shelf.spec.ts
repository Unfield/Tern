import { expect, test } from '@playwright/test';

test('sign up, save a link, read it and delete it', async ({ page }) => {
	const errors: string[] = [];
	page.on('pageerror', (e) => errors.push(e.message));

	await page.goto('/');
	await expect(page).toHaveURL(/\/login\?next=%2F$/);

	await page.goto('/signup');
	await page.getByLabel('Name').fill('Ada');
	await page.getByLabel('Email').fill(`ada-${Date.now()}@example.com`);
	await page.locator('input[name=password]').fill('a long enough password');
	await page.getByRole('button', { name: 'Create account' }).click();
	await expect(page.getByRole('heading', { name: 'Your shelf' })).toBeVisible();

	// The browser checks the rules from the Go struct before posting.
	await page.getByLabel('Address').fill('not an address');
	await page.getByRole('button', { name: 'Save' }).click();
	await expect(page.getByText('Address must be a web address')).toBeVisible();

	await page.getByLabel('Address').fill('https://go.dev/blog/go1.26');
	await page.getByLabel('Title').fill('Go 1.26 is released');
	await page.getByRole('button', { name: 'Save' }).click();
	const link = page.getByRole('link', { name: 'Go 1.26 is released' });
	await expect(link).toBeVisible();
	await expect(page.getByLabel('Address')).toHaveValue('');

	await link.click();
	await page.getByRole('button', { name: 'Mark as read' }).click();
	await expect(page.getByRole('button', { name: 'Mark as unread' })).toBeVisible();

	await page.getByRole('link', { name: '← Your shelf' }).click();
	await expect(page.getByText('Nothing here. Save a link above.')).toBeVisible();
	await page.getByRole('link', { name: 'Read', exact: true }).click();
	await expect(page).toHaveURL(/\?show=read$/);
	await page.getByRole('link', { name: 'Go 1.26 is released' }).click();
	await page.getByRole('button', { name: 'Delete' }).click();
	await expect(page.getByRole('heading', { name: 'Your shelf' })).toBeVisible();

	expect(errors).toEqual([]);
});
