import { expect, test, type Page } from '@playwright/test';

let n = 0;
const uniqueName = () => `user-${process.pid}-${Date.now()}-${n++}`;

async function signIn(page: Page, name = uniqueName()) {
	await page.goto('/');
	await expect(page).toHaveURL(/\/login$/);
	await page.getByLabel('Your name').fill(name);
	await page.getByRole('button', { name: 'Continue' }).click();
	await expect(page.getByTestId('user')).toHaveText(name);
	return name;
}

/** Marks the window so a full page load can be detected. */
async function mark(page: Page) {
	await page.evaluate(() => ((window as any).__tern_marker = true));
}

async function stillMarked(page: Page) {
	expect(await page.evaluate(() => (window as any).__tern_marker)).toBe(true);
}

test('signed-out users are sent to the login page', async ({ page }) => {
	await page.goto('/');
	await expect(page).toHaveURL(/\/login$/);
	await expect(page.getByRole('heading', { name: 'Sign in' })).toBeVisible();
});

test('login validation errors render in place', async ({ page }) => {
	await page.goto('/login');
	await mark(page);
	await page.getByLabel('Your name').fill('x'.repeat(41));
	await page.getByRole('button', { name: 'Continue' }).click();
	await expect(page.getByRole('alert')).toHaveText('Enter a name of up to 40 characters.');
	await stillMarked(page);
});

test('todos can be created, toggled and deleted without reloading', async ({ page }) => {
	await signIn(page);
	await mark(page);

	const input = page.getByLabel('New todo');
	await input.fill('Write docs');
	await page.getByRole('button', { name: 'Add' }).click();
	await expect(page.getByRole('link', { name: 'Write docs' })).toBeVisible();
	await expect(input).toHaveValue(''); // the form resets after success
	await expect(page.getByTestId('left')).toHaveText('1 left');

	await page.getByRole('button', { name: 'Mark as done' }).click();
	await expect(page.getByTestId('left')).toHaveText('0 left');

	await page.getByRole('button', { name: 'Delete Write docs' }).click();
	await expect(page.getByText('Nothing to do.')).toBeVisible();
	await stillMarked(page);
});

test('failed actions keep the input and show the error', async ({ page }) => {
	await signIn(page);
	await page.getByLabel('New todo').fill('   ');
	await page.getByRole('button', { name: 'Add' }).click();
	await expect(page.getByRole('alert')).toHaveText('A todo needs a title of up to 120 characters.');
});

test('links navigate on the client and history works', async ({ page }) => {
	await signIn(page);
	await page.getByLabel('New todo').fill('Plan trip');
	await page.getByRole('button', { name: 'Add' }).click();
	await mark(page);

	await page.getByRole('link', { name: 'Plan trip' }).click();
	await expect(page.getByTestId('detail-title')).toHaveText('Plan trip');
	await expect(page).toHaveTitle('Plan trip · Todos');
	await stillMarked(page);

	await page.goBack();
	await expect(page.getByRole('heading', { name: 'Your todos' })).toBeVisible();
	await page.goForward();
	await expect(page.getByTestId('detail-title')).toHaveText('Plan trip');
	await stillMarked(page);
});

test('unknown todos show the error page', async ({ page }) => {
	await signIn(page);
	const res = await page.goto('/todos/999999');
	expect(res?.status()).toBe(404);
	await expect(page.getByTestId('error-status')).toHaveText('404');
	await expect(page.getByTestId('error-message')).toHaveText('Not Found');
});

test('unknown URLs show the error page', async ({ page }) => {
	const res = await page.goto('/no/such/page');
	expect(res?.status()).toBe(404);
	await expect(page.getByTestId('error-status')).toHaveText('404');
});

test('signing out redirects to the login page', async ({ page }) => {
	await signIn(page);
	await page.getByRole('button', { name: 'Sign out' }).click();
	await expect(page).toHaveURL(/\/login$/);
	await page.goto('/');
	await expect(page).toHaveURL(/\/login$/);
});

test('the JSON endpoint returns the user\'s todos', async ({ page }) => {
	await signIn(page);
	await page.getByLabel('New todo').fill('From the API');
	await page.getByRole('button', { name: 'Add' }).click();
	await expect(page.getByRole('link', { name: 'From the API' })).toBeVisible();

	const res = await page.request.get('/api/todos');
	expect(res.status()).toBe(200);
	const todos = await res.json();
	expect(todos.map((t: { title: string }) => t.title)).toEqual(['From the API']);

	const anonymous = await page.request.get('/api/todos', { headers: { cookie: '' } });
	expect(anonymous.status()).toBe(401);
});

test('pages hydrate without warnings', async ({ page }) => {
	const problems: string[] = [];
	page.on('console', (msg) => {
		if (msg.type() === 'error' || msg.type() === 'warning') problems.push(msg.text());
	});
	page.on('pageerror', (err) => problems.push(err.message));

	await signIn(page);
	await page.getByLabel('New todo').fill('Hydrate me');
	await page.getByRole('button', { name: 'Add' }).click();
	await expect(page.getByRole('link', { name: 'Hydrate me' })).toBeVisible();
	await page.reload(); // server-rendered, then hydrated
	await page.getByRole('button', { name: 'Mark as done' }).click();
	await expect(page.getByTestId('left')).toHaveText('0 left');
	expect(problems).toEqual([]);
});

test('pages are rendered on the server', async ({ request }) => {
	const html = await (await request.get('/login')).text();
	expect(html).toContain('data-tern-ssr');
	expect(html).toContain('<h1>Sign in</h1>');
	expect(html).toContain('<title>Sign in · Todos</title>');
});

test.describe('without JavaScript', () => {
	test.use({ javaScriptEnabled: false });

	test('the app works with plain forms and links', async ({ page }) => {
		const name = uniqueName();
		await page.goto('/');
		await expect(page).toHaveURL(/\/login$/);
		await page.getByLabel('Your name').fill(name);
		await page.getByRole('button', { name: 'Continue' }).click();
		await expect(page.getByTestId('user')).toHaveText(name);

		await page.getByLabel('New todo').fill('No JS needed');
		await page.getByRole('button', { name: 'Add' }).click();
		await expect(page.getByRole('link', { name: 'No JS needed' })).toBeVisible();

		await page.getByLabel('New todo').fill('');
		await page.getByRole('button', { name: 'Add' }).click();
		await expect(page.getByRole('alert')).toHaveText('A todo needs a title of up to 120 characters.');

		await page.getByRole('link', { name: 'No JS needed' }).click();
		await expect(page.getByTestId('detail-title')).toHaveText('No JS needed');
	});
});
