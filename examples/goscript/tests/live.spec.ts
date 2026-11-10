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

test('live todos: bindings, pending, errors, optimistic updates, patches and events', async ({ browser }) => {
	const one = await (await browser.newContext()).newPage();
	const two = await (await browser.newContext()).newPage();
	const problems: string[] = [];
	const frames: string[] = [];
	for (const p of [one, two]) {
		p.on('console', (m) => m.type() === 'error' && problems.push(m.text()));
		p.on('pageerror', (e) => problems.push(e.message));
	}
	one.on('websocket', (ws) => ws.on('framereceived', (f) => frames.push(String(f.payload))));
	await one.goto('/');
	await two.goto('/');
	const a = one.getByTestId('todos');
	const b = two.getByTestId('todos');
	await expect(a.getByRole('button', { name: 'Add' })).toBeDisabled();

	// The input is bound to Go state; the button enables when Go agrees.
	await a.getByTestId('draft').fill('write the docs');
	await expect(a.getByRole('button', { name: 'Add' })).toBeEnabled();
	await a.getByRole('button', { name: 'Add' }).click();
	// The Func takes a moment: its pending state shows.
	await expect(a.getByRole('button', { name: 'Adding…' })).toBeVisible();
	await expect(a.getByText('write the docs')).toBeVisible();
	await expect(a.getByTestId('draft')).toHaveValue('');
	// The event reached the parent page through the onadded prop.
	await expect(one.getByTestId('last-added')).toHaveText('Last added here: write the docs');
	// The other visitor sees it, but not the event.
	await expect(b.getByText('write the docs')).toBeVisible();
	await expect(two.getByTestId('last-added')).toHaveCount(0);

	// A Fail error is shown; nothing is added.
	await a.getByTestId('draft').fill('x'.repeat(61));
	await a.getByRole('button', { name: 'Add' }).click();
	await expect(a.getByTestId('error')).toHaveText('Keep it under 60 characters.');

	// A second item goes to the other browser as a patch, not the whole list.
	await b.getByTestId('draft').fill('ship it');
	await b.getByRole('button', { name: 'Add' }).click();
	await expect(a.getByText('ship it')).toBeVisible();
	expect(frames.some((f) => f.includes('"patch"') && f.includes('ship it'))).toBe(true);

	// Toggling is optimistic, then confirmed by the server for everyone.
	const remaining = Number(await a.getByTestId('remaining').textContent());
	await a.getByRole('checkbox').first().check();
	await expect(a.getByTestId('remaining')).toHaveText(String(remaining - 1));
	await expect(b.getByTestId('remaining')).toHaveText(String(remaining - 1));
	await b.getByRole('button', { name: 'Clear done' }).click();
	await expect(a.getByText('write the docs')).toHaveCount(0);

	// The server pushes the clock.
	const t0 = await a.getByTestId('now').textContent();
	await expect(a.getByTestId('now')).not.toHaveText(t0!, { timeout: 3000 });
	expect(problems).toEqual([]);
});

test('a reconnect remounts on the server and keeps bound values', async ({ page }) => {
	// Keep a handle on the page's WebSockets, to drop them like a deploy would.
	await page.addInitScript(() => {
		const Native = window.WebSocket;
		const sockets: WebSocket[] = [];
		(window as unknown as { __sockets: WebSocket[] }).__sockets = sockets;
		window.WebSocket = class extends Native {
			constructor(url: string | URL, protocols?: string | string[]) {
				super(url, protocols);
				sockets.push(this);
			}
		};
	});
	await page.goto('/');
	const todos = page.getByTestId('todos');
	await todos.getByTestId('draft').fill('half typed');
	await expect(todos.getByRole('button', { name: 'Add' })).toBeEnabled();
	await page.evaluate(() => (window as unknown as { __sockets: WebSocket[] }).__sockets.forEach((s) => s.close()));
	await expect(todos.getByTestId('offline')).toBeVisible();
	await expect(todos.getByTestId('offline')).toHaveCount(0);
	expect(await page.evaluate(() => (window as unknown as { __sockets: WebSocket[] }).__sockets.length)).toBe(2);
	// The new server instance got the draft back: Go computes canAdd from it.
	await expect(todos.getByTestId('draft')).toHaveValue('half typed');
	await expect(todos.getByRole('button', { name: 'Add' })).toBeEnabled();
	await todos.getByRole('button', { name: 'Add' }).click();
	await expect(todos.getByText('half typed')).toBeVisible();
});
