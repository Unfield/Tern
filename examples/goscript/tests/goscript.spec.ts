import { expect, test } from '@playwright/test';

test('the server renders the state Setup computed at build time', async ({ request }) => {
	const html = await (await request.get('/')).text();
	expect(html).toContain('data-testid="count">0</strong>');
	expect(html).toContain('data-testid="parity">even</span>');
	expect(html).toContain('5,000,000');
});

test('Go runes drive the markup in the browser', async ({ page }) => {
	const problems: string[] = [];
	page.on('console', (m) => m.type() === 'error' && problems.push(m.text()));
	page.on('pageerror', (e) => problems.push(e.message));
	await page.goto('/');
	const counter = page.getByTestId('counter');
	// Mounted with the start={10} prop.
	await expect(counter.getByTestId('count')).toHaveText('10');
	await expect(counter.getByTestId('doubled')).toHaveText('20');

	await counter.getByRole('button', { name: 'Add 1' }).click();
	await expect(counter.getByTestId('count')).toHaveText('11');
	await expect(counter.getByTestId('parity')).toHaveText('odd');

	// An input event reaches Go as a goscript.Event.
	await counter.getByTestId('step').fill('5');
	await expect(counter.getByRole('button', { name: 'Add 5' })).toBeVisible();
	await counter.getByRole('button', { name: 'Add 5' }).click();
	await expect(counter.getByTestId('count')).toHaveText('16');
	// An Effect keeps the last values.
	await expect(counter.getByTestId('history')).toHaveText('10, 11, 16');

	await counter.getByRole('button', { name: 'Reset' }).click();
	await expect(counter.getByTestId('count')).toHaveText('10');
	expect(problems).toEqual([]);
});

test('Go and TypeScript compute the same result', async ({ page }) => {
	await page.goto('/');
	await page.getByTestId('primes-go').getByRole('button').click();
	await page.getByTestId('primes-js').getByRole('button').click();
	await expect(page.getByTestId('primes-go').getByTestId('result')).toHaveText('348513');
	await expect(page.getByTestId('primes-js').getByTestId('result')).toHaveText('348513');
});
