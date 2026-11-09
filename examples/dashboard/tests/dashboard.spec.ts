import { expect, test, type Page } from '@playwright/test';

let n = 0;
const uniqueName = () => `User ${process.pid}-${Date.now()}-${n++}`;

/** Collects console errors, page errors and hydration warnings. */
function watchConsole(page: Page) {
	const problems: string[] = [];
	page.on('console', (msg) => {
		if (msg.type() === 'error' || (msg.type() === 'warning' && /hydrat/i.test(msg.text()))) problems.push(msg.text());
	});
	page.on('pageerror', (err) => problems.push(err.message));
	return problems;
}

async function signIn(page: Page, name = uniqueName()) {
	await page.goto('/login');
	await page.getByLabel('Name').fill(name);
	await page.getByRole('button', { name: 'Continue' }).click();
	await expect(page.getByRole('heading', { name: `Welcome back, ${name}` })).toBeVisible();
	return name;
}

async function mark(page: Page) {
	await page.evaluate(() => ((window as any).__marker = true));
}

async function expectSameWindow(page: Page) {
	expect(await page.evaluate(() => (window as any).__marker)).toBe(true);
}

test('the overview renders stats, a chart and streamed activity', async ({ page }) => {
	const problems = watchConsole(page);
	await signIn(page);
	await expect(page.getByTestId('stat-projects')).toHaveText('3');
	await expect(page.getByTestId('stat-open')).toHaveText('5');
	await expect(page.getByTestId('chart').locator('svg').first()).toBeVisible();
	await expect(page.getByTestId('activity').locator('li')).toHaveCount(6);

	// A full load renders on the server, streams the activity, and hydrates cleanly.
	await page.reload();
	await expect(page.getByTestId('activity').locator('li')).toHaveCount(6);
	expect(problems).toEqual([]);
});

test('pages are server-rendered with shadcn-svelte components', async ({ page, request }) => {
	await signIn(page);
	const html = await page.evaluate(() => fetch('/projects').then((r) => r.text()));
	expect(html).toContain('data-tern-ssr');
	expect(html).toContain('Website redesign');
	expect(html).toContain('data-slot="table"');

	const login = await (await request.get('/login')).text();
	expect(login).toContain('data-slot="card"');
});

test('the sidebar navigates on the client and collapses', async ({ page }) => {
	const problems = watchConsole(page);
	await signIn(page);
	await mark(page);

	await page.getByRole('link', { name: 'Projects', exact: true }).click();
	await expect(page.getByRole('heading', { name: 'Projects' })).toBeVisible();
	await expect(page.getByTestId('section')).toHaveText('Projects');
	await page.getByTestId('sidebar-projects').getByRole('link', { name: 'Mobile app' }).click();
	await expect(page.getByTestId('project-name')).toHaveText('Mobile app');
	await page.getByRole('link', { name: 'Settings' }).click();
	await expect(page.getByRole('heading', { name: 'Settings' })).toBeVisible();
	await expectSameWindow(page);

	await page.getByRole('button', { name: 'Toggle Sidebar' }).first().click();
	await expect(page.locator('[data-slot="sidebar"][data-state="collapsed"]')).toHaveCount(1);
	expect(problems).toEqual([]);
});

test('searching and filtering projects updates the URL without reloading', async ({ page }) => {
	await signIn(page);
	await page.goto('/projects');
	await mark(page);

	const search = page.getByLabel('Search projects');
	await search.fill('mobile');
	await search.press('Enter');
	await expect(page).toHaveURL(/\/projects\?q=mobile$/);
	await expect(page.getByTestId('projects').locator('tr')).toHaveCount(1);
	await expect(search).toBeFocused();

	await search.fill('');
	await search.press('Enter');
	await page.getByRole('tab', { name: 'Paused' }).click();
	await expect(page).toHaveURL(/status=paused/);
	await expect(page.getByTestId('projects')).toContainText('Q3 planning');
	await expect(page.getByTestId('projects').locator('tr')).toHaveCount(1);
	await expectSameWindow(page);
});

test('the new project dialog validates, creates and shows a toast', async ({ page }) => {
	await signIn(page);
	await page.goto('/projects');

	await page.getByRole('button', { name: 'New project' }).click();
	const dialog = page.getByRole('dialog');
	await expect(dialog).toBeVisible();
	await dialog.getByRole('button', { name: 'Create project' }).click();
	await expect(dialog.getByRole('alert')).toHaveText('Give the project a name.');

	await dialog.getByLabel('Name').fill('Launch event');
	await dialog.getByLabel('Description').fill('Plan the launch party.');
	await dialog.getByRole('button', { name: 'Create project' }).click();
	await expect(dialog).toBeHidden();
	await expect(page.getByText('Project created')).toBeVisible();
	await expect(page.getByTestId('projects')).toContainText('Launch event');
	// The layout reloaded too: the new project is in the sidebar.
	await expect(page.getByTestId('sidebar-projects')).toContainText('Launch event');
});

test('tasks can be added with a select, completed and deleted from a menu', async ({ page }) => {
	const problems = watchConsole(page);
	await signIn(page);
	await page.getByTestId('sidebar-projects').getByRole('link', { name: 'Mobile app' }).click();
	await expect(page.getByTestId('progress')).toHaveText('1/3 done');

	await page.getByRole('button', { name: 'Add task' }).click();
	await expect(page.getByRole('alert')).toHaveText('A task needs a title of up to 120 characters.');

	await page.getByLabel('Title').fill('Write release notes');
	await page.getByRole('combobox', { name: 'Priority' }).click();
	await page.getByRole('option', { name: 'High' }).click();
	await page.getByRole('button', { name: 'Add task' }).click();
	await expect(page.getByText('Task added')).toBeVisible();
	const task = page.getByTestId('tasks').locator('li', { hasText: 'Write release notes' });
	await expect(task).toContainText('high');
	await expect(page.getByTestId('progress')).toHaveText('1/4 done');

	await page.getByRole('checkbox', { name: 'Complete Write release notes' }).click();
	await expect(page.getByTestId('progress')).toHaveText('2/4 done');

	await page.getByRole('button', { name: 'Actions for Write release notes' }).click();
	await page.getByRole('menuitem', { name: 'Delete task' }).click();
	await expect(task).toHaveCount(0);
	await expect(page.getByTestId('progress')).toHaveText('1/3 done');
	expect(problems).toEqual([]);
});

test('project settings save and deleting a project redirects', async ({ page }) => {
	await signIn(page);
	await page.goto('/projects');
	await page.getByRole('link', { name: 'Q3 planning' }).click();
	await page.getByRole('tab', { name: 'Settings' }).click();

	await page.getByLabel('Name').fill('Q3 planning (final)');
	await page.getByRole('combobox').filter({ hasText: 'Paused' }).click();
	await page.getByRole('option', { name: 'Done' }).click();
	await page.getByRole('button', { name: 'Save changes' }).click();
	await expect(page.getByText('Project saved')).toBeVisible();
	await expect(page.getByTestId('project-name')).toHaveText('Q3 planning (final)');

	await page.getByRole('button', { name: 'Delete project' }).click();
	await expect(page).toHaveURL(/\/projects$/);
	await expect(page.getByTestId('projects')).not.toContainText('Q3 planning');
});

test('settings show field errors and persist', async ({ page }) => {
	await signIn(page);
	await page.goto('/settings');

	await page.getByLabel('Email', { exact: true }).fill('not an email');
	await page.getByRole('button', { name: 'Save' }).click();
	await expect(page.getByRole('alert')).toHaveText("That doesn't look like an email address.");
	await expect(page.getByLabel('Email', { exact: true })).toHaveValue('not an email');

	await page.getByLabel('Display name').fill('Marcel');
	await page.getByLabel('Email', { exact: true }).fill('marcel@example.com');
	await page.getByRole('button', { name: 'Save' }).click();
	await expect(page.getByText('Settings saved')).toBeVisible();
	await expect(page.getByTestId('user')).toHaveText('Marcel');

	await page.reload();
	await expect(page.getByLabel('Email', { exact: true })).toHaveValue('marcel@example.com');
});

test('the user menu toggles the theme and signs out', async ({ page }) => {
	await signIn(page);
	const dark = () => page.evaluate(() => document.documentElement.classList.contains('dark'));
	const before = await dark();
	await page.getByTestId('user-menu').click();
	await page.getByRole('menuitem', { name: 'Toggle theme' }).click();
	await expect.poll(dark).toBe(!before);

	await page.getByTestId('user-menu').click();
	await page.getByRole('menuitem', { name: 'Sign out' }).click();
	await expect(page).toHaveURL(/\/login$/);
});

test('missing projects show the error page', async ({ page }) => {
	await signIn(page);
	const res = await page.goto('/projects/999999');
	expect(res?.status()).toBe(404);
	await expect(page.getByTestId('error-message')).toHaveText('Not Found');
});

test('the stats API returns JSON for the signed-in user', async ({ page }) => {
	await signIn(page);
	const stats = await page.evaluate(() => fetch('/api/stats').then((r) => r.json()));
	expect(stats.projects).toBe(3);
	expect(stats.perDay).toHaveLength(14);
});

test('layout loads are reused across navigations and invalidate reruns only dependents', async ({ page }) => {
	await signIn(page);
	const reusedFor = async (action: () => Promise<void>) => {
		const response = page.waitForResponse((r) => r.url().includes('__data.json'));
		await action();
		const first = (await (await response).text()).split('\n')[0]!;
		return (JSON.parse(first).reused ?? []) as number[];
	};

	expect(await reusedFor(() => page.getByRole('link', { name: 'Projects', exact: true }).click())).toEqual([0, 1]);
	await expect(page.getByRole('heading', { name: 'Projects' })).toBeVisible();

	// Both layouts (the root and the app shell with the sidebar) come from the client's cache.
	expect(await reusedFor(() => page.getByTestId('sidebar-projects').getByRole('link', { name: 'Mobile app' }).click())).toEqual([0, 1]);
	await expect(page.getByTestId('project-name')).toHaveText('Mobile app');
	await expect(page.getByTestId('sidebar-projects')).toContainText('Website redesign');

	await page.getByRole('link', { name: 'Overview' }).click();
	await expect(page.getByTestId('stat-projects')).toHaveText('3');
	// invalidate('app:stats') reruns the overview's load but not the layouts.
	expect(await reusedFor(() => page.getByRole('button', { name: 'Refresh' }).click())).toEqual([0, 1]);
});

test('beforeNavigate can stop navigation with unsaved changes', async ({ page }) => {
	await signIn(page);
	await page.goto('/settings');
	await page.getByLabel('Display name').fill('Changed');

	page.once('dialog', (d) => void d.dismiss());
	await page.getByRole('link', { name: 'Projects', exact: true }).click();
	await expect(page).toHaveURL(/\/settings$/);
	await expect(page.getByLabel('Display name')).toHaveValue('Changed');

	page.once('dialog', (d) => void d.accept());
	await page.getByRole('link', { name: 'Projects', exact: true }).click();
	await expect(page).toHaveURL(/\/projects$/);
});
