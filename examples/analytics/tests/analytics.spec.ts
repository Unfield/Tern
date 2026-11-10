import AxeBuilder from '@axe-core/playwright';
import { expect, test, type Page } from '@playwright/test';

test.use({ reducedMotion: 'reduce' });

// Test account, created by the first test on a fresh database.
const owner = { name: 'Ada Lovelace', email: 'ada@example.com', password: 'correct horse battery' };

async function audit(page: Page) {
	const { violations } = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa', 'best-practice']).analyze();
	expect(violations.map((v) => `${v.id}: ${v.help} (${v.nodes.map((n) => n.target.join(' ')).join(', ')})`)).toEqual([]);
}

async function signIn(page: Page) {
	await page.goto('/login');
	await page.getByRole('textbox', { name: 'Email' }).fill(owner.email);
	await page.locator('input[name=password]').fill(owner.password);
	await page.getByRole('button', { name: 'Sign in' }).click();
	await expect(page).toHaveURL(/\/sites$/);
}

test('a new install asks for an owner, checked by Go rules in the browser', async ({ page }) => {
	await page.goto('/');
	await expect(page).toHaveURL(/\/setup$/);
	await audit(page);
	await page.getByRole('textbox', { name: 'Name' }).fill(owner.name);
	await page.getByRole('textbox', { name: 'Email' }).fill(owner.email);
	await page.locator('input[name=password]').fill('short');
	await page.getByRole('button', { name: 'Create account' }).click();
	await expect(page.getByText('Password must be at least 12 characters.')).toBeVisible();
	await page.locator('input[name=password]').fill(owner.password);
	await page.getByRole('button', { name: 'Create account' }).click();
	await expect(page).toHaveURL(/\/sites$/);
	await expect(page.getByRole('heading', { name: 'Demo site' })).toBeVisible();
	// Setup is closed once there's an owner.
	await page.goto('/setup');
	await expect(page).toHaveURL(/\/sites$/);
});

for (const theme of ['light', 'dark'] as const) {
	test(`the dashboard passes axe in ${theme} mode`, async ({ page }) => {
		const problems: string[] = [];
		page.on('pageerror', (e) => problems.push(e.message));
		page.on('console', (m) => m.type() === 'error' && problems.push(m.text()));
		await page.emulateMedia({ colorScheme: theme });
		await signIn(page);
		await audit(page);
		await page.getByRole('link', { name: 'Open dashboard' }).click();
		await expect(page.getByRole('heading', { name: 'Demo site', level: 1 })).toBeVisible();
		await expect(page.getByTestId('live')).toContainText('right now');
		await audit(page);
		expect(problems).toEqual([]);
	});
}

test('page views from the script show up live, and periods switch', async ({ page, context }) => {
	await signIn(page);
	await page.goto('/sites/demo.example');
	const live = page.getByTestId('live');
	await expect(live).toContainText(/\d+ visitors? right now/);
	const before = Number((await live.textContent())!.match(/\d+/)![0]);
	// A visit to a page with the script, in another tab.
	const visitor = await context.browser()!.newContext({ userAgent: 'Mozilla/5.0 (iPhone; CPU iPhone OS 18_1 like Mac OS X) Version/18.1 Mobile/15E148 Safari/604.1' });
	const tab = await visitor.newPage();
	await tab.goto('/demo');
	await expect(live).toContainText(`${before + 1} visitor`);
	// Client-side navigation counts too, as the same visitor.
	await tab.getByRole('link', { name: 'About' }).click();
	await expect(tab.getByRole('heading', { name: 'About demo.example' })).toBeVisible();
	await page.reload();
	await expect(page.getByTestId('live')).toContainText(`${before + 1} visitor`);
	await visitor.close();
	// The period buttons reload the figures for 7 days.
	const week = page.getByRole('radio', { name: '7 days' });
	await week.click();
	await expect(page).toHaveURL(/days=7$/);
	await expect(week).toHaveAttribute('aria-checked', 'true');
	await page.getByRole('figure', { name: 'Traffic' }).getByRole('button', { name: 'Show data' }).click();
	await expect(page.getByRole('figure', { name: 'Traffic' }).getByRole('table').getByRole('row')).toHaveCount(8);
	// The top pages table includes the visit.
	await page.getByRole('searchbox', { name: 'Search Top pages' }).fill('demo');
	await expect(page.getByRole('table', { name: 'Top pages' }).getByRole('row', { name: /\/demo\/about/ })).toBeVisible();
});

test('adding a site checks the domain, then shows the script tag', async ({ page }) => {
	await signIn(page);
	await page.getByTestId('add-site').click();
	const dialog = page.getByRole('dialog', { name: 'Add a site' });
	await dialog.getByRole('textbox', { name: 'Domain' }).fill('https://acme.example');
	await dialog.getByRole('textbox', { name: 'Name' }).fill('Acme');
	await dialog.getByRole('button', { name: 'Add site' }).click();
	await expect(dialog.getByText('Enter a domain such as example.com, without https://.')).toBeVisible();
	await dialog.getByRole('textbox', { name: 'Domain' }).fill('demo.example');
	await dialog.getByRole('button', { name: 'Add site' }).click();
	await expect(page.getByRole('dialog', { name: 'Add a site' }).getByText('This site is already here.')).toBeVisible();
	await page.getByRole('dialog', { name: 'Add a site' }).getByRole('textbox', { name: 'Domain' }).fill('acme.example');
	await page.getByRole('dialog', { name: 'Add a site' }).getByRole('button', { name: 'Add site' }).click();
	await expect(page).toHaveURL(/\/sites\/acme\.example$/);
	await expect(page.getByRole('heading', { name: 'No page views yet' })).toBeVisible();
	await expect(page.locator('pre')).toContainText('data-domain="acme.example"');
	await audit(page);
	// Deleting asks first.
	await page.getByRole('button', { name: 'Delete this site' }).click();
	await page.getByRole('alertdialog', { name: 'Delete Acme?' }).getByRole('button', { name: 'Delete site' }).click();
	await expect(page).toHaveURL(/\/sites$/);
	await expect(page.getByRole('heading', { name: 'Acme' })).toHaveCount(0);
});

test('signing in and out, and the stats API', async ({ page, request }) => {
	const anonymous = await request.get('/api/sites/demo.example/stats');
	expect(anonymous.status()).toBe(401);
	await page.goto('/sites');
	await expect(page).toHaveURL(/\/login\?next=%2Fsites$/);
	await page.getByRole('textbox', { name: 'Email' }).fill(owner.email);
	await page.locator('input[name=password]').fill('wrong password');
	await page.getByRole('button', { name: 'Sign in' }).click();
	await expect(page.getByText('Wrong email or password.')).toBeVisible();
	await page.locator('input[name=password]').fill(owner.password);
	await page.getByRole('button', { name: 'Sign in' }).click();
	await expect(page).toHaveURL(/\/sites$/);
	const stats = await page.request.get('/api/sites/demo.example/stats?days=7');
	expect(stats.status()).toBe(200);
	const body = await stats.json();
	expect(body.days).toHaveLength(7);
	expect(body.summary.visitors).toBeGreaterThan(0);
	await page.getByRole('button', { name: 'Sign out' }).click();
	await expect(page).toHaveURL(/\/login$/);
});
