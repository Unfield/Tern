import AxeBuilder from '@axe-core/playwright';
import { expect, test, type Page } from '@playwright/test';

test.use({ reducedMotion: 'reduce' });

async function audit(page: Page) {
	const { violations } = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa', 'best-practice']).analyze();
	expect(violations.map((v) => `${v.id}: ${v.help} (${v.nodes.map((n) => n.target.join(' ')).join(', ')})`)).toEqual([]);
}

for (const path of ['/auth/signin', '/auth/signup', '/auth/consent']) {
	for (const theme of ['light', 'dark'] as const) {
		test(`${path} passes axe in ${theme} mode`, async ({ page }) => {
			await page.emulateMedia({ colorScheme: theme });
			await page.goto(path);
			if (theme === 'dark') await page.getByTestId('theme').click();
			await audit(page);
		});
	}
}

test('sign in checks the form and shows the server’s answer', async ({ page }) => {
	await page.goto('/auth/signin?error=access_denied');
	await expect(page.getByRole('alert')).toContainText('You cancelled the sign-in');
	await expect(page.getByRole('link', { name: 'Continue with GitHub' })).toHaveAttribute('href', '/auth/github/login');
	await page.getByRole('button', { name: 'Sign in' }).click();
	await expect(page.getByText('Email is required.')).toBeVisible();
	await audit(page);
	await page.getByRole('textbox', { name: 'Email' }).fill('ada@example.com');
	await page.locator('input[name=password]').fill('wrong');
	await page.getByRole('button', { name: 'Sign in' }).click();
	await expect(page.getByText('Wrong email or password.')).toBeVisible();
	await page.locator('input[name=password]').fill('correct horse');
	await page.getByRole('button', { name: 'Sign in' }).click();
	await expect(page).toHaveURL(/\/auth\/consent$/);
	await expect(page.getByRole('heading', { name: 'Team wiki wants to access your account' })).toBeVisible();
});

test('sign up takes its password hint from the Go rules', async ({ page }) => {
	await page.goto('/auth/signup');
	await expect(page.getByText('At least 12 characters.')).toBeVisible();
	await page.getByRole('button', { name: 'Create account' }).click();
	await expect(page.getByText('Agree to the terms to continue.')).toBeVisible();
	await expect(page.getByText('Name is required.')).toBeVisible();
});

test('the consent screen posts a decision with the request', async ({ page }) => {
	await page.goto('/auth/consent');
	const list = page.getByRole('list', { name: 'It will be able to:' });
	await expect(list.getByRole('listitem')).toHaveCount(3);
	const allow = page.getByRole('button', { name: 'Allow' });
	await expect(allow).toHaveAttribute('name', 'decision');
	await expect(allow).toHaveAttribute('value', 'allow');
	await expect(page.locator('input[type=hidden][name=client_id]')).toHaveValue('wiki');
});
