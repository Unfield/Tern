import { expect, test } from '@playwright/test';

test('live components share state across browsers', async ({ browser }) => {
	const one = await (await browser.newContext()).newPage();
	const two = await (await browser.newContext()).newPage();
	const problems: string[] = [];
	for (const p of [one, two]) {
		p.on('console', (m) => (m.type() === 'error' || /hydrat/i.test(m.text())) && problems.push(m.text()));
		p.on('pageerror', (e) => problems.push(e.message));
	}
	await one.goto('/');
	await expect(one.getByTestId('live').getByTestId('online')).toHaveText(/^[1-9]/);
	const before = Number(await one.getByTestId('live').getByTestId('online').textContent());
	await two.goto('/');
	// Both see the second visitor.
	await expect(two.getByTestId('live').getByTestId('online')).toHaveText(String(before + 1));
	await expect(one.getByTestId('live').getByTestId('online')).toHaveText(String(before + 1));

	const clicks = Number(await one.getByTestId('live').getByTestId('clicks').textContent());
	await one.getByTestId('live').getByRole('button', { name: 'Click' }).click();
	await one.getByTestId('live').getByRole('button', { name: 'Click' }).click();
	await two.getByTestId('live').getByRole('button', { name: 'Click' }).click();
	await expect(one.getByTestId('live').getByTestId('clicks')).toHaveText(String(clicks + 3));
	await expect(two.getByTestId('live').getByTestId('clicks')).toHaveText(String(clicks + 3));
	// Per-instance state stays per instance.
	await expect(two.getByTestId('live').getByTestId('share')).toContainText('of all clicks are yours');

	await two.close();
	await expect(one.getByTestId('live').getByTestId('online')).toHaveText(String(before));
	expect(problems).toEqual([]);
});

test('live components render on the server with their props', async ({ request }) => {
	const html = await (await request.get('/')).text();
	expect(html).toContain('data-testid="hello">Hello, visitor');
	expect(html).toMatch(/<script type="application\/json" id="tern-live-[^"]+">\{.*"hello":"Hello, visitor"/);
});
